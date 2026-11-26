package cmd_test

import (
	"bytes"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/apimachinery/pkg/util/httpstream/spdy"
	"k8s.io/client-go/tools/clientcmd"
	api "k8s.io/client-go/tools/clientcmd/api"
)

type workflow map[string]any

func ciRun(name string, created time.Time) workflow {
	stamp := created.UTC().Format(time.RFC3339)
	return workflow{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "Workflow",
		"metadata":   map[string]any{"name": name, "creationTimestamp": stamp, "labels": map[string]any{}},
		"spec":       map[string]any{"arguments": map[string]any{"parameters": []any{}}},
		"status":     map[string]any{"phase": "Pending", "startedAt": stamp},
	}
}

func (w workflow) label(key, value string) workflow {
	w["metadata"].(map[string]any)["labels"].(map[string]any)[key] = value
	return w
}

func (w workflow) flow(name string) workflow { return w.label("flow", name) }

func (w workflow) from(template string) workflow {
	w["spec"].(map[string]any)["workflowTemplateRef"] = map[string]any{"name": template}
	return w
}

func (w workflow) pr(number, sha, branch string) workflow {
	arguments := w["spec"].(map[string]any)["arguments"].(map[string]any)
	arguments["parameters"] = append(arguments["parameters"].([]any), map[string]any{"name": "revision", "value": branch})
	return w.label("pr", number).label("sha", sha)
}

func (w workflow) queued() workflow {
	delete(w["status"].(map[string]any), "startedAt")
	return w
}

func (w workflow) running() workflow {
	w["status"].(map[string]any)["phase"] = "Running"
	return w
}

func (w workflow) finished(phase string, took time.Duration) workflow {
	status := w["status"].(map[string]any)
	started, _ := time.Parse(time.RFC3339, status["startedAt"].(string))
	status["phase"] = phase
	status["finishedAt"] = started.Add(took).Format(time.RFC3339)
	return w
}

func (w workflow) step(id, name, phase string, started time.Time, took time.Duration, message string) workflow {
	status := w["status"].(map[string]any)
	nodes, _ := status["nodes"].(map[string]any)
	if nodes == nil {
		nodes = map[string]any{}
		status["nodes"] = nodes
	}
	node := map[string]any{"id": id, "displayName": name, "templateName": name, "type": "Pod", "phase": phase, "message": message}
	if !started.IsZero() {
		node["startedAt"] = started.UTC().Format(time.RFC3339)
		if took > 0 {
			node["finishedAt"] = started.Add(took).UTC().Format(time.RFC3339)
		}
	}
	nodes[id] = node
	nodes[w["metadata"].(map[string]any)["name"].(string)] = map[string]any{"id": "dag", "displayName": "dag", "type": "DAG", "phase": status["phase"]}
	return w
}

type fakeCluster struct {
	server     *httptest.Server
	byNS       map[string][]workflow
	forbidden  map[string]bool
	versions   []workflow
	logs       map[string]string
	unwritable map[string]bool
	writes     []written
	secrets    map[string]map[string]string
	pods       map[string][]corev1.Pod
	forwarded  chan string
	objects    map[string]any
	answers    map[string]string
	complaints map[string]string
	execs      []string
}

func newFakeCluster() *fakeCluster {
	f := &fakeCluster{byNS: map[string][]workflow{}, forbidden: map[string]bool{}, logs: map[string]string{}, unwritable: map[string]bool{},
		secrets: map[string]map[string]string{}, pods: map[string][]corev1.Pod{}, forwarded: make(chan string, 10), objects: map[string]any{},
		answers: map[string]string{}, complaints: map[string]string{}}
	f.server = httptest.NewTLSServer(http.HandlerFunc(f.serve))
	DeferCleanup(f.server.Close)
	return f
}

func (f *fakeCluster) workflows(namespace string, runs ...workflow) {
	f.byNS[namespace] = append(f.byNS[namespace], runs...)
}

func (f *fakeCluster) progress(namespace string, versions ...workflow) {
	f.versions = versions
	f.workflows(namespace, versions[0])
}

func (f *fakeCluster) forbid(namespace string) {
	f.forbidden[namespace] = true
}

func (f *fakeCluster) readOnly(namespace string) {
	f.unwritable[namespace] = true
}

type written struct {
	method, contentType string
	body                map[string]any
}

