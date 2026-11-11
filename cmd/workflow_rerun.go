package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
	"github.com/cloudwalksolutions/ccloud/internal/workflows"
)

func workflowRerun(c *cobra.Command, region cloud.Region, app string, runs workflows.Runs, run workflows.Run) error {
	started, err := runs.Rerun(c.Context(), run)
	if err != nil {
		return errMayNotChange(err, region, app)
	}
	_, err = fmt.Fprintf(c.OutOrStdout(), "Started %s, a rerun of %s. Follow it with: ccloud workflow watch %s -a %s -r %s\n", started, run.Name(), started, app, region.Name)
	return err
}
