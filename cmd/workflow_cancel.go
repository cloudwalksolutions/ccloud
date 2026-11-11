package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
	"github.com/cloudwalksolutions/ccloud/internal/workflows"
)

func workflowCancel(c *cobra.Command, region cloud.Region, app string, runs workflows.Runs, run workflows.Run) error {
	if !run.Status.FinishedAt.IsZero() {
		return fmt.Errorf("%s already finished: %s", run.Name(), run.Status.Phase)
	}
	if err := runs.Cancel(c.Context(), run.Name()); err != nil {
		return errMayNotChange(err, region, app)
	}
	_, err := fmt.Fprintf(c.OutOrStdout(), "Cancelling %s. Follow it with: ccloud workflow watch %s -a %s -r %s\n", run.Name(), run.Name(), app, region.Name)
	return err
}

func errMayNotChange(err error, region cloud.Region, app string) error {
	if apierrors.IsForbidden(err) {
		err = fmt.Errorf("you may not cancel or rerun %s's runs in %s (ci-%s)", app, region.Name, app)
	}
	return err
}
