package cmd

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
	"github.com/cloudwalksolutions/ccloud/internal/workflows"
)

func workflowLogs(c *cobra.Command, home cloud.Cloud, region cloud.Region, app string, runs workflows.Runs, run workflows.Run) error {
	kept := &logReader{c: c, home: home, region: region}
	onlyFailed, _ := c.Flags().GetBool("failed")
	for _, step := range run.Steps() {
		if step.StartedAt.IsZero() || (onlyFailed && !failed(step.Phase)) {
			continue
		}
		text, err := runs.StepLog(c.Context(), run, step)
		if apierrors.IsNotFound(err) {
			text, err = keptLog(kept, app, run, step)
		}
		if err != nil {
			return err
		}
		for line := range strings.Lines(text) {
			fmt.Fprintf(c.OutOrStdout(), "%s\t%s\n", step.DisplayName, strings.TrimSuffix(line, "\n"))
		}
	}
	return nil
}

func failed(phase string) bool {
	return phase == "Failed" || phase == "Error"
}

func keptLog(kept *logReader, app string, run workflows.Run, step workflows.Step) (string, error) {
	entries, err := kept.read(url.Values{
		"namespace": {"ci-" + app},
		"pod":       {run.PodName(step)},
		"container": {"main"},
		"start":     {step.StartedAt.UTC().Format(time.RFC3339)},
		"end":       {step.FinishedAt.Add(time.Second).UTC().Format(time.RFC3339)},
		"direction": {"forward"},
		"limit":     {"5000"},
	})
	var text strings.Builder
	for _, entry := range entries {
		text.WriteString(entry.Line + "\n")
	}
	return text.String(), err
}
