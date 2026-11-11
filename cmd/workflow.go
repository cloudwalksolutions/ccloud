package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
)

func workflowCmd(home cloud.Cloud) *cobra.Command {
	workflow := &cobra.Command{
		Use:   "workflow",
		Short: "Your app's CI runs: list them and see their steps",
	}
	list := appCmd(home, &cobra.Command{
		Use:     "list --app <app> --region <region>",
		Short:   "List an app's recent CI runs, newest first",
		Example: "  ccloud workflow list -a <app> -r " + home.Regions[0].Name + " --pr 105",
	}, workflowList)
	list.Flags().String("flow", "", "only this flow, e.g. api-tests")
	list.Flags().String("pr", "", "only runs for this pull request number")
	list.Flags().IntP("limit", "L", 20, "at most this many runs")
	workflow.AddCommand(list)
	workflow.AddCommand(appCmd(home, &cobra.Command{
		Use:     "view <run> --app <app> --region <region>",
		Short:   "Show a run's steps, how long each took and why any failed",
		Example: "  ccloud workflow view api-tests-x7k2p -a <app> -r " + home.Regions[0].Name,
		Args: func(c *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf("which run? ccloud workflow view <run>, where <run> is an ID from ccloud workflow list")
			}
			return nil
		},
	}, workflowView))
	return workflow
}

func appCmd(home cloud.Cloud, command *cobra.Command, run func(*cobra.Command, cloud.Region, string) error) *cobra.Command {
	var app string
	command.Flags().StringVarP(&app, "app", "a", "", "the app, as it is named in its namespaces")
	return regionCmd(home, command, func(c *cobra.Command, region cloud.Region) error {
		if app == "" {
			return fmt.Errorf("which app? %s --app <app>", c.CommandPath())
		}
		return run(c, region, app)
	})
}
