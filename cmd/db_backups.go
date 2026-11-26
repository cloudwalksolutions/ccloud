package cmd

import (
	"fmt"
	"slices"
	"time"

	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/rest"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
	"github.com/cloudwalksolutions/ccloud/internal/database"
	"github.com/cloudwalksolutions/ccloud/internal/workflows"
)

func dbBackupsCmd(home cloud.Cloud) *cobra.Command {
	backups := appCmd(home, &cobra.Command{
		Use:     "backups --env <env> [--app <app>]",
		Short:   "List an environment's database backups, newest first",
		Example: "  ccloud db backups -e stage",
	}, namedEnv, func(c *cobra.Command, region cloud.Region, app string) error {
		return dbBackups(c, home, region, app)
	})
	backups.Flags().StringP("env", "e", "", "the environment: stage or prod, for admins")
	backups.Flags().String("from", "", "list the backups another region copied off-site, such as before moving the environment here")
	return backups
}

func dbBackups(c *cobra.Command, home cloud.Cloud, region cloud.Region, app string) error {
	env, _ := c.Flags().GetString("env")
	err := listBackups(c, home, region, app, env)
	if apierrors.IsForbidden(err) {
		err = fmt.Errorf("you may not read %s's %s backups in %s", app, env, region.Name)
	}
	return err
}

func listBackups(c *cobra.Command, home cloud.Cloud, region cloud.Region, app, env string) error {
	from, _ := c.Flags().GetString("from")
	namespace := env + "-" + app
	db, backups, err := backupsFrom(c, home, region, app, env, from)
	if err != nil {
		return err
	}
	if len(backups) == 0 {
		_, err = fmt.Fprintf(c.OutOrStdout(), "%s's %s database in %s has no backups yet; ccloud db backup takes one.\n", app, env, region.Name)
		return err
	}
	copied := time.Now()
	if from == "" {
		copied, err = copiedOffsite(c, home, region, db, namespace)
	}
	if err != nil {
		return fmt.Errorf("cannot tell which of %s's %s backups in %s are off-site: %w", app, env, region.Name, err)
	}
	table := newTable(c.OutOrStdout(), "LABEL", "TYPE", "FINISHED", "SIZE", "NOTE", "OFF-SITE")
	for _, backup := range slices.Backward(backups) {
		offsite := "not yet"
		if backup.Finished.Before(copied) {
			offsite = "yes"
		}
		table.row(backup.Label, backup.Type, age(time.Since(backup.Finished)), memory(float64(backup.Stored)), backup.Note, offsite)
	}
	return table.print()
}

func backupsFrom(c *cobra.Command, home cloud.Cloud, region cloud.Region, app, env, from string) (database.Database, []database.Backup, error) {
	if from == "" {
		return backupsOf(c, home, region, app, env)
	}
	if _, err := home.Find(from); err != nil {
		return database.Database{}, nil, err
	}
	namespace := env + "-" + app
	fetched := fmt.Sprintf("following it until %s's off-site copy of the backups is here. Ctrl-C stops watching, not the fetch", from)
	if err := runWorkflow(c, c.ErrOrStderr(), region, app, namespace, home.Database.Fetch, fetched, workflows.Parameter{Name: "from", Value: from}); err != nil {
		return database.Database{}, nil, err
	}
	return backupsOf(c, home, region, app, env, fmt.Sprintf(home.Database.Fetched, from, namespace))
}

func copiedOffsite(c *cobra.Command, home cloud.Cloud, region cloud.Region, db database.Database, namespace string) (time.Time, error) {
	copied, err := db.CopiedOffsite(c.Context())
	var taken []workflows.Run
	if err == nil {
		var runs workflows.Runs
		runs, err = connect(region, func(cfg *rest.Config) (workflows.Runs, error) { return workflows.In(cfg, namespace) })
		if err == nil {
			taken, err = runs.List(c.Context(), "")
		}
	}
	for _, run := range taken {
		if run.Flow() == home.Database.Backup && run.Status.Phase == "Succeeded" && run.Status.FinishedAt.After(copied) {
			copied = run.Status.FinishedAt
		}
	}
	return copied, err
}
