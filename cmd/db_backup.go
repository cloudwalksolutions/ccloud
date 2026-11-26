package cmd

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/rest"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
	"github.com/cloudwalksolutions/ccloud/internal/database"
	"github.com/cloudwalksolutions/ccloud/internal/workflows"
)

func dbBackupCmd(home cloud.Cloud) *cobra.Command {
	backup := appCmd(home, &cobra.Command{
		Use:     "backup --env <env> [--app <app>] [--note <text>]",
		Short:   "Back an environment's database up now, in full, and copy it off-site",
		Example: "  ccloud db backup -e stage --note \"before the migration\"",
	}, namedEnv, func(c *cobra.Command, region cloud.Region, app string) error {
		return dbBackup(c, home, region, app)
	})
	backup.Flags().StringP("env", "e", "", "the environment: stage or prod, for admins")
	backup.Flags().String("note", "", "a note kept with the backup, shown when you list backups")
	return backup
}

func dbBackup(c *cobra.Command, home cloud.Cloud, region cloud.Region, app string) error {
	env, _ := c.Flags().GetString("env")
	note, _ := c.Flags().GetString("note")
	if strings.ContainsAny(note, "\"\\\n\r") {
		return fmt.Errorf("a note may not hold quotes, backslashes or line breaks: %q", note)
	}
	if env == "prod" {
		cautionAboutProd(c.ErrOrStderr(), app)
	}
	runs, err := connect(region, func(cfg *rest.Config) (workflows.Runs, error) { return workflows.In(cfg, env+"-"+app) })
	var name string
	if err == nil {
		name, err = runs.Submit(c.Context(), home.Database.Backup, workflows.Parameter{Name: "note", Value: note})
	}
	var run workflows.Run
	if err == nil {
		fmt.Fprintf(c.ErrOrStderr(), "Submitted %s; following it until the backup is off-site. Ctrl-C stops watching, not the backup.\n", name)
		run, err = getRun(c, runs, region, app, name)
	}
	if err == nil {
		err = follow(c, region, app, runs, run, 3*time.Second, true)
	}
	if apierrors.IsForbidden(err) {
		err = fmt.Errorf("you may not back up %s's %s database in %s", app, env, region.Name)
	}
	if err != nil {
		return err
	}
	backups, err := backupsOf(c, home, region, app, env)
	if err != nil {
		return err
	}
	fmt.Fprintf(c.OutOrStdout(), "Backed up %s's %s database in %s, and copied it off-site:\n", app, env, region.Name)
	return printBackup(c.OutOrStdout(), backups[len(backups)-1])
}

func backupsOf(c *cobra.Command, home cloud.Cloud, region cloud.Region, app, env string, options ...string) ([]database.Backup, error) {
	db, err := connect(region, func(cfg *rest.Config) (database.Database, error) { return database.In(cfg, env+"-"+app, home.Database) })
	var pod string
	if err == nil {
		pod, err = db.Primary(c.Context())
	}
	if err != nil {
		return nil, fmt.Errorf("%s's %s database has %w in %s", app, env, err, region.Name)
	}
	return db.Backups(c.Context(), pod, options...)
}

func printBackup(out io.Writer, backup database.Backup) error {
	table := newTable(out)
	table.row("  label", backup.Label)
	table.row("  finished", age(time.Since(backup.Finished)))
	table.row("  size", memory(float64(backup.Stored)))
	table.row("  note", backup.Note)
	return table.print()
}
