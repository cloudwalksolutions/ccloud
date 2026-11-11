package cmd

import (
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
	"github.com/cloudwalksolutions/ccloud/internal/workflows"
)

func workflowList(c *cobra.Command, region cloud.Region, app string, ci workflows.Runs) error {
	selector := labels.Set{}
	for _, flag := range []string{"flow", "pr"} {
		if value, _ := c.Flags().GetString(flag); value != "" {
			selector[flag] = value
		}
	}
	limit, _ := c.Flags().GetInt("limit")
	namespace := "ci-" + app
	runs, err := ci.List(c.Context(), selector.String())
	if apierrors.IsForbidden(err) {
		err = fmt.Errorf("you may not read %s's runs in %s (%s)", app, region.Name, namespace)
	}
	if err != nil {
		return err
	}
	if len(runs) == 0 {
		_, err := fmt.Fprintf(c.OutOrStdout(), "No runs for %s in %s.\n", app, region.Name)
		return err
	}
	now := time.Now()
	colour := paletteFor(c.OutOrStdout())
	table := tabwriter.NewWriter(c.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, colour.paint("01", "STATUS")+"\tFLOW\tPR\tBRANCH\tSHA\tID\tELAPSED\tAGE")
	for _, run := range runs[:min(len(runs), limit)] {
		pr := ""
		if run.PR() != "" {
			pr = "#" + run.PR()
		}
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			colour.status(run.Status.Phase), run.Flow(), pr, run.Branch(), short(run.SHA()), run.Name(),
			run.Elapsed(now).Round(time.Second), age(now.Sub(run.Metadata.CreationTimestamp)))
	}
	return table.Flush()
}

func symbol(phase string) string {
	switch phase {
	case "Succeeded":
		return "✓"
	case "Failed", "Error":
		return "X"
	case "Skipped", "Omitted":
		return "-"
	default:
		return "*"
	}
}

func short(sha string) string {
	return sha[:min(len(sha), 7)]
}

func age(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
