package cmd

import (
	"errors"
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"

	"ccloud/internal/google"
	"ccloud/internal/regions"
)

func clustersCmd() *cobra.Command {
	clusters := &cobra.Command{
		Use:   "clusters",
		Short: "Reach a region's Kubernetes cluster as yourself",
	}
	clusters.AddCommand(&cobra.Command{
		Use:   "auth <region>",
		Short: "Sign in to a region's cluster and add it to your kubeconfig as ccloud-<region>",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			region, err := regions.Find(args[0])
			if err != nil {
				return err
			}
			if google.ClientID == "" {
				return errors.New("this ccloud was built without a Google client; build it with the -ldflags in cli/README.md")
			}
			for _, tool := range []string{"kubectl", "cloudflared", "kubectl-oidc_login"} {
				if _, err := exec.LookPath(tool); err != nil {
					return fmt.Errorf("%s is not on your PATH; ccloud needs kubectl, cloudflared and kubelogin (kubectl-oidc_login)", tool)
				}
			}
			return auth(c, region)
		},
	})
	clusters.AddCommand(&cobra.Command{
		Use:    "credentials <region>",
		Short:  "kubectl's exec plugin for a ccloud context: the tunnel, then your Google token",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			region, err := regions.Find(args[0])
			if err != nil {
				return err
			}
			return credentials(c, region)
		},
	})
	return clusters
}
