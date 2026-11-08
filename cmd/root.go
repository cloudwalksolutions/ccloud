package cmd

import (
	"io"

	"github.com/spf13/cobra"
)

func Run(args []string, stdout, stderr io.Writer) error {
	root := &cobra.Command{
		Use:           "ccloud",
		Short:         "Your apps in the cloudwalk datacenter",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(clustersCmd())
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	return root.Execute()
}
