package cmd

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
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
	}, command.list)
	list.Flags().String("flow", "", "only this flow, e.g. api-tests")
	list.Flags().String("pr", "", "only runs for this pull request number")
	list.Flags().IntP("limit", "L", 20, "at most this many runs")
	workflow.AddCommand(list)
	workflow.AddCommand(runCmd(home, &cobra.Command{
		Use:     "view <run> --app <app> --region <region>",
		Short:   "Show a run's steps, how long each took and why any failed",
		Example: "  ccloud workflow view api-tests-x7k2p -a <app> -r " + home.Regions[0].Name,
	}, command.view))
	watch := runCmd(home, &cobra.Command{
		Use:     "watch <run> --app <app> --region <region>",
		Short:   "Follow a run's steps until it finishes",
		Example: "  ccloud workflow watch api-tests-x7k2p -a <app> -r " + home.Regions[0].Name,
	}, command.watch)
	watch.Flags().DurationP("interval", "i", 3*time.Second, "how often to look again")
	watch.Flags().Bool("exit-status", false, "exit non-zero when the run does not succeed")
	workflow.AddCommand(watch)
	stepLogs := runCmd(home, &cobra.Command{
		Use:     "logs <run> --app <app> --region <region>",
		Short:   "Print each step's log, named for its step, long after its pods are gone",
		Example: "  ccloud workflow logs api-tests-x7k2p -a <app> -r " + home.Regions[0].Name + " --failed",
	}, command.stepLogs)
	stepLogs.Flags().Bool("failed", false, "only the steps that failed")
	workflow.AddCommand(stepLogs)
	workflow.AddCommand(runCmd(home, &cobra.Command{
		Use:     "cancel <run> --app <app> --region <region>",
		Short:   "Stop a run still going; its exit handlers still report",
		Example: "  ccloud workflow cancel api-tests-x7k2p -a <app> -r " + home.Regions[0].Name,
	}, command.cancel))
	workflow.AddCommand(runCmd(home, &cobra.Command{
		Use:     "rerun <run> --app <app> --region <region>",
		Short:   "Start the run again from the beginning, for the same pull request and commit",
		Example: "  ccloud workflow rerun api-tests-x7k2p -a <app> -r " + home.Regions[0].Name,
	}, command.rerun))
	return workflow
}
