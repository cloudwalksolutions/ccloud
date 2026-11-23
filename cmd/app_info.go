package cmd

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
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
	table := newTable(c.OutOrStdout(), "ENV", "REGION", "URL")
	for _, r := range rows {
		table.row(r.env, r.region, r.url)
	}
	return table.print()
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
	return errors.Join(printPulls(out, s.Pulls), printWeeks(out, s, weeks))
}

func printPulls(out io.Writer, pulls []gateway.Pull) error {
	if len(pulls) == 0 {
		_, err := fmt.Fprintln(out, "No open pull requests.")
		return err
	}
	color := paletteFor(out)
	table := newTable(out, "PR", "TITLE", "AUTHOR", "OPENED", "REVIEW", "CHECKS", "ENVIRONMENT")
	for _, p := range pulls {
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
		table.row("#"+strconv.Itoa(p.Number), p.Title, p.Author, opened, review, strings.Join(checks, "  "), p.Environment)
	}
	return table.print()
}

func printWeeks(out io.Writer, s gateway.Summary, weeks int) error {
	fmt.Fprintf(out, "\nLast %s:\n", quantity(weeks, "week"))
	table := newTable(out)
	table.row("  pull requests", fmt.Sprintf("%d merged; %s from open to merge; %s to a first review",
		s.Flow.Merged, hours(s.Flow.OpenToMergeHours), hours(s.Flow.FirstReviewHours)))
	table.row("  delivery", fmt.Sprintf("%s; %s from merge to prod", quantity(s.Delivery.Deploys, "prod deploy"), hours(s.Delivery.MergeToProdHours)))
	for _, ci := range s.CI {
		table.row("  "+ci.Flow, fmt.Sprintf("%s; median %s; %s pass first time; %s", quantity(ci.Runs, "run"),
			time.Duration(ci.MedianSeconds*float64(time.Second)).Round(time.Second), percent(ci.FirstTryPassRate), quantity(ci.Reruns, "rerun")))
	}
	return table.print()
}

func quantity(n int, thing string) string {
	if n == 1 {
		return "1 " + thing
	}
	return fmt.Sprintf("%d %ss", n, thing)
}

func hours(h float64) string { return strconv.FormatFloat(h, 'f', -1, 64) + "h" }
