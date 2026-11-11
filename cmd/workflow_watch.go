package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
)

const clearScreen = "\x1b[H\x1b[2J"

func workflowWatch(c *cobra.Command, region cloud.Region, app string) error {
	cfg, err := signedInTo(region)
	if err != nil {
		return err
	}
	interval, err := c.Flags().GetDuration("interval")
	if err != nil {
		return err
	}
	out := c.OutOrStdout()
	redraw := terminal(out)
	run, err := getRun(c, cfg, region, app)
	for err == nil && run.Status.FinishedAt.IsZero() {
		if redraw {
			fmt.Fprint(out, clearScreen)
			render(out, run, time.Now())
			fmt.Fprintf(out, "\nLooking again every %s; Ctrl-C stops watching, not the run.\n", interval)
		}
		time.Sleep(interval)
		run, err = getRun(c, cfg, region, app)
	}
	if err != nil {
		return err
	}
	if redraw {
		fmt.Fprint(out, clearScreen)
	}
	render(out, run, time.Now())
	fmt.Fprintf(out, "\n%s %s completed: %s\n", paletteFor(out).status(run.Status.Phase), run.Name(), run.Status.Phase)
	if strict, _ := c.Flags().GetBool("exit-status"); strict && run.Status.Phase != "Succeeded" {
		return fmt.Errorf("%s did not succeed: %s", run.Name(), run.Status.Phase)
	}
	return nil
}
