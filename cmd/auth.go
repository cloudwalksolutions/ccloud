package cmd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
	"github.com/cloudwalksolutions/ccloud/internal/kubeconfig"
)

func auth(c *cobra.Command, home cloud.Cloud, region cloud.Region) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	scratch, err := os.MkdirTemp("", "ccloud-auth-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(scratch) }()

	entry := kubeconfig.Entry(home, region, self)
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
	return switched(c, region)
}
