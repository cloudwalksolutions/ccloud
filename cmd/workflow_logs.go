package cmd

import (
	"bufio"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
	"github.com/cloudwalksolutions/ccloud/internal/logs"
	"github.com/cloudwalksolutions/ccloud/internal/workflows"
)

func workflowLogs(c *cobra.Command, home cloud.Cloud, region cloud.Region, app string) error {
	runs, run, err := openRun(c, region, app)
	if err != nil {
		return err
	}
	namespace := "ci-" + app
	out := c.OutOrStdout()
	onlyFailed, _ := c.Flags().GetBool("failed")
	var gateway *logs.Gateway
	for _, step := range run.Steps() {
		if step.StartedAt.IsZero() || (onlyFailed && !failed(step.Phase)) {
			continue
		}
		stream, err := runs.StepLog(c.Context(), run, step)
		if apierrors.IsNotFound(err) {
			if gateway == nil {
				signedIn, err := signInToLogs(c, home, region)
				if err != nil {
					return err
				}
				gateway = &signedIn
			}
			if err := keptLog(c, *gateway, namespace, run, step); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		err = printStep(out, step, stream)
		_ = stream.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func failed(phase string) bool {
	return phase == "Failed" || phase == "Error"
}

func printStep(out io.Writer, step workflows.Step, stream io.Reader) error {
	lines := bufio.NewScanner(stream)
	for lines.Scan() {
		fmt.Fprintf(out, "%s\t%s\n", step.DisplayName, lines.Text())
	}
	return lines.Err()
}

func keptLog(c *cobra.Command, gateway logs.Gateway, namespace string, run workflows.Run, step workflows.Step) error {
	entries, err := gateway.Read(c.Context(), url.Values{
		"namespace": {namespace},
		"pod":       {run.PodName(step)},
		"container": {"main"},
		"start":     {step.StartedAt.UTC().Format(time.RFC3339)},
		"end":       {step.FinishedAt.Add(time.Second).UTC().Format(time.RFC3339)},
		"direction": {"forward"},
		"limit":     {"5000"},
	})
	if err != nil {
		return err
	}
	for _, entry := range entries {
		fmt.Fprintf(c.OutOrStdout(), "%s\t%s\n", step.DisplayName, entry.Line)
	}
	return nil
}
