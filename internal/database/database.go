package database

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	batchv1client "k8s.io/client-go/kubernetes/typed/batch/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/transport/spdy"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
)

type Credentials struct {
	User, Password, Name string
}

type Database struct {
	core      *corev1client.CoreV1Client
	batch     *batchv1client.BatchV1Client
	transport http.RoundTripper
	upgrader  spdy.Upgrader
	namespace string
	where     cloud.Database
}

func In(cfg *rest.Config, namespace string, where cloud.Database) (Database, error) {
	core, coreErr := corev1client.NewForConfig(cfg)
	batch, batchErr := batchv1client.NewForConfig(cfg)
	transport, upgrader, spdyErr := spdy.RoundTripperFor(cfg)
	return Database{core: core, batch: batch, transport: transport, upgrader: upgrader, namespace: namespace, where: where}, errors.Join(coreErr, batchErr, spdyErr)
}

func (d Database) CopiedOffsite(ctx context.Context) (time.Time, error) {
	jobs, err := d.batch.Jobs(d.where.Offsite.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return time.Time{}, err
	}
	var copied time.Time
	for _, job := range jobs.Items {
		if !slices.ContainsFunc(job.OwnerReferences, func(o metav1.OwnerReference) bool { return o.Kind == "CronJob" && o.Name == d.where.Offsite.CronJob }) {
			continue
		}
		for _, condition := range job.Status.Conditions {
			if condition.Type == batchv1.JobComplete && condition.Status == corev1.ConditionTrue && job.Status.StartTime != nil && job.Status.StartTime.After(copied) {
				copied = job.Status.StartTime.Time
			}
		}
	}
	return copied, nil
}

func (d Database) Credentials(ctx context.Context) (Credentials, error) {
	secret, err := d.core.Secrets(d.namespace).Get(ctx, d.where.Secret, metav1.GetOptions{})
	if err != nil {
		return Credentials{}, err
	}
	return Credentials{User: string(secret.Data["user"]), Password: string(secret.Data["password"]), Name: string(secret.Data["dbname"])}, nil
}

var ErrNoPrimary = errors.New("no primary running")

func (d Database) Primary(ctx context.Context) (string, error) {
	pods, err := d.core.Pods(d.namespace).List(ctx, metav1.ListOptions{LabelSelector: d.where.Primary})
	if err != nil || len(pods.Items) == 0 {
		return "", cmp.Or(err, ErrNoPrimary)
	}
	return pods.Items[0].Name, nil
}

type Backup struct {
	Label    string
	Type     string
	Finished time.Time
	Stored   int64
	Note     string
}

func (d Database) Backups(ctx context.Context, pod string, options ...string) ([]Backup, error) {
	command := append([]string{"pgbackrest", "info", "--stanza=db", "--output=json"}, options...)
	url := d.core.RESTClient().Post().Resource("pods").Namespace(d.namespace).Name(pod).SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{Container: d.where.Container, Command: command, Stdout: true, Stderr: true}, scheme.ParameterCodec).URL()
	executor, err := remotecommand.NewSPDYExecutorForTransports(d.transport, d.upgrader, http.MethodPost, url)
	var stdout, stderr bytes.Buffer
	if err == nil {
		err = executor.StreamWithContext(ctx, remotecommand.StreamOptions{Stdout: &stdout, Stderr: &stderr})
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	var stanzas []struct {
		Backup []struct {
			Label     string `json:"label"`
			Type      string `json:"type"`
			Timestamp struct {
				Stop int64 `json:"stop"`
			} `json:"timestamp"`
			Info struct {
				Repository struct {
					Size int64 `json:"size"`
				} `json:"repository"`
			} `json:"info"`
			Annotation struct {
				Note string `json:"note"`
			} `json:"annotation"`
		} `json:"backup"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &stanzas); err != nil {
		return nil, fmt.Errorf("pgbackrest info answered something other than JSON: %w", err)
	}
	var backups []Backup
	for _, stanza := range stanzas {
		for _, b := range stanza.Backup {
			backups = append(backups, Backup{Label: b.Label, Type: b.Type, Finished: time.Unix(b.Timestamp.Stop, 0), Stored: b.Info.Repository.Size, Note: b.Annotation.Note})
		}
	}
	return backups, nil
}

func (d Database) Forward(ctx context.Context, pod string, local int, ready chan struct{}, errOut io.Writer) error {
	url := d.core.RESTClient().Post().Resource("pods").Namespace(d.namespace).Name(pod).SubResource("portforward").URL()
	dialer := spdy.NewDialer(d.upgrader, &http.Client{Transport: d.transport}, http.MethodPost, url)
	ports := []string{fmt.Sprintf("%d:%d", local, d.where.Port)}
	forwarder, err := portforward.NewOnAddresses(dialer, []string{"localhost"}, ports, ctx.Done(), ready, io.Discard, errOut)
	if err == nil {
		err = forwarder.ForwardPorts()
	}
	return err
}
