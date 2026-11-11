package cmd

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/cloudwalksolutions/ccloud/internal/workflows"
)

func (c command) cancel(ctx context.Context, run workflows.Run) error {
	if !run.Status.FinishedAt.IsZero() {
		return fmt.Errorf("%s already finished: %s", run.Name(), run.Status.Phase)
	}
	if err := c.runs.Cancel(ctx, run.Name()); err != nil {
		return c.mayNotChange(err)
	}
	_, err := fmt.Fprintf(c.OutOrStdout(), "Cancelling %s. Follow it with: ccloud workflow watch %s -a %s -r %s\n", run.Name(), run.Name(), c.app, c.region.Name)
	return err
}

func (c command) rerun(ctx context.Context, run workflows.Run) error {
	started, err := c.runs.Rerun(ctx, run)
	if err != nil {
		return c.mayNotChange(err)
	}
	_, err = fmt.Fprintf(c.OutOrStdout(), "Started %s, a rerun of %s. Follow it with: ccloud workflow watch %s -a %s -r %s\n", started, run.Name(), started, c.app, c.region.Name)
	return err
}

func (c command) mayNotChange(err error) error {
	if apierrors.IsForbidden(err) {
		err = fmt.Errorf("you may not cancel or rerun %s's runs in %s (%s)", c.app, c.region.Name, c.ci())
	}
	return err
}
