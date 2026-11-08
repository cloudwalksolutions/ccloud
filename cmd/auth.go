package cmd

import (
	"bytes"
	"fmt"
	"strings"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"

	"ccloud/internal/kubeconfig"
	"ccloud/internal/regions"
)

func auth(c *cobra.Command, region regions.Region) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	scratch, err := os.MkdirTemp("", "ccloud-auth-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(scratch) }()

	entry := kubeconfig.Entry(region, self)
	trial := filepath.Join(scratch, "kubeconfig")
	if err := clientcmd.WriteToFile(*entry, trial); err != nil {
		return err
	}

	var reason bytes.Buffer
	whoami := exec.Command("kubectl", "--kubeconfig", trial, "auth", "whoami")
	whoami.Stdout, whoami.Stderr = c.OutOrStdout(), &reason
	if err := whoami.Run(); err != nil {
		return fmt.Errorf("signing in to %s failed, so your kubeconfig is unchanged: %s", region.Name, strings.TrimSpace(reason.String()))
	}
	if err := kubeconfig.Merge(kubeconfig.Path(), entry); err != nil {
		return err
	}
	_, err = fmt.Fprintf(c.OutOrStdout(), "Switched to context %q.\n", region.Context())
	return err
}
