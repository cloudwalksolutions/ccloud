package database

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
)

type Credentials struct {
	User, Password, Name string
}

type Database struct {
	core      *corev1client.CoreV1Client
	transport http.RoundTripper
	upgrader  spdy.Upgrader
	namespace string
	where     cloud.Database
}

func In(cfg *rest.Config, namespace string, where cloud.Database) (Database, error) {
	core, coreErr := corev1client.NewForConfig(cfg)
	transport, upgrader, spdyErr := spdy.RoundTripperFor(cfg)
	return Database{core: core, transport: transport, upgrader: upgrader, namespace: namespace, where: where}, errors.Join(coreErr, spdyErr)
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
