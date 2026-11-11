package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
	"github.com/cloudwalksolutions/ccloud/internal/kubeconfig"
	"github.com/cloudwalksolutions/ccloud/internal/workflows"
)

type command struct {
	*cobra.Command
	home   cloud.Cloud
	region cloud.Region
	app    string
	runs   workflows.Runs
}

func (c command) ci() string { return "ci-" + c.app }

func regionCmd(home cloud.Cloud, cobraCmd *cobra.Command, run func(command, context.Context) error) *cobra.Command {
	var name string
	cobraCmd.Flags().StringVarP(&name, "region", "r", "", "the "+home.Name+" region: "+strings.Join(home.Names(), ", "))
	if cobraCmd.Args == nil {
		cobraCmd.Args = cobra.NoArgs
	}
	cobraCmd.RunE = func(c *cobra.Command, _ []string) error {
		if name == "" {
			return fmt.Errorf("which region? %s --region <region>, where <region> is one of %s", c.CommandPath(), strings.Join(home.Names(), ", "))
		}
		region, err := home.Find(name)
		if err != nil {
			return err
		}
		return run(command{Command: c, home: home, region: region}, c.Context())
	}
	return cobraCmd
}

func appCmd(home cloud.Cloud, cobraCmd *cobra.Command, run func(command, context.Context) error) *cobra.Command {
	var app string
	cobraCmd.Flags().StringVarP(&app, "app", "a", "", "the app, as it is named in its namespaces")
	return regionCmd(home, cobraCmd, func(c command, ctx context.Context) error {
		if app == "" {
			return fmt.Errorf("which app? %s --app <app>", c.CommandPath())
		}
		c.app = app
		return run(c, ctx)
	})
}

func ciCmd(home cloud.Cloud, cobraCmd *cobra.Command, run func(command, context.Context) error) *cobra.Command {
	return appCmd(home, cobraCmd, func(c command, ctx context.Context) error {
		cfg, err := kubeconfig.REST(kubeconfig.Path(), c.region.Context())
		if err != nil {
			return c.signInError(err)
		}
		c.runs, err = workflows.In(cfg, c.ci())
		if err != nil {
			return fmt.Errorf("your %s context is damaged; sign in again with ccloud clusters auth --region %s: %w", c.region.Context(), c.region.Name, err)
		}
		return run(c, ctx)
	})
}

func runCmd(home cloud.Cloud, cobraCmd *cobra.Command, run func(command, context.Context, workflows.Run) error) *cobra.Command {
	cobraCmd.Args = func(c *cobra.Command, args []string) error {
		if len(args) != 1 {
			return fmt.Errorf("which run? %s <run>, where <run> is an ID from ccloud workflow list", c.CommandPath())
		}
		return nil
	}
	return ciCmd(home, cobraCmd, func(c command, ctx context.Context) error {
		r, err := c.getRun(ctx)
		if err != nil {
			return err
		}
		return run(c, ctx, r)
	})
}

func (c command) getRun(ctx context.Context) (workflows.Run, error) {
	name := c.Flags().Arg(0)
	r, err := c.runs.Get(ctx, name)
	if apierrors.IsNotFound(err) {
		err = fmt.Errorf("%s has no run %s in %s; see ccloud workflow list -a %s -r %s", c.app, name, c.region.Name, c.app, c.region.Name)
	}
	return r, err
}

func (c command) signInError(err error) error {
	if errors.Is(err, kubeconfig.ErrNoContext) {
		err = fmt.Errorf("you have not signed in to %s yet; run ccloud clusters auth --region %s", c.region.Name, c.region.Name)
	}
	return err
}

func (c command) switched() error {
	_, err := fmt.Fprintf(c.OutOrStdout(), "Switched to context %q.\n", c.region.Context())
	return err
}
