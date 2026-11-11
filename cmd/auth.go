package cmd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
	"github.com/cloudwalksolutions/ccloud/internal/kubeconfig"
)

func auth(c *cobra.Command, home cloud.Cloud, region cloud.Region) error {
	entry := kubeconfig.Entry(home, region)
	trial := kubeconfig.Path() + "." + region.Context()
	defer func() { _ = os.Remove(trial) }()
	if err := clientcmd.WriteToFile(*entry, trial); err != nil {
		return err
	}

	var reason bytes.Buffer
	whoami := exec.CommandContext(c.Context(), "kubectl", "--kubeconfig", trial, "auth", "whoami")
	whoami.Stdout, whoami.Stderr = c.OutOrStdout(), &reason
	if err := whoami.Run(); err != nil {
		return fmt.Errorf("signing in to %s failed, so your kubeconfig is unchanged: %s", region.Name, strings.TrimSpace(reason.String()))
	}
	if err := kubeconfig.Merge(kubeconfig.Path(), entry); err != nil {
		return err
	}
	return switched(c, region)
}
