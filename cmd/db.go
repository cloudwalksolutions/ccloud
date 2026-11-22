package cmd

import (
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"sync"

	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/rest"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
	"github.com/cloudwalksolutions/ccloud/internal/database"
)

func dbCmd(home cloud.Cloud) *cobra.Command {
	db := &cobra.Command{
		Use:   "db",
		Short: "Your app's databases: reach one from your machine",
	}
	tunnel := appCmd(home, &cobra.Command{
		Use:     "tunnel --env <env> [--app <app>]",
		Short:   "Open a local port to an environment's database and print what a client needs to connect",
		Example: "  ccloud db tunnel -e stage\n  ccloud db tunnel -e stage --background\n  ccloud db tunnel -e stage --stop",
	}, namedEnv, func(c *cobra.Command, region cloud.Region, app string) error {
		return dbTunnel(c, home, region, app)
	})
	tunnel.Flags().StringP("env", "e", "", "the environment: stage, or a feature environment's name; prod for admins")
	tunnel.Flags().IntP("port", "p", 0, "the local port to listen on; the database's own port, or the next free one, when left out")
	tunnel.Flags().BoolP("sensitive", "s", false, "print the password whole, rather than its first characters")
	tunnel.Flags().Bool("background", false, "keep the tunnel open after the command returns, until --stop")
	tunnel.Flags().Bool("stop", false, "close the environment's tunnel open in the background")
	db.AddCommand(tunnel, dbResetCmd(home))
	return db
}

type dbConnection struct {
	app, env  string
	region    cloud.Region
	port      int
	owner     database.Credentials
	sensitive bool
}

func dbTunnel(c *cobra.Command, home cloud.Cloud, region cloud.Region, app string) error {
	env, _ := c.Flags().GetString("env")
	if stop, _ := c.Flags().GetBool("stop"); stop {
		return stopTunnel(c, app, env)
	}
	if env == "prod" {
		cautionAboutProd(c.ErrOrStderr(), app)
	}
	db, owner, pod, err := primaryOf(c, home, region, app, env)
	if err != nil {
		return err
	}
	port, _ := c.Flags().GetInt("port")
	if port == 0 {
		port = freePortFrom(home.Database.Port)
	}
	if !free(port) {
		return fmt.Errorf("localhost:%d is already in use; pick another port with --port", port)
	}
	sensitive, _ := c.Flags().GetBool("sensitive")
	opened := dbConnection{app: app, env: env, region: region, port: port, owner: owner, sensitive: sensitive}
	if background, _ := c.Flags().GetBool("background"); background {
		return openInBackground(c, opened)
	}
	done := step(c, fmt.Sprintf("Opening localhost:%d to %s's %s database", port, app, env))
	var finished sync.Once
	ready := make(chan struct{})
	go func() {
		<-ready
		finished.Do(func() { done(nil, fmt.Sprintf("Opened localhost:%d", port)) })
		_ = opened.print(c.OutOrStdout(), "Ctrl-C")
	}()
	err = db.Forward(c.Context(), pod, port, ready, c.ErrOrStderr())
	finished.Do(func() { done(err, "") })
	return err
}

func primaryOf(c *cobra.Command, home cloud.Cloud, region cloud.Region, app, env string) (db database.Database, owner database.Credentials, pod string, err error) {
	done := step(c, fmt.Sprintf("Reading %s's %s database credentials in %s", app, env, region.Name))
	defer func() { done(err, fmt.Sprintf("Read %s's %s database credentials", app, env)) }()
	db, err = connect(region, func(cfg *rest.Config) (database.Database, error) { return database.In(cfg, env+"-"+app, home.Database) })
	if err != nil {
		return db, owner, "", err
	}
	owner, err = db.Credentials(c.Context())
	if err != nil {
		return db, owner, "", errMayNotTunnel(err, region, app, env)
	}
	pod, err = db.Primary(c.Context())
	if err != nil {
		return db, owner, "", fmt.Errorf("%s's %s database has %w in %s", app, env, err, region.Name)
	}
	return db, owner, pod, nil
}

func (t dbConnection) print(out io.Writer, closer string) error {
	password := truncated(t.owner.Password)
	userinfo := t.owner.User + ":" + password
	if t.sensitive {
		password = t.owner.Password
		userinfo = url.UserPassword(t.owner.User, password).String()
	}
	fmt.Fprintf(out, "Tunneled to %s's %s database in %s; %s closes it.\n", t.app, t.env, t.region.Name, closer)
	table := newTable(out)
	table.row("  host", "localhost")
	table.row("  port", strconv.Itoa(t.port))
	table.row("  database", t.owner.Name)
	table.row("  user", t.owner.User)
	table.row("  password", password)
	table.row("  url", fmt.Sprintf("postgres://%s@localhost:%d/%s", userinfo, t.port, t.owner.Name))
	return table.print()
}

func truncated(secret string) string {
	return secret[:min(4, len(secret))] + "…"
}

func cautionAboutProd(out io.Writer, app string) {
	fmt.Fprint(out, paletteFor(out).paint("31", fmt.Sprintf("CAUTION: this is %s's production database, and its only primary.\n", app)+
		"  - Every query adds load to the live app: keep them small and indexed.\n"+
		"  - A write changes real data at once, and nothing here undoes it.\n"+
		"  - Reads show customers' personal data: select only what you need.\n"))
}

func freePortFrom(port int) int {
	for !free(port) {
		port++
	}
	return port
}

func free(port int) bool {
	listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))
	if err == nil {
		_ = listener.Close()
	}
	return err == nil
}

func errMayNotTunnel(err error, region cloud.Region, app, env string) error {
	if apierrors.IsForbidden(err) {
		err = fmt.Errorf("you may not tunnel into %s's %s database in %s", app, env, region.Name)
	}
	if apierrors.IsNotFound(err) {
		err = fmt.Errorf("%s's %s has no database in %s", app, env, region.Name)
	}
	return err
}
