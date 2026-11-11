package cmd

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
)

func Run(args []string, stdout, stderr io.Writer, home cloud.Cloud, version string) error {
	return RunContext(context.Background(), args, stdout, stderr, home, version)
}

func RunContext(ctx context.Context, args []string, stdout, stderr io.Writer, home cloud.Cloud, version string) error {
	root := &cobra.Command{
		Use:           "ccloud",
		Version:       version,
		Short:         fmt.Sprintf("The developer CLI for your apps in the %s cloud", home.Name),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(clustersCmd(home), workflowCmd(home), logsCmd(home))
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	return root.ExecuteContext(ctx)
}
