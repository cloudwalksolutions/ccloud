package cmd

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
	"github.com/cloudwalksolutions/ccloud/internal/workflows"
)

func workflowView(c *cobra.Command, region cloud.Region, app string, _ workflows.Runs, run workflows.Run) error {
	render(c.OutOrStdout(), run, time.Now())
	if failed(run.Status.Phase) {
		fmt.Fprintf(c.OutOrStdout(), "\nTo see what failed, try: ccloud workflow logs %s -a %s -r %s --failed\n", run.Name(), app, region.Name)
	}
	return nil
}

func render(out io.Writer, run workflows.Run, now time.Time) {
	colour := paletteFor(out)
	title := []string{colour.status(run.Status.Phase) + " " + run.Flow()}
	if run.PR() != "" {
		title = append(title, "#"+run.PR())
	}
	for _, part := range []string{run.Branch(), short(run.SHA())} {
		if part != "" {
			title = append(title, part)
		}
	}
	fmt.Fprintln(out, strings.Join(title, " · "))
	verb := "took"
	if run.Status.FinishedAt.IsZero() {
		verb = "has run for"
	}
	fmt.Fprintf(out, "%s started %s and %s %s\n", run.Name(), age(now.Sub(run.Status.StartedAt)), verb, run.Elapsed(now).Round(time.Second))
	for i, step := range run.Steps() {
		if i == 0 {
			fmt.Fprintln(out)
		}
		line := colour.status(step.Phase) + " " + step.DisplayName
		if step.Took() > 0 {
			line += " in " + step.Took().Round(time.Second).String()
		}
		fmt.Fprintln(out, line)
		if step.Phase == "Failed" || step.Phase == "Error" {
			fmt.Fprintln(out, "  "+step.Message)
		}
	}
}
