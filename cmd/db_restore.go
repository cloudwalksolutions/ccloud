package cmd

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
	"github.com/cloudwalksolutions/ccloud/internal/workflows"
)

func dbRestoreCmd(home cloud.Cloud) *cobra.Command {
	restore := appCmd(home, &cobra.Command{
		Use:     "restore --env <env> [--app <app>] [--backup <label>] [--from <region>]",
		Short:   "Replace an environment's database with one of its backups, the newest unless you name one",
		Example: "  ccloud db restore -e stage\n  ccloud db restore -e stage --backup 20261126-175333F\n  ccloud db restore -e stage --from south1",
	}, namedEnv, func(c *cobra.Command, region cloud.Region, app string) error {
		return dbRestore(c, home, region, app)
	})
	restore.Flags().StringP("env", "e", "", "the environment: stage or prod, for admins")
	restore.Flags().String("backup", "", "the label of the backup to restore, as ccloud db backups lists it")
	restore.Flags().String("from", "", "restore a backup another region copied off-site, such as when moving the environment here")
	restore.Flags().Bool("dangerously-restore", false, "restore without asking you to confirm")
	return restore
}

func dbRestore(c *cobra.Command, home cloud.Cloud, region cloud.Region, app string) error {
	env, _ := c.Flags().GetString("env")
	err := restore(c, home, region, app, env)
	if apierrors.IsForbidden(err) {
		err = fmt.Errorf("you may not restore %s's %s database in %s", app, env, region.Name)
	}
	return err
}

func restore(c *cobra.Command, home cloud.Cloud, region cloud.Region, app, env string) error {
	unconfirmed, _ := c.Flags().GetBool("dangerously-restore")
	if !unconfirmed && !terminal(c.InOrStdin()) {
		return fmt.Errorf("restoring replaces %s's %s database in %s; pass --dangerously-restore to restore it without confirming", app, env, region.Name)
	}
	if env == "prod" {
		cautionAboutProd(c.ErrOrStderr(), app)
	}
	label, _ := c.Flags().GetString("backup")
	from, _ := c.Flags().GetString("from")
	if label == "" {
		_, backups, err := backupsFrom(c, home, region, app, env, from)
		if err != nil {
			return err
		}
		if len(backups) == 0 {
			return fmt.Errorf("%s's %s database in %s has no backups to restore; ccloud db backup takes one", app, env, region.Name)
		}
		label = backups[len(backups)-1].Label
	}
	if !unconfirmed {
		if err := typeToConfirm(c, env, "restore", "restored", fmt.Sprintf("This replaces %s's %s database in %s with backup %s; whatever was written since then is lost.", app, env, region.Name, label)); err != nil {
			return err
		}
	}
	parameters := []workflows.Parameter{{Name: "backup", Value: label}}
	taken := ""
	if from != "" {
		parameters = append(parameters, workflows.Parameter{Name: "from", Value: from})
		taken = ", taken in " + from
	}
	err := runWorkflow(c, c.OutOrStdout(), region, app, env+"-"+app, home.Database.Restore,
		"following it until the app is back on the restored database. Ctrl-C stops watching, not the restore", parameters...)
	if err == nil {
		_, err = fmt.Fprintf(c.OutOrStdout(), "Restored %s's %s database in %s from backup %s%s.\n", app, env, region.Name, label, taken)
	}
	return err
}

func typeToConfirm(c *cobra.Command, env, verb, done, consequence string) error {
	fmt.Fprintf(c.OutOrStdout(), "%s\nType %s to %s it: ", consequence, env, verb)
	typed, _ := bufio.NewReader(c.InOrStdin()).ReadString('\n')
	typed = strings.TrimSpace(typed)
	if typed != env {
		return fmt.Errorf("you typed %q, not %q; nothing was %s", typed, env, done)
	}
	return nil
}