func (f *fakeCluster) write(w http.ResponseWriter, r *http.Request, namespace string) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/apis/argoproj.io/v1alpha1/namespaces/"), "/")
	if f.unwritable[namespace] {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"apiVersion": "v1", "kind": "Status", "status": "Failure", "reason": "Forbidden", "code": 403,
			"message": `workflows.argoproj.io is forbidden: User "dev@example.test" cannot ` + strings.ToLower(r.Method) + ` resource "workflows"`,
		})
		return
	}
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	f.writes = append(f.writes, written{method: r.Method, contentType: r.Header.Get("Content-Type"), body: body})
	if r.Method == http.MethodPost {
		generateName := body["metadata"].(map[string]any)["generateName"].(string)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"apiVersion": body["apiVersion"], "kind": body["kind"], "spec": body["spec"],
			"metadata": map[string]any{"name": generateName + "r3run", "generateName": generateName},
		})
		return
	}
	for _, run := range f.byNS[namespace] {
		if run["metadata"].(map[string]any)["name"] == parts[2] {
			run["spec"].(map[string]any)["shutdown"] = "Stop"
			_ = json.NewEncoder(w).Encode(run)
			return
		}
	}
	http.NotFound(w, r)
}

func (f *fakeCluster) podLog(namespace, pod, text string) {
	f.logs[namespace+"/"+pod] = text
}

func (f *fakeCluster) serveLog(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/namespaces/"), "/")
	text, ok := f.logs[parts[0]+"/"+parts[2]]
	if !ok || r.URL.Query().Get("container") != "main" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"apiVersion": "v1", "kind": "Status", "status": "Failure", "reason": "NotFound", "code": 404,
			"message": `pods "` + parts[2] + `" not found`,
		})
		return
	}
	_, _ = w.Write([]byte(text))
}

func (f *fakeCluster) secret(namespace, name string, data map[string]string) {
	f.secrets[namespace+"/"+name] = data
}

func (f *fakeCluster) pod(namespace, name string, podLabels map[string]string) {
	f.pods[namespace] = append(f.pods[namespace], corev1.Pod{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: podLabels},
	})
}

func (f *fakeCluster) status(w http.ResponseWriter, code int, reason metav1.StatusReason, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(metav1.Status{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Status"},
		Status:   metav1.StatusFailure, Reason: reason, Code: int32(code), Message: message,
	})
}

