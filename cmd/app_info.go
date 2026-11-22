package cmd

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
	"github.com/cloudwalksolutions/ccloud/internal/gateway"
)

func appInfoCmd(home cloud.Cloud) *cobra.Command {
	var app, region string
	var summary bool
	var weeks int
	command := &cobra.Command{
		Use:     "app [--summary] [--app <app>]",
		Short:   "An app's environments and where they serve; with --summary, its open pull requests and recent weeks",
		Example: "  ccloud app\n  ccloud app --summary --weeks 2",
		Args:    cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			named, err := whichApp(c, home, app)
			if err != nil {
				return err
			}
			if summary {
				return summarize(c, home, region, named, weeks)
			}
			environments, err := everyRegionsEnvironments(c, home, named)
			if err != nil {
				return err
			}
			return printEnvironmentList(c, environments)
		},
	}
	command.Flags().StringVarP(&app, "app", "a", "", "the app, as it is named in its namespaces")
	command.Flags().StringVarP(&region, "region", "r", "", "with --summary, the region running the app's CI; found for you when left out")
	command.Flags().BoolVar(&summary, "summary", false, "open pull requests with their checks, then how the last weeks went")
	command.Flags().IntVar(&weeks, "weeks", 4, "with --summary, how many weeks back to look")
	return command
}

func printEnvironmentList(c *cobra.Command, environments map[string][]gateway.Environment) error {
	type row struct{ env, region, url string }
	var rows []row
	for region, envs := range environments {
		for _, e := range envs {
			rows = append(rows, row{e.Name, region, e.URL})
		}
	}
	slices.SortFunc(rows, func(a, b row) int { return strings.Compare(a.env, b.env) })
	color := paletteFor(c.OutOrStdout())
	table := tabwriter.NewWriter(c.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, color.paint("01", "ENV")+"\tREGION\tURL")
	for _, r := range rows {
		fmt.Fprintln(table, strings.TrimRight(r.env+"\t"+r.region+"\t"+r.url, "\t"))
	}
	return table.Flush()
}

func summarize(c *cobra.Command, home cloud.Cloud, name, app string, weeks int) error {
	region, err := regionFor(c, home, name, app, "ci")
	if err != nil {
		return err
	}
	done := step(c, fmt.Sprintf("Asking %s for %s's summary", region.Name, app))
	signedIn, err := sessionOf(c).gateway(c, home, region)
	var s gateway.Summary
	if err == nil {
		s, err = signedIn.Summary(c.Context(), app, weeks)
	}
	done(err, fmt.Sprintf("Read %s's summary", app))
	if err != nil {
		return fmt.Errorf("asking %s for %s's summary: %w", region.Name, app, err)
	}
	out := c.OutOrStdout()
	color := paletteFor(out)
	table := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if len(s.Pulls) == 0 {
		fmt.Fprintln(table, "No open pull requests.")
	} else {
		fmt.Fprintln(table, color.paint("01", "PR")+"\tTITLE\tAUTHOR\tOPENED\tREVIEW\tCHECKS\tENVIRONMENT")
	}
	for _, p := range s.Pulls {
		review := "waiting"
		switch {
		case p.Draft:
			review = "draft"
		case p.Reviewed:
			review = "reviewed"
		}
		checks := make([]string, len(p.Checks))
		for i, check := range p.Checks {
			checks[i] = color.status(check.Status) + " " + check.Flow
		}
		opened := age(time.Duration(p.OpenedHoursAgo * float64(time.Hour)))
		fmt.Fprintln(table, strings.TrimRight(fmt.Sprintf("#%d\t%s\t%s\t%s\t%s\t%s\t%s",
			p.Number, p.Title, p.Author, opened, review, strings.Join(checks, "  "), p.Environment), "\t"))
	}
	fmt.Fprintf(table, "\nLast %s:\n", quantity(weeks, "week"))
	fmt.Fprintf(table, "  pull requests\t%d merged; %s from open to merge; %s to a first review\n",
		s.Flow.Merged, hours(s.Flow.OpenToMergeHours), hours(s.Flow.FirstReviewHours))
	fmt.Fprintf(table, "  delivery\t%s; %s from merge to prod\n", quantity(s.Delivery.Deploys, "prod deploy"), hours(s.Delivery.MergeToProdHours))
	for _, ci := range s.CI {
		fmt.Fprintf(table, "  %s\t%s; median %s; %s pass first time; %s\n", ci.Flow, quantity(ci.Runs, "run"),
			time.Duration(ci.MedianSeconds*float64(time.Second)), percent(ci.FirstTryPassRate), quantity(ci.Reruns, "rerun"))
	}
	return table.Flush()
}

func quantity(n int, thing string) string {
	if n == 1 {
		return "1 " + thing
	}
	return fmt.Sprintf("%d %ss", n, thing)
}

func hours(h float64) string { return strconv.FormatFloat(h, 'f', -1, 64) + "h" }
