package cmd

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/cloudwalksolutions/ccloud/internal/workflows"
)

func (c command) stepLogs(ctx context.Context, run workflows.Run) error {
	kept := &logReader{command: c}
	onlyFailed, _ := c.Flags().GetBool("failed")
	for _, step := range run.Steps() {
		if step.StartedAt.IsZero() || (onlyFailed && !failed(step.Phase)) {
			continue
		}
		text, err := c.runs.StepLog(ctx, run, step)
		if apierrors.IsNotFound(err) {
			text, err = kept.stepLog(ctx, run, step)
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

func (r *logReader) stepLog(ctx context.Context, run workflows.Run, step workflows.Step) (string, error) {
	entries, err := r.read(ctx, url.Values{
		"namespace": {r.ci()},
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
