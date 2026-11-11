package workflows

import (
	"cmp"
	"context"
	"encoding/json"
	"maps"
	"math"
	"slices"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
)

type Run struct {
	Metadata struct {
		Name              string            `json:"name"`
		GenerateName      string            `json:"generateName"`
		CreationTimestamp time.Time         `json:"creationTimestamp"`
		Labels            map[string]string `json:"labels"`
	} `json:"metadata"`
	Spec   map[string]any `json:"spec"`
	Status struct {
		Phase      string          `json:"phase"`
		StartedAt  time.Time       `json:"startedAt"`
		FinishedAt time.Time       `json:"finishedAt"`
		Nodes      map[string]Step `json:"nodes"`
	} `json:"status"`
}

type Step struct {
	ID           string    `json:"id"`
	TemplateName string    `json:"templateName"`
	DisplayName  string    `json:"displayName"`
	Type         string    `json:"type"`
	Phase        string    `json:"phase"`
	Message      string    `json:"message"`
	StartedAt    time.Time `json:"startedAt"`
	FinishedAt   time.Time `json:"finishedAt"`
}

func (s Step) Took() time.Duration {
	if s.StartedAt.IsZero() || s.FinishedAt.IsZero() {
		return 0
	}
	return s.FinishedAt.Sub(s.StartedAt)
}

func (s Step) startOrder() int64 {
	if s.StartedAt.IsZero() {
		return math.MaxInt64
	}
	return s.StartedAt.UnixNano()
}

func (r Run) PodName(step Step) string {
	return r.Name() + "-" + step.TemplateName + "-" + strings.TrimPrefix(step.ID, r.Name()+"-")
}

func (r Run) Steps() []Step {
	var steps []Step
	for _, node := range r.Status.Nodes {
		if node.Type == "Pod" && !strings.Contains(node.DisplayName, ".hooks.") && !strings.HasSuffix(node.DisplayName, ".onExit") {
			steps = append(steps, node)
		}
	}
	slices.SortFunc(steps, func(a, b Step) int { return cmp.Compare(a.startOrder(), b.startOrder()) })
	return steps
}

func (r Run) Name() string { return r.Metadata.Name }
func (r Run) Flow() string { return r.Metadata.Labels["flow"] }
func (r Run) PR() string   { return r.Metadata.Labels["pr"] }
func (r Run) SHA() string  { return r.Metadata.Labels["sha"] }

func (r Run) Branch() string {
	arguments, _ := r.Spec["arguments"].(map[string]any)
	parameters, _ := arguments["parameters"].([]any)
	for _, parameter := range parameters {
		named, _ := parameter.(map[string]any)
		if named["name"] == "revision" {
			value, _ := named["value"].(string)
			return value
		}
	}
	return ""
}

func (r Run) Elapsed(now time.Time) time.Duration {
	if r.Status.StartedAt.IsZero() {
		return 0
	}
	if r.Status.FinishedAt.IsZero() {
		return now.Sub(r.Status.StartedAt)
	}
	return r.Status.FinishedAt.Sub(r.Status.StartedAt)
}

type Runs struct {
	core      *corev1client.CoreV1Client
	namespace string
}

func In(cfg *rest.Config, namespace string) (Runs, error) {
	core, err := corev1client.NewForConfig(cfg)
	return Runs{core: core, namespace: namespace}, err
}

func (r Runs) path(name ...string) string {
	return strings.Join(append([]string{"/apis/argoproj.io/v1alpha1/namespaces", r.namespace, "workflows"}, name...), "/")
}

func (r Runs) Get(ctx context.Context, name string) (Run, error) {
	var run Run
	raw, err := r.core.RESTClient().Get().AbsPath(r.path(name)).DoRaw(ctx)
	if err == nil {
		err = json.Unmarshal(raw, &run)
	}
	return run, err
}

func (r Runs) List(ctx context.Context, selector string) ([]Run, error) {
	var list struct {
		Items []Run `json:"items"`
	}
	raw, err := r.core.RESTClient().Get().AbsPath(r.path()).Param("labelSelector", selector).DoRaw(ctx)
	if err == nil {
		err = json.Unmarshal(raw, &list)
	}
	slices.SortFunc(list.Items, func(a, b Run) int {
		return b.Metadata.CreationTimestamp.Compare(a.Metadata.CreationTimestamp)
	})
	return list.Items, err
}

func (r Runs) StepLog(ctx context.Context, run Run, step Step) (string, error) {
	text, err := r.core.Pods(r.namespace).GetLogs(run.PodName(step), &corev1.PodLogOptions{Container: "main"}).DoRaw(ctx)
	return string(text), err
}

func (r Runs) Cancel(ctx context.Context, name string) error {
	return r.core.RESTClient().Patch(types.MergePatchType).AbsPath(r.path(name)).Body([]byte(`{"spec":{"shutdown":"Stop"}}`)).Do(ctx).Error()
}

func (r Runs) Rerun(ctx context.Context, run Run) (string, error) {
	spec := maps.Clone(run.Spec)
	delete(spec, "shutdown")
	labels := map[string]any{}
	for key, value := range run.Metadata.Labels {
		if !strings.HasPrefix(key, "workflows.argoproj.io/") {
			labels[key] = value
		}
	}
	rerun := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "Workflow",
		"metadata":   map[string]any{"generateName": run.Metadata.GenerateName, "labels": labels},
		"spec":       spec,
	}}
	var created Run
	raw, err := r.core.RESTClient().Post().AbsPath(r.path()).Body(rerun).DoRaw(ctx)
	if err == nil {
		err = json.Unmarshal(raw, &created)
	}
	return created.Name(), err
}
