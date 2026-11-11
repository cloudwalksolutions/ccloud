package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
	"github.com/cloudwalksolutions/ccloud/internal/kubeconfig"
	"github.com/cloudwalksolutions/ccloud/internal/workflows"
)

func workflowCmd(home cloud.Cloud) *cobra.Command {
	workflow := &cobra.Command{
		Use:   "workflow",
		Short: "Your app's CI runs: list them and see their steps",
	}
	list := ciCmd(home, &cobra.Command{
		Use:     "list --app <app> --region <region>",
		Short:   "List an app's recent CI runs, newest first",
		Example: "  ccloud workflow list -a <app> -r " + home.Regions[0].Name + " --pr 105",
	}, workflowList)
	list.Flags().String("flow", "", "only this flow, e.g. api-tests")
	list.Flags().String("pr", "", "only runs for this pull request number")
	list.Flags().IntP("limit", "L", 20, "at most this many runs")
	workflow.AddCommand(list)
	workflow.AddCommand(runCmd(home, &cobra.Command{
		Use:     "view <run> --app <app> --region <region>",
		Short:   "Show a run's steps, how long each took and why any failed",
		Example: "  ccloud workflow view api-tests-x7k2p -a <app> -r " + home.Regions[0].Name,
	}, workflowView))
	watch := runCmd(home, &cobra.Command{
		Use:     "watch <run> --app <app> --region <region>",
		Short:   "Follow a run's steps until it finishes",
		Example: "  ccloud workflow watch api-tests-x7k2p -a <app> -r " + home.Regions[0].Name,
	}, workflowWatch)
	watch.Flags().DurationP("interval", "i", 3*time.Second, "how often to look again")
	watch.Flags().Bool("exit-status", false, "exit non-zero when the run does not succeed")
	workflow.AddCommand(watch)
	stepLogs := runCmd(home, &cobra.Command{
		Use:     "logs <run> --app <app> --region <region>",
		Short:   "Print each step's log, named for its step, long after its pods are gone",
		Example: "  ccloud workflow logs api-tests-x7k2p -a <app> -r " + home.Regions[0].Name + " --failed",
	}, func(c *cobra.Command, region cloud.Region, app string, runs workflows.Runs, run workflows.Run) error {
		return workflowLogs(c, home, region, app, runs, run)
	})
	stepLogs.Flags().Bool("failed", false, "only the steps that failed")
	workflow.AddCommand(stepLogs)
	workflow.AddCommand(runCmd(home, &cobra.Command{
		Use:     "cancel <run> --app <app> --region <region>",
		Short:   "Stop a run still going; its exit handlers still report",
		Example: "  ccloud workflow cancel api-tests-x7k2p -a <app> -r " + home.Regions[0].Name,
	}, workflowCancel))
	workflow.AddCommand(runCmd(home, &cobra.Command{
		Use:     "rerun <run> --app <app> --region <region>",
		Short:   "Start the run again from the beginning, for the same pull request and commit",
		Example: "  ccloud workflow rerun api-tests-x7k2p -a <app> -r " + home.Regions[0].Name,
	}, workflowRerun))
	return workflow
}

func ciCmd(home cloud.Cloud, command *cobra.Command, action func(*cobra.Command, cloud.Region, string, workflows.Runs) error) *cobra.Command {
	return appCmd(home, command, func(c *cobra.Command, region cloud.Region, app string) error {
		cfg, err := kubeconfig.REST(kubeconfig.Path(), region.Context())
		if err != nil {
			return signInError(err, region)
		}
		runs, err := workflows.In(cfg, "ci-"+app)
		if err != nil {
			return err
		}
		return action(c, region, app, runs)
	})
}

type runAction func(c *cobra.Command, region cloud.Region, app string, runs workflows.Runs, run workflows.Run) error

func runCmd(home cloud.Cloud, command *cobra.Command, action runAction) *cobra.Command {
	command.Args = oneRun
	return ciCmd(home, command, func(c *cobra.Command, region cloud.Region, app string, runs workflows.Runs) error {
		run, err := getRun(c, runs, region, app)
		if err != nil {
			return err
		}
		return action(c, region, app, runs, run)
	})
}

func getRun(c *cobra.Command, runs workflows.Runs, region cloud.Region, app string) (workflows.Run, error) {
	name := c.Flags().Arg(0)
	run, err := runs.Get(c.Context(), name)
	if apierrors.IsNotFound(err) {
		return run, fmt.Errorf("%s has no run %s in %s; see ccloud workflow list -a %s -r %s", app, name, region.Name, app, region.Name)
	}
	return run, err
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
