package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
	"github.com/cloudwalksolutions/ccloud/internal/gateway"
)

type regionUsage struct {
	region cloud.Region
	usage  gateway.Usage
}

func topCmd(home cloud.Cloud) *cobra.Command {
	var app, region, env string
	command := &cobra.Command{
		Use:     "top [-e <env>] [--app <app>]",
		Short:   "How much CPU and memory an app uses, everywhere or in one environment, and what is under pressure",
		Example: "  ccloud top\n  ccloud top -e prod",
		Args:    cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			named, err := whichApp(c, home, app)
			if err != nil {
				return err
			}
			usages, err := usagesOf(c, home, region, named, env)
			if err != nil {
				return err
			}
			if len(usages) == 0 {
				_, err := fmt.Fprintf(c.OutOrStdout(), "%s has no environment you may read.\n", named)
				return err
			}
			if env == "" {
				return printEnvironments(c.OutOrStdout(), usages)
			}
			return printContainers(c.OutOrStdout(), usages)
		},
	}
	command.Flags().StringVarP(&app, "app", "a", "", "the app, as it is named in its namespaces")
	command.Flags().StringVarP(&region, "region", "r", "", "the "+home.Name+" region: "+strings.Join(home.Names(), ", ")+"; every region hosting the app when left out")
	command.Flags().StringVarP(&env, "env", "e", "", "one environment, container by container; every environment when left out")
	return command
}

func usagesOf(c *cobra.Command, home cloud.Cloud, name, app, env string) (usages []regionUsage, err error) {
	regions, err := namedOr(c, home, name, func(*cobra.Command) ([]cloud.Region, error) { return home.Regions, nil })
	if err != nil {
		return nil, err
	}
	names := make([]string, len(regions))
	for i, region := range regions {
		names[i] = region.Name
	}
	done := step(c, fmt.Sprintf("Asking %s what %s uses", and(names), app))
	defer func() { done(err, fmt.Sprintf("Read what %s uses", app)) }()
	for _, region := range regions {
		signedIn, err := sessionOf(c).gateway(c, home, region)
		var found []gateway.Usage
		if err == nil {
			found, err = signedIn.Resources(c.Context(), app, env)
		}
		if err != nil {
			return nil, fmt.Errorf("asking %s what %s uses: %w", region.Name, app, err)
		}
		for _, usage := range found {
			usages = append(usages, regionUsage{region: region, usage: usage})
		}
	}
	return usages, nil
}
