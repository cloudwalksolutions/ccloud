package workflows

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

var resource = schema.GroupVersionResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "workflows"}

type Run struct {
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
	slices.SortFunc(steps, func(a, b Step) int {
		switch {
		case a.StartedAt.IsZero() != b.StartedAt.IsZero():
			if a.StartedAt.IsZero() {
				return 1
			}
			return -1
		default:
			return a.StartedAt.Compare(b.StartedAt)
		}
	})
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
	var run Run
	raw, err := json.Marshal(item.Object)
	if err != nil {
		return run, err
	}
	return run, json.Unmarshal(raw, &run)
}

func Get(ctx context.Context, cfg *rest.Config, namespace, name string) (Run, error) {
	client, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return Run{}, err
	}
	item, err := client.Resource(resource).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return Run{}, err
	}
	return decode(item)
}

func Cancel(ctx context.Context, cfg *rest.Config, namespace, name string) error {
	client, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return err
	}
	_, err = client.Resource(resource).Namespace(namespace).Patch(ctx, name, types.MergePatchType, []byte(`{"spec":{"shutdown":"Stop"}}`), metav1.PatchOptions{})
	return err
}

func Rerun(ctx context.Context, cfg *rest.Config, namespace, name string) (string, error) {
	client, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return "", err
	}
	runs := client.Resource(resource).Namespace(namespace)
	original, err := runs.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
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
	created, err := runs.Create(ctx, rerun, metav1.CreateOptions{})
	if err != nil {
		return "", err
	}
	return created.GetName(), nil
}

func List(ctx context.Context, cfg *rest.Config, namespace, selector string) ([]Run, error) {
	client, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	list, err := client.Resource(resource).Namespace(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
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
