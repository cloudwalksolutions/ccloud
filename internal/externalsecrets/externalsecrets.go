package externalsecrets

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	appsv1client "k8s.io/client-go/kubernetes/typed/apps/v1"
	"k8s.io/client-go/rest"
)

type Secret struct {
	Env, Project, Name string
}

type Namespace struct {
	apps      *appsv1client.AppsV1Client
	namespace string
}

func In(cfg *rest.Config, namespace string) (Namespace, error) {
	apps, err := appsv1client.NewForConfig(cfg)
	return Namespace{apps: apps, namespace: namespace}, err
}

type remote struct{ store, name string }

func (n Namespace) Secrets(ctx context.Context, deployment, container string) ([]Secret, error) {
	env, err := n.env(ctx, deployment, container)
	var remotes map[string]remote
	if err == nil {
		remotes, err = n.remotes(ctx)
	}
	projects := map[string]string{}
	var secrets []Secret
	for _, variable := range env {
		ref := variable.ValueFrom.SecretKeyRef
		from, ok := remotes[ref.Name+"/"+ref.Key]
		if !ok {
			continue
		}
		if _, known := projects[from.store]; !known {
			if projects[from.store], err = n.project(ctx, from.store); err != nil {
				return nil, fmt.Errorf("reading secret store %s: %w", from.store, err)
			}
		}
		secrets = append(secrets, Secret{Env: variable.Name, Project: projects[from.store], Name: from.name})
	}
	return secrets, err
}

var ErrNoDeployment = errors.New("no such deployment")

func (n Namespace) env(ctx context.Context, deployment, container string) ([]corev1.EnvVar, error) {
	found, err := n.apps.Deployments(n.namespace).Get(ctx, deployment, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		err = ErrNoDeployment
	}
	var env []corev1.EnvVar
	for _, c := range found.Spec.Template.Spec.Containers {
		if c.Name != container {
			continue
		}
		for _, variable := range c.Env {
			if variable.ValueFrom != nil && variable.ValueFrom.SecretKeyRef != nil {
				env = append(env, variable)
			}
		}
	}
	return env, err
}

func (n Namespace) remotes(ctx context.Context) (map[string]remote, error) {
	var list struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Spec struct {
				SecretStoreRef struct {
					Name string `json:"name"`
				} `json:"secretStoreRef"`
				Target struct {
					Name string `json:"name"`
				} `json:"target"`
				Data []struct {
					SecretKey string `json:"secretKey"`
					RemoteRef struct {
						Key string `json:"key"`
					} `json:"remoteRef"`
				} `json:"data"`
			} `json:"spec"`
		} `json:"items"`
	}
	err := n.get(ctx, "externalsecrets", &list)
	remotes := map[string]remote{}
	for _, item := range list.Items {
		target := cmp.Or(item.Spec.Target.Name, item.Metadata.Name)
		for _, data := range item.Spec.Data {
			remotes[target+"/"+data.SecretKey] = remote{store: item.Spec.SecretStoreRef.Name, name: data.RemoteRef.Key}
		}
	}
	return remotes, err
}

func (n Namespace) project(ctx context.Context, store string) (string, error) {
	var found struct {
		Spec struct {
			Provider struct {
				GCPSM struct {
					ProjectID string `json:"projectID"`
				} `json:"gcpsm"`
			} `json:"provider"`
		} `json:"spec"`
	}
	err := n.get(ctx, "secretstores/"+store, &found)
	return found.Spec.Provider.GCPSM.ProjectID, err
}

func (n Namespace) get(ctx context.Context, resource string, into any) error {
	raw, err := n.apps.RESTClient().Get().AbsPath("/apis/external-secrets.io/v1/namespaces", n.namespace, resource).DoRaw(ctx)
	if err == nil {
		err = json.Unmarshal(raw, into)
	}
	return err
}
