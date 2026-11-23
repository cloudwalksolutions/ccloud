package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/rest"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
	"github.com/cloudwalksolutions/ccloud/internal/workflows"
)

func workflowCmd(home cloud.Cloud) *cobra.Command {
	workflow := &cobra.Command{
		Use:   "workflow",
		Short: "Your app's CI runs: list them and see their steps",
	}
	list := ciCmd(home, &cobra.Command{
		Use:     "list [--app <app>]",
		Short:   "List an app's recent CI runs, newest first",
		Example: "  ccloud workflow list --pr 105",
	}, workflowList)
	list.Flags().String("flow", "", "only this flow, e.g. api-tests")
	list.Flags().String("pr", "", "only runs for this pull request number")
	list.Flags().IntP("limit", "L", 20, "at most this many runs")
	workflow.AddCommand(list)
	workflow.AddCommand(runCmd(home, &cobra.Command{
		Use:     "view <run> [--app <app>]",
		Short:   "Show a run's steps, how long each took and why any failed",
		Example: "  ccloud workflow view api-tests-x7k2p",
	}, workflowView))
	watch := runCmd(home, &cobra.Command{
		Use:     "watch <run> [--app <app>]",
		Short:   "Follow a run's steps until it finishes",
		Example: "  ccloud workflow watch api-tests-x7k2p --exit-status",
	}, workflowWatch)
	watch.Flags().DurationP("interval", "i", 3*time.Second, "how often to look again")
	watch.Flags().Bool("exit-status", false, "exit non-zero when the run does not succeed")
	workflow.AddCommand(watch)
	stepLogs := runCmd(home, &cobra.Command{
		Use:     "logs <run> [--app <app>]",
		Short:   "Print each step's log, named for its step, long after its pods are gone",
		Example: "  ccloud workflow logs api-tests-x7k2p --failed",
	}, func(c *cobra.Command, region cloud.Region, app string, runs workflows.Runs, run workflows.Run) error {
		return workflowLogs(c, home, region, app, runs, run)
	})
	stepLogs.Flags().Bool("failed", false, "only the steps that failed")
	workflow.AddCommand(stepLogs)
	workflow.AddCommand(runCmd(home, &cobra.Command{
		Use:     "cancel <run> [--app <app>]",
		Short:   "Stop a run still going; its exit handlers still report",
		Example: "  ccloud workflow cancel api-tests-x7k2p",
	}, workflowCancel))
	workflow.AddCommand(runCmd(home, &cobra.Command{
		Use:     "rerun <run> [--app <app>]",
		Short:   "Start the run again from the beginning, for the same pull request and commit",
		Example: "  ccloud workflow rerun api-tests-x7k2p",
	}, workflowRerun))
	return workflow
}

func ciCmd(home cloud.Cloud, command *cobra.Command, action func(*cobra.Command, cloud.Region, string, workflows.Runs) error) *cobra.Command {
	return appCmd(home, command, ciEnv, func(c *cobra.Command, region cloud.Region, app string) error {
		runs, err := connect(region, func(cfg *rest.Config) (workflows.Runs, error) { return workflows.In(cfg, "ci-"+app) })
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
		name := c.Flags().Arg(0)
		done := step(c, fmt.Sprintf("Reading %s in %s", name, region.Name))
		run, err := getRun(c, runs, region, app, name)
		done(err, fmt.Sprintf("Read %s", name))
		if err != nil {
			return err
		}
		return action(c, region, app, runs, run)
	})
}

func getRun(c *cobra.Command, runs workflows.Runs, region cloud.Region, app, name string) (workflows.Run, error) {
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

func failed(phase string) bool {
	return phase == "Failed" || phase == "Error"
}
