package cmd

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
)

func Run(args []string, stdout, stderr io.Writer, home cloud.Cloud) error {
	root := &cobra.Command{
		Use:           "ccloud",
		Short:         "Your apps in the cloudwalk datacenter",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(clustersCmd(home))
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	return root.Execute()
}
