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
	for _, ran := range run.Steps() {
		if ran.StartedAt.IsZero() || (onlyFailed && !failed(ran.Phase)) {
			continue
		}
		text, err := stepLog(c, kept, app, runs, run, ran)
		if err != nil {
			return err
		}
		for line := range strings.Lines(text) {
			fmt.Fprintf(c.OutOrStdout(), "%s\t%s\n", ran.DisplayName, strings.TrimSuffix(line, "\n"))
		}
	}
	return nil
}

func stepLog(c *cobra.Command, kept *logReader, app string, runs workflows.Runs, run workflows.Run, ran workflows.Step) (string, error) {
	done := step(c, fmt.Sprintf("Reading %s's log", ran.DisplayName))
	text, err := runs.StepLog(c.Context(), run, ran)
	if apierrors.IsNotFound(err) {
		done(nil, ran.DisplayName+"'s pod is gone")
		done = step(c, fmt.Sprintf("Reading %s's kept log from %s's gateway", ran.DisplayName, kept.region.Name))
		text, err = keptLog(kept, app, run, ran)
	}
	done(err, fmt.Sprintf("Read %s's log", ran.DisplayName))
	return text, err
}

func failed(phase string) bool {
	return phase == "Failed" || phase == "Error"
}

func keptLog(kept *logReader, app string, run workflows.Run, step workflows.Step) (string, error) {
	page, err := kept.read(url.Values{
		"namespace": {"ci-" + app},
		"pod":       {run.PodName(step)},
		"container": {"main"},
		"start":     {step.StartedAt.UTC().Format(time.RFC3339)},
		"end":       {step.FinishedAt.Add(time.Second).UTC().Format(time.RFC3339)},
		"direction": {"forward"},
		"limit":     {"5000"},
	})
	var text strings.Builder
	for _, entry := range page.Entries {
		text.WriteString(entry.Line + "\n")
	}
	return text.String(), err
}
