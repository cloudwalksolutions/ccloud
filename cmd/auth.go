package cmd

import (
	"bytes"
	"fmt"
	"io"
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

	done := step(c, fmt.Sprintf("Signing in to %s's cluster", region.Name))
	var who, reason bytes.Buffer
	whoami := exec.CommandContext(c.Context(), "kubectl", "--kubeconfig", trial, "auth", "whoami")
	whoami.Stdout, whoami.Stderr = &who, io.MultiWriter(c.ErrOrStderr(), &reason)
	err := whoami.Run()
	done(err, fmt.Sprintf("Signed in to %s's cluster", region.Name))
	if err != nil {
		return fmt.Errorf("signing in to %s failed, so your kubeconfig is unchanged: %s", region.Name, strings.TrimSpace(reason.String()))
	}
	fmt.Fprint(c.OutOrStdout(), who.String())
	if err := kubeconfig.Merge(kubeconfig.Path(), entry); err != nil {
		return err
	}
	return switched(c, region)
}
