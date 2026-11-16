package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
	"github.com/cloudwalksolutions/ccloud/internal/gateway"
	"github.com/cloudwalksolutions/ccloud/internal/repo"
)

type envOf func(*cobra.Command) (string, error)

func ciEnv(*cobra.Command) (string, error) { return "ci", nil }

func appCmd(home cloud.Cloud, command *cobra.Command, env envOf, run func(*cobra.Command, cloud.Region, string) error) *cobra.Command {
	var app, region string
	command.Flags().StringVarP(&app, "app", "a", "", "the app, as it is named in its namespaces")
	command.Flags().StringVarP(&region, "region", "r", "", "the "+home.Name+" region: "+strings.Join(home.Names(), ", ")+"; found for you when left out")
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
	return repo.App(wd, home.AppDir())
}

func regionFor(c *cobra.Command, home cloud.Cloud, name, app, env string) (cloud.Region, error) {
	if name != "" {
		return home.Find(name)
	}
	environments, err := everyRegionsEnvironments(c, home, app)
	if err != nil {
		return cloud.Region{}, err
	}
	var hosts, has []string
	for _, region := range home.Regions {
		for _, environment := range environments[region.Name] {
			has = append(has, fmt.Sprintf("%s (%s)", environment.Name, region.Name))
			if environment.Name == env {
				hosts = append(hosts, region.Name)
			}
		}
	}
	switch len(hosts) {
	case 0:
		return cloud.Region{}, fmt.Errorf("no region hosts %s's %s; it has %s", app, env, strings.Join(has, ", "))
	case 1:
		return home.Find(hosts[0])
	}
	return cloud.Region{}, fmt.Errorf("%s both host %s's %s; name one with --region", strings.Join(hosts, " and "), app, env)
}

func everyRegionsEnvironments(c *cobra.Command, home cloud.Cloud, app string) (map[string][]gateway.Environment, error) {
	environments := map[string][]gateway.Environment{}
	for _, region := range home.Regions {
		signedIn, err := signInToGateway(c, home, region)
		if err == nil {
			environments[region.Name], err = signedIn.Environments(c.Context(), app)
		}
		if err != nil {
			return nil, fmt.Errorf("asking %s where %s runs: %w; or name the region with --region", region.Name, app, err)
		}
	}
	return environments, nil
}
