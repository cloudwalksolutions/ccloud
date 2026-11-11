package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"k8s.io/client-go/tools/clientcmd"

	"github.com/cloudwalksolutions/ccloud/internal/kubeconfig"
)

func (c command) auth(ctx context.Context) error {
	if c.home.ClientID == "" {
		return fmt.Errorf("this ccloud was built without an OIDC client for the %s cloud; install a release build", c.home.Name)
	}
	for _, tool := range []string{"kubectl", "cloudflared", "kubectl-oidc_login"} {
		if _, err := exec.LookPath(tool); err != nil {
			return fmt.Errorf("%s is not on your PATH; ccloud needs kubectl, cloudflared and kubelogin (kubectl-oidc_login)", tool)
		}
	}
	entry := kubeconfig.Entry(c.home, c.region)
	trial := kubeconfig.Path() + "." + c.region.Context()
	defer func() { _ = os.Remove(trial) }()
	if err := clientcmd.WriteToFile(*entry, trial); err != nil {
		return err
	}

	var reason bytes.Buffer
	whoami := exec.Command("kubectl", "--kubeconfig", trial, "auth", "whoami")
	whoami.Stdout, whoami.Stderr = c.OutOrStdout(), &reason
	if err := whoami.Run(); err != nil {
		return fmt.Errorf("signing in to %s failed, so your kubeconfig is unchanged: %s", c.region.Name, strings.TrimSpace(reason.String()))
	}
	if err := kubeconfig.Merge(kubeconfig.Path(), entry); err != nil {
		return err
	}
	return c.switched()
}

func (c command) setContext(_ context.Context) error {
	namespace, _ := c.Flags().GetString("namespace")
	if err := kubeconfig.Use(kubeconfig.Path(), c.region.Context(), namespace); err != nil {
		return c.signInError(err)
	}
	return c.switched()
}
