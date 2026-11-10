package cmd

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
)

func clustersCmd(home cloud.Cloud) *cobra.Command {
	first := home.Regions[0].Name
	clusters := &cobra.Command{
		Use:   "clusters",
		Short: "Reach a region's Kubernetes cluster as yourself",
	}
	clusters.AddCommand(regionCmd(home, &cobra.Command{
		Use:     "auth --region <region>",
		Short:   "Sign in to a region's cluster and add it to your kubeconfig as ccloud-<region>",
		Example: "  ccloud clusters auth --region " + first,
	}, func(c *cobra.Command, region cloud.Region) error {
		if home.ClientID == "" {
			return fmt.Errorf("this ccloud was built without an OIDC client for the %s cloud; install a release build", home.Name)
		}
		for _, tool := range []string{"kubectl", "cloudflared", "kubectl-oidc_login"} {
			if _, err := exec.LookPath(tool); err != nil {
				return fmt.Errorf("%s is not on your PATH; ccloud needs kubectl, cloudflared and kubelogin (kubectl-oidc_login)", tool)
			}
		}
		return auth(c, home, region)
	}))
	set := regionCmd(home, &cobra.Command{
		Use:     "set-context --region <region> [--namespace <namespace>]",
		Short:   "Switch kubectl to a region you have signed in to",
		Example: "  ccloud clusters set-context --region " + first + " --namespace <namespace>",
	}, setContext)
	set.Flags().String("namespace", "", "the namespace kubectl uses there; left as it was when omitted")
	clusters.AddCommand(set)
	clusters.AddCommand(regionCmd(home, &cobra.Command{
		Use:    "credentials --region <region>",
		Short:  "kubectl's exec plugin for a ccloud context: the tunnel, then your OIDC token",
		Hidden: true,
	}, func(c *cobra.Command, region cloud.Region) error {
		return credentials(c, home, region)
	}))
	return clusters
}

func regionCmd(home cloud.Cloud, command *cobra.Command, run func(*cobra.Command, cloud.Region) error) *cobra.Command {
	var name string
	command.Flags().StringVar(&name, "region", "", "the "+home.Name+" region: "+strings.Join(home.Names(), ", "))
	command.Args = cobra.NoArgs
	command.RunE = func(c *cobra.Command, _ []string) error {
		if name == "" {
			return fmt.Errorf("which region? %s --region <region>, where <region> is one of %s", c.CommandPath(), strings.Join(home.Names(), ", "))
		}
		region, err := home.Find(name)
		if err != nil {
			return err
		}
		return run(c, region)
	}
	return command
}
