package cmd

import (
	"bufio"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/rest"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
	"github.com/cloudwalksolutions/ccloud/internal/workflows"
)

func dbResetCmd(home cloud.Cloud) *cobra.Command {
	reset := appCmd(home, &cobra.Command{
		Use:     "reset --env <env> [--app <app>]",
		Short:   "Drop an environment's database and migrate and seed it again from scratch",
		Example: "  ccloud db reset -e feat-sports-and-divisions",
	}, namedEnv, func(c *cobra.Command, region cloud.Region, app string) error {
		return dbReset(c, home, region, app)
	})
	reset.Flags().StringP("env", "e", "", "the environment: a feature environment's name; stage for admins")
	reset.Flags().Bool("dangerously-reset", false, "reset without asking you to confirm")
	return reset
}

func dbReset(c *cobra.Command, home cloud.Cloud, region cloud.Region, app string) error {
	env, _ := c.Flags().GetString("env")
	if unconfirmed, _ := c.Flags().GetBool("dangerously-reset"); !unconfirmed {
		if err := confirmReset(c, region, app, env); err != nil {
			return err
		}
	}
	runs, err := connect(region, func(cfg *rest.Config) (workflows.Runs, error) { return workflows.In(cfg, env+"-"+app) })
	if err != nil {
		return err
	}
	name, err := runs.Submit(c.Context(), home.Database.Reset)
	var run workflows.Run
	if err == nil {
		run, err = getRun(c, runs, region, app, name)
	}
	if err == nil {
		err = follow(c, region, app, runs, run, 3*time.Second, true)
	}
	if apierrors.IsForbidden(err) {
		err = fmt.Errorf("you may not reset %s's %s database in %s", app, env, region.Name)
	}
	return err
}

func confirmReset(c *cobra.Command, region cloud.Region, app, env string) error {
	if !terminal(c.InOrStdin()) {
		return fmt.Errorf("resetting drops %s's %s database in %s; pass --dangerously-reset to reset it without confirming", app, env, region.Name)
	}
	fmt.Fprintf(c.OutOrStdout(), "This drops every table in %s's %s database in %s, then migrates and seeds it from scratch.\nType %s to reset it: ", app, env, region.Name, env)
	typed, _ := bufio.NewReader(c.InOrStdin()).ReadString('\n')
	typed = strings.TrimSpace(typed)
	if typed != env {
		return fmt.Errorf("you typed %q, not %q; nothing was reset", typed, env)
	}
	return nil
}
