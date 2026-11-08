package cmd

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"ccloud/internal/google"
	"ccloud/internal/regions"
)

func clustersCmd() *cobra.Command {
	clusters := &cobra.Command{
		Use:   "clusters",
		Short: "Reach a region's Kubernetes cluster as yourself",
	}
	clusters.AddCommand(regionCmd(&cobra.Command{
		Use:     "auth --region <region>",
		Short:   "Sign in to a region's cluster and add it to your kubeconfig as ccloud-<region>",
		Example: "  ccloud clusters auth --region us-central1",
	}, func(c *cobra.Command, region regions.Region) error {
		if google.ClientID == "" {
			return errors.New("this ccloud was built without a Google client; build it with the -ldflags in cli/README.md")
		}
		for _, tool := range []string{"kubectl", "cloudflared", "kubectl-oidc_login"} {
			if _, err := exec.LookPath(tool); err != nil {
				return fmt.Errorf("%s is not on your PATH; ccloud needs kubectl, cloudflared and kubelogin (kubectl-oidc_login)", tool)
			}
		}
		return auth(c, region)
	}))
	clusters.AddCommand(regionCmd(&cobra.Command{
		Use:    "credentials --region <region>",
		Short:  "kubectl's exec plugin for a ccloud context: the tunnel, then your Google token",
		Hidden: true,
	}, credentials))
	return clusters
}

func regionCmd(command *cobra.Command, run func(*cobra.Command, regions.Region) error) *cobra.Command {
	var name string
	command.Flags().StringVar(&name, "region", "", "the datacenter region: "+strings.Join(regions.Names(), ", "))
	command.Args = cobra.NoArgs
	command.RunE = func(c *cobra.Command, _ []string) error {
		if name == "" {
			return fmt.Errorf("which region? %s --region <region>, where <region> is one of %s", c.CommandPath(), strings.Join(regions.Names(), ", "))
		}
		region, err := regions.Find(name)
		if err != nil {
			return err
		}
		return run(c, region)
	}
	return command
}
