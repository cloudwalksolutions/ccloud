package cmd

import (
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
	}, command.auth))
	set := regionCmd(home, &cobra.Command{
		Use:     "set-context --region <region> [--namespace <namespace>]",
		Short:   "Switch kubectl to a region you have signed in to",
		Example: "  ccloud clusters set-context --region " + first + " --namespace <namespace>",
	}, command.setContext)
	set.Flags().String("namespace", "", "the namespace kubectl uses there; left as it was when omitted")
	clusters.AddCommand(set)
	clusters.AddCommand(regionCmd(home, &cobra.Command{
		Use:    "credentials --region <region>",
		Short:  "kubectl's exec plugin for a ccloud context: the tunnel, then your OIDC token",
		Hidden: true,
	}, command.credentials))
	return clusters
}