func (f *fakeCluster) serveCore(w http.ResponseWriter, r *http.Request, namespace, kind string, rest []string) {
	if f.forbidden[namespace] {
		f.status(w, http.StatusForbidden, metav1.StatusReasonForbidden, kind+` is forbidden: User "dev@example.test" cannot get resource "`+kind+`"`)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	switch {
	case kind == "secrets" && len(rest) == 1 && f.secrets[namespace+"/"+rest[0]] != nil:
		secret := corev1.Secret{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"}, ObjectMeta: metav1.ObjectMeta{Name: rest[0]}, Data: map[string][]byte{}}
		for key, value := range f.secrets[namespace+"/"+rest[0]] {
			secret.Data[key] = []byte(value)
		}
		_ = json.NewEncoder(w).Encode(secret)
	case kind == "pods" && len(rest) == 0:
		selector, _ := labels.Parse(r.URL.Query().Get("labelSelector"))
		list := corev1.PodList{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "PodList"}}
		for _, pod := range f.pods[namespace] {
			if selector.Matches(labels.Set(pod.Labels)) {
				list.Items = append(list.Items, pod)
			}
		}
		_ = json.NewEncoder(w).Encode(list)
	case kind == "pods" && len(rest) == 2 && f.unwritable[namespace]:
		f.status(w, http.StatusForbidden, metav1.StatusReasonForbidden, `pods "`+rest[0]+`" is forbidden: User "dev@example.test" cannot create resource "pods/`+rest[1]+`"`)
	case kind == "pods" && len(rest) == 2 && rest[1] == "portforward":
		f.portForward(w, r, namespace+"/"+rest[0])
	case kind == "pods" && len(rest) == 2 && rest[1] == "exec":
		f.serveExec(w, r, namespace+"/"+rest[0])
	default:
		f.status(w, http.StatusNotFound, metav1.StatusReasonNotFound, kind+` "`+strings.Join(rest, "/")+`" not found`)
	}
}

func (f *fakeCluster) portForward(w http.ResponseWriter, r *http.Request, pod string) {
	if _, err := httpstream.Handshake(r, w, []string{"portforward.k8s.io"}); err != nil {
		return
	}
	streams := make(chan httpstream.Stream)
	connection := spdy.NewResponseUpgrader().UpgradeResponse(w, r, func(stream httpstream.Stream, _ <-chan struct{}) error {
		streams <- stream
		return nil
	})
	if connection == nil {
		return
	}
	defer func() { _ = connection.Close() }()
	for {
		select {
		case stream := <-streams:
			if stream.Headers().Get("streamType") == "error" {
				_ = stream.Close()
				continue
			}
			f.forwarded <- pod + ":" + stream.Headers().Get("port")
			go func() {
				_, _ = io.Copy(stream, stream)
				_ = stream.Close()
			}()
		case <-connection.CloseChan():
			return
		}
	}
}

func (f *fakeCluster) exec(pod, stdout string) {
	f.answers[pod] = stdout
}

func (f *fakeCluster) execFails(pod, stderr string) {
	f.complaints[pod] = stderr
}

func (f *fakeCluster) serveExec(w http.ResponseWriter, r *http.Request, pod string) {
	query := r.URL.Query()
	f.execs = append(f.execs, pod+" "+query.Get("container")+": "+strings.Join(query["command"], " "))
	if _, err := httpstream.Handshake(r, w, []string{"v4.channel.k8s.io"}); err != nil {
		return
	}
	streams := make(chan httpstream.Stream)
	connection := spdy.NewResponseUpgrader().UpgradeResponse(w, r, func(stream httpstream.Stream, _ <-chan struct{}) error {
		streams <- stream
		return nil
	})
	if connection == nil {
		return
	}
	defer func() { _ = connection.Close() }()
	var failures httpstream.Stream
	written := 0
	finish := func() {
		if written < 2 || failures == nil {
			return
		}
		if complaint, ok := f.complaints[pod]; ok {
			_ = json.NewEncoder(failures).Encode(metav1.Status{Status: metav1.StatusFailure, Message: "command terminated with exit code 1: " + complaint})
		}
		_ = failures.Close()
	}
	for {
		select {
		case stream := <-streams:
			switch stream.Headers().Get("streamType") {
			case "stdout":
				_, _ = io.WriteString(stream, f.answers[pod])
				_ = stream.Close()
				written++
			case "stderr":
				_, _ = io.WriteString(stream, f.complaints[pod])
				_ = stream.Close()
				written++
			case "error":
				failures = stream
			default:
				_ = stream.Close()
			}
			finish()
		case <-connection.CloseChan():
			return
		}
	}
}

func (f *fakeCluster) object(path string, object any) {
	f.objects[path] = object
}

func (f *fakeCluster) serve(w http.ResponseWriter, r *http.Request) {
	if object, ok := f.objects[r.URL.Path]; ok {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(object)
		return
	}
	if core, ok := strings.CutPrefix(r.URL.Path, "/api/v1/namespaces/"); ok && !strings.HasSuffix(core, "/log") {
		parts := strings.Split(core, "/")
		f.serveCore(w, r, parts[0], parts[1], parts[2:])
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/v1/namespaces/") {
		f.serveLog(w, r)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/apis/argoproj.io/v1alpha1/namespaces/"), "/")
	if len(parts) < 2 || parts[1] != "workflows" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if f.forbidden[parts[0]] {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"apiVersion": "v1", "kind": "Status", "status": "Failure", "reason": "Forbidden", "code": 403,
			"message": `workflows.argoproj.io is forbidden: User "dev@example.test" cannot list resource "workflows"`,
		})
		return
	}
	if r.Method != http.MethodGet {
		f.write(w, r, parts[0])
		return
	}
	if len(parts) == 3 {
		f.get(w, parts[0], parts[2])
		return
	}
	selector, err := labels.Parse(r.URL.Query().Get("labelSelector"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	items := []workflow{}
	for _, run := range f.byNS[parts[0]] {
		set := labels.Set{}
		for key, value := range run["metadata"].(map[string]any)["labels"].(map[string]any) {
			set[key] = value.(string)
		}
		if selector.Matches(set) {
			items = append(items, run)
		}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "WorkflowList",
		"metadata":   map[string]any{},
		"items":      items,
	})
}

func (f *fakeCluster) get(w http.ResponseWriter, namespace, name string) {
	for i, run := range f.byNS[namespace] {
		if run["metadata"].(map[string]any)["name"] == name {
			if len(f.versions) > 1 {
				f.versions = f.versions[1:]
				f.byNS[namespace][i] = f.versions[0]
			}
			if f.byNS[namespace][i] == nil {
				f.byNS[namespace] = slices.Delete(f.byNS[namespace], i, i+1)
			}
			_ = json.NewEncoder(w).Encode(run)
			return
		}
	}
	w.WriteHeader(http.StatusNotFound)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"apiVersion": "v1", "kind": "Status", "status": "Failure", "reason": "NotFound", "code": 404,
		"message": `workflows.argoproj.io "` + name + `" not found`,
		"details": map[string]any{"name": name, "group": "argoproj.io", "kind": "workflows"},
	})
}

func (f *fakeCluster) signIn(region string) {
	cfg := api.NewConfig()
	name := "ccloud-" + region
	authority := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: f.server.Certificate().Raw})
	cfg.Clusters[name] = &api.Cluster{Server: f.server.URL, CertificateAuthorityData: authority}
	cfg.AuthInfos[name] = &api.AuthInfo{Token: "dev-token"}
	cfg.Contexts[name] = &api.Context{Cluster: name, AuthInfo: name}
	cfg.CurrentContext = name
	Expect(clientcmd.WriteToFile(*cfg, os.Getenv("KUBECONFIG"))).To(Succeed())
}

func lines(out *bytes.Buffer) []string {
	return strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
}
