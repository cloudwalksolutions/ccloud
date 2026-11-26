package cmd

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
)

func dbResetCmd(home cloud.Cloud) *cobra.Command {
	reset := appCmd(home, &cobra.Command{
		Use:     "reset --env <env> [--app <app>]",
		Short:   "Drop an environment's database and migrate and seed it again from scratch",
		Example: "  ccloud db reset -e feat-new-checkout",
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
	err := runWorkflow(c, c.OutOrStdout(), region, app, env+"-"+app, home.Database.Reset,
		"following it until the database is migrated and seeded again. Ctrl-C stops watching, not the reset")
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
