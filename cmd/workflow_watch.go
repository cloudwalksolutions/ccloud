package cmd

import (
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
	"github.com/cloudwalksolutions/ccloud/internal/workflows"
)

const clearScreen = "\x1b[H\x1b[2J"

func workflowWatch(c *cobra.Command, region cloud.Region, app string, runs workflows.Runs, run workflows.Run) error {
	interval, _ := c.Flags().GetDuration("interval")
	strict, _ := c.Flags().GetBool("exit-status")
	return follow(c, region, app, runs, run, interval, strict)
}

func printChanges(out io.Writer, run workflows.Run, phases map[string]string) {
	for _, step := range run.Steps() {
		if phases[step.ID] != step.Phase {
			phases[step.ID] = step.Phase
			fmt.Fprintf(out, "%s %s %s\n", time.Now().Format(time.TimeOnly), step.DisplayName, step.Phase)
		}
	}
}

func follow(c *cobra.Command, region cloud.Region, app string, runs workflows.Runs, run workflows.Run, interval time.Duration, strict bool) error {
	out := c.OutOrStdout()
	redraw := terminal(out)
	phases := map[string]string{}
	var err error
	for err == nil && run.Status.FinishedAt.IsZero() {
		if redraw {
			fmt.Fprint(out, clearScreen)
			render(out, run, time.Now())
			fmt.Fprintf(out, "\nLooking again every %s; Ctrl-C stops watching, not the run.\n", interval)
		} else {
			printChanges(c.ErrOrStderr(), run, phases)
		}
		select {
		case <-c.Context().Done():
			return nil
		case <-time.After(interval):
		}
		run, err = getRun(c, runs, region, app, run.Name())
	}
	if err != nil {
		return err
	}
	if !redraw {
		printChanges(c.ErrOrStderr(), run, phases)
	}
	if redraw {
		fmt.Fprint(out, clearScreen)
	}
	render(out, run, time.Now())
	fmt.Fprintf(out, "\n%s %s completed: %s\n", paletteFor(out).status(run.Status.Phase), run.Name(), run.Status.Phase)
	if strict && run.Status.Phase != "Succeeded" {
		return fmt.Errorf("%s did not succeed: %s", run.Name(), run.Status.Phase)
	}
	return nil
}
