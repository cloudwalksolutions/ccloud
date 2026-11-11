package cmd

import (
	"fmt"
	"time"

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
		Args:    oneRun,
	}, workflowView))
	watch := appCmd(home, &cobra.Command{
		Use:     "watch <run> --app <app> --region <region>",
		Short:   "Follow a run's steps until it finishes",
		Example: "  ccloud workflow watch api-tests-x7k2p -a <app> -r " + home.Regions[0].Name,
		Args:    oneRun,
	}, workflowWatch)
	watch.Flags().DurationP("interval", "i", 3*time.Second, "how often to look again")
	watch.Flags().Bool("exit-status", false, "exit non-zero when the run does not succeed")
	workflow.AddCommand(watch)
	stepLogs := appCmd(home, &cobra.Command{
		Use:     "logs <run> --app <app> --region <region>",
		Short:   "Print each step's log, named for its step, long after its pods are gone",
		Example: "  ccloud workflow logs api-tests-x7k2p -a <app> -r " + home.Regions[0].Name + " --failed",
		Args:    oneRun,
	}, func(c *cobra.Command, region cloud.Region, app string) error {
		return workflowLogs(c, home, region, app)
	})
	stepLogs.Flags().Bool("failed", false, "only the steps that failed")
	workflow.AddCommand(stepLogs)
	return workflow
}

func oneRun(c *cobra.Command, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("which run? %s <run>, where <run> is an ID from ccloud workflow list", c.CommandPath())
	}
	return nil
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
