package cmd

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/cloudwalksolutions/ccloud/internal/app"
	"github.com/cloudwalksolutions/ccloud/internal/cloud"
	"github.com/cloudwalksolutions/ccloud/internal/repo"
)

type envOf func(*cobra.Command) (string, error)

func ciEnv(*cobra.Command) (string, error) { return "ci", nil }

const envHelp = "the environment: prod, stage, ci, or a feature environment's name"

func appFlag(command *cobra.Command, app *string) {
	command.Flags().StringVarP(app, "app", "a", "", "the app, as it is named in its namespaces")
}

func regionHelp(home cloud.Cloud) string {
	return "the " + home.Name + " region: " + strings.Join(home.Names(), ", ")
}

func appCmd(home cloud.Cloud, command *cobra.Command, env envOf, run func(*cobra.Command, cloud.Region, string) error) *cobra.Command {
	var app, region string
	appFlag(command, &app)
	command.Flags().StringVarP(&region, "region", "r", "", regionHelp(home)+"; found for you when left out")
	if command.Args == nil {
		command.Args = cobra.NoArgs
	}
	command.RunE = func(c *cobra.Command, _ []string) error {
		named, err := whichApp(c, home, app)
		if err != nil {
			return err
		}
		environment, err := env(c)
		if err != nil {
			return err
		}
		found, err := regionFor(c, home, region, named, environment)
		if err != nil {
			return err
		}
		return run(c, found, named)
	}
	return command
}

func whichApp(c *cobra.Command, home cloud.Cloud, app string) (string, error) {
	named, err := appNamed(home, app)
	if err == nil && named == "" {
		err = fmt.Errorf("which app? %s --app <app>", c.CommandPath())
	}
	return named, err
}

func appNamed(home cloud.Cloud, app string) (string, error) {
	if app != "" {
		return app, nil
	}
	wd, _ := os.Getwd()
	_, named, err := repo.App(wd, home.AppDir())
	return named, err
}

func regionFor(c *cobra.Command, home cloud.Cloud, name, app, env string) (cloud.Region, error) {
	if name != "" {
		return home.Find(name)
	}
	environments, err := everyRegionsEnvironments(c, home, app)
	if err != nil {
		return cloud.Region{}, err
	}
	environment, err := environments.Named(env)
	if err != nil {
		return cloud.Region{}, err
	}
	return home.Find(environment.Region)
}

func everyRegionsEnvironments(c *cobra.Command, home cloud.Cloud, named string) (app.Environments, error) {
	environments := app.Environments{App: named}
	done := step(c, fmt.Sprintf("Finding where %s runs in %s", named, app.And(home.Names())))
	found := make([][]app.Environment, len(home.Regions))
	failed := make([]error, len(home.Regions))
	var asking sync.WaitGroup
	for i, region := range home.Regions {
		asking.Go(func() {
			signedIn, err := sessionOf(c).gateway(c, home, region)
			if err == nil {
				found[i], err = signedIn.Environments(c.Context(), named)
			}
			failed[i] = err
		})
	}
	asking.Wait()
	for i, region := range home.Regions {
		if err := failed[i]; err != nil {
			done(err, "")
			return environments, fmt.Errorf("asking %s where %s runs: %w; or name the region with --region", region.Name, named, err)
		}
		for _, environment := range found[i] {
			environment.Region = region.Name
			environments.All = append(environments.All, environment)
		}
	}
	if len(environments.All) == 0 {
		err := fmt.Errorf("no region runs %s, or you may not read any of its environments", named)
		done(err, "")
		return environments, err
	}
	done(nil, fmt.Sprintf("%s runs in %s", named, app.And(environments.Regions())))
	return environments, nil
}
