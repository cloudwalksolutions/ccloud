package workflows

import (
	"cmp"
	"context"
	"encoding/json"
	"io"
	"math"
	"slices"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
)

var resource = schema.GroupVersionResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "workflows"}

type Run struct {
	object   *unstructured.Unstructured
	Metadata struct {
		Name              string            `json:"name"`
		CreationTimestamp time.Time         `json:"creationTimestamp"`
		Labels            map[string]string `json:"labels"`
	} `json:"metadata"`
	Spec struct {
		Arguments struct {
			Parameters []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
			} `json:"parameters"`
		} `json:"arguments"`
	} `json:"spec"`
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
	for _, parameter := range r.Spec.Arguments.Parameters {
		if parameter.Name == "revision" {
			return parameter.Value
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

func decode(item *unstructured.Unstructured) (Run, error) {
	run := Run{object: item}
	raw, err := json.Marshal(item.Object)
	if err != nil {
		return run, err
	}
	return run, json.Unmarshal(raw, &run)
}

type Runs struct {
	workflows dynamic.ResourceInterface
	pods      corev1client.PodInterface
}

func In(cfg *rest.Config, namespace string) (Runs, error) {
	workflows, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return Runs{}, err
	}
	pods, err := corev1client.NewForConfig(cfg)
	if err != nil {
		return Runs{}, err
	}
	return Runs{workflows: workflows.Resource(resource).Namespace(namespace), pods: pods.Pods(namespace)}, nil
}

func (r Runs) Get(ctx context.Context, name string) (Run, error) {
	item, err := r.workflows.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return Run{}, err
	}
	return decode(item)
}

func (r Runs) StepLog(ctx context.Context, run Run, step Step) (io.ReadCloser, error) {
	return r.pods.GetLogs(run.PodName(step), &corev1.PodLogOptions{Container: "main"}).Stream(ctx)
}

func (r Runs) Cancel(ctx context.Context, name string) error {
	_, err := r.workflows.Patch(ctx, name, types.MergePatchType, []byte(`{"spec":{"shutdown":"Stop"}}`), metav1.PatchOptions{})
	return err
}

func (r Runs) Rerun(ctx context.Context, run Run) (string, error) {
	original := run.object
	spec, _, _ := unstructured.NestedMap(original.Object, "spec")
	delete(spec, "shutdown")
	labels := map[string]any{}
	for key, value := range original.GetLabels() {
		if !strings.HasPrefix(key, "workflows.argoproj.io/") {
			labels[key] = value
		}
	}
	rerun := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": resource.GroupVersion().String(),
		"kind":       "Workflow",
		"metadata":   map[string]any{"generateName": original.GetGenerateName(), "labels": labels},
		"spec":       spec,
	}}
	created, err := r.workflows.Create(ctx, rerun, metav1.CreateOptions{})
	if err != nil {
		return "", err
	}
	return created.GetName(), nil
}

func (r Runs) List(ctx context.Context, selector string) ([]Run, error) {
	list, err := r.workflows.List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, err
	}
	runs := make([]Run, 0, len(list.Items))
	for i := range list.Items {
		run, err := decode(&list.Items[i])
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	slices.SortFunc(runs, func(a, b Run) int {
		return b.Metadata.CreationTimestamp.Compare(a.Metadata.CreationTimestamp)
	})
	return runs, nil
}
