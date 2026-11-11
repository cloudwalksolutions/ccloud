package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudwalksolutions/ccloud/internal/workflows"
)

const clearScreen = "\x1b[H\x1b[2J"

func (c command) watch(ctx context.Context, run workflows.Run) error {
	interval, _ := c.Flags().GetDuration("interval")
	out := c.OutOrStdout()
	redraw := terminal(out)
	var err error
	for err == nil && run.Status.FinishedAt.IsZero() {
		if redraw {
			fmt.Fprint(out, clearScreen)
			render(out, run, time.Now())
			fmt.Fprintf(out, "\nLooking again every %s; Ctrl-C stops watching, not the run.\n", interval)
		}
		time.Sleep(interval)
		run, err = c.getRun(ctx)
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
