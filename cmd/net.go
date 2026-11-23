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

	"github.com/cloudwalksolutions/ccloud/internal/app"
	"github.com/cloudwalksolutions/ccloud/internal/cloud"
)

func netCmd(home cloud.Cloud) *cobra.Command {
	var appOption, regionOption, env string
	var since time.Duration
	command := &cobra.Command{
		Use:     "net [--env <env>] [--app <app>]",
		Short:   "An app's requests, the internet hosts it calls and what went unanswered, and what was dropped: every environment, or one",
		Example: "  ccloud net\n  ccloud net -e prod --since 6h",
		Args:    cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			appName, err := whichApp(c, home, appOption)
			if err != nil {
				return err
			}
			if env == "" {
				return showEveryEnvironment(c, home, regionOption, appName, since)
			}
			region, err := regionFor(c, home, regionOption, appName, env)
			if err != nil {
				return err
			}
			return showEnvironment(c, home, region, appName, env, since)
		},
	}
	appFlag(command, &appOption)
	command.Flags().StringVarP(&regionOption, "region", "r", "", regionHelp(home)+"; found for you when left out")
	command.Flags().StringVarP(&env, "env", "e", "", envHelp+"; every environment when left out")
	command.Flags().DurationVar(&since, "since", time.Hour, "how far back to look")
	return command
}

func showEnvironment(c *cobra.Command, home cloud.Cloud, region cloud.Region, appName, env string, since time.Duration) error {
	done := step(c, fmt.Sprintf("Asking %s about %s's %s traffic", region.Name, appName, env))
	signedIn, err := sessionOf(c).gateway(c, home, region)
	var seen app.Traffic
	if err == nil {
		seen, err = signedIn.Network(c.Context(), appName, env, since)
	}
	done(err, fmt.Sprintf("Read %s's %s traffic", appName, env))
	if err != nil {
		return fmt.Errorf("asking %s about %s's %s traffic: %w", region.Name, appName, env, err)
	}
	out := c.OutOrStdout()
	color := paletteFor(out)
	fmt.Fprintf(out, "%s · %s · last %s\n", env, region.Name, window(since))
	return errors.Join(printIngress(out, color, seen.Ingress), printEgress(out, color, seen), printDrops(out, color, seen))
}

func showEveryEnvironment(c *cobra.Command, home cloud.Cloud, regionOption, appName string, since time.Duration) error {
	regions, err := namedOr(c, home, regionOption, func(*cobra.Command) ([]cloud.Region, error) { return home.Regions, nil })
	if err != nil {
		return err
	}
	done := step(c, fmt.Sprintf("Asking about %s's traffic", appName))
	seen, err := everyEnvironmentsTraffic(c, home, regions, appName, since)
	done(err, fmt.Sprintf("Read %s's traffic", appName))
	if err != nil {
		return err
	}
	slices.SortStableFunc(seen, func(a, b app.EnvironmentTraffic) int {
		return app.Environment{Name: a.Name}.Compare(app.Environment{Name: b.Name})
	})
	return printEnvironments(c.OutOrStdout(), seen)
}

func everyEnvironmentsTraffic(c *cobra.Command, home cloud.Cloud, regions []cloud.Region, appName string, since time.Duration) ([]app.EnvironmentTraffic, error) {
	var seen []app.EnvironmentTraffic
	for _, region := range regions {
		signedIn, err := sessionOf(c).gateway(c, home, region)
		var found []app.EnvironmentTraffic
		if err == nil {
			found, err = signedIn.EveryNetwork(c.Context(), appName, since)
		}
		if err != nil {
			return nil, fmt.Errorf("asking %s about %s's traffic: %w", region.Name, appName, err)
		}
		seen = append(seen, found...)
	}
	return seen, nil
}

func printEnvironments(out io.Writer, seen []app.EnvironmentTraffic) error {
	table := newTable(out, "ENV", "REQ/S", "5XX", "P95", "HOSTS", "CONNS", "NO ANSWER", "DROPPED")
	table.rightAlign(1, 2, 3, 4, 5, 6, 7)
	for _, e := range seen {
		opened, unanswered := e.Connections()
		table.row(e.Name, fmt.Sprintf("%.2f", e.Ingress.RequestsPerSecond), fmt.Sprintf("%.1f%%", e.Ingress.ServerErrors*100),
			seconds(e.Ingress.P95Seconds), strconv.Itoa(e.Hosts()), fmt.Sprintf("%.0f", opened), partOf(unanswered, opened), fmt.Sprintf("%.0f", e.Dropped()))
	}
	return table.print()
}

func printIngress(out io.Writer, color palette, in app.Ingress) error {
	heading := color.section("↓", "INGRESS")
	if in.RequestsPerSecond == 0 {
		_, err := fmt.Fprintln(out, heading+"  "+color.none())
		return err
	}
	fmt.Fprintf(out, "%s  %.2f requests/s · %s 5xx · p95 %s\n", heading, in.RequestsPerSecond,
		color.errorRate(in.ServerErrors), color.latency(in.P95Seconds))
	table := newTable(out, "  HOST", "SERVICE", "REQ/S", "5XX", "P95")
	table.rightAlign(2, 3, 4)
	for _, h := range in.Hosts {
		if h.RequestsPerSecond == 0 {
			continue
		}
		table.row("  "+h.Host, h.Service, fmt.Sprintf("%.2f", h.RequestsPerSecond), color.errorRate(h.ServerErrors), color.latency(h.P95Seconds))
	}
	return table.print()
}

func printEgress(out io.Writer, color palette, seen app.Traffic) error {
	heading := color.section("↑", "EGRESS")
	if len(seen.Egress) == 0 {
		_, err := fmt.Fprintln(out, heading+"  "+color.none())
		return err
	}
	opened, unanswered := seen.Connections()
	fmt.Fprintf(out, "%s  %s · %.0f connections · %.0f unanswered\n", heading, quantity(seen.Hosts(), "host"), opened, unanswered)
	table := newTable(out, "  WORKLOAD · HOST", "PORT", "CONNS", "NO ANSWER", "RESET")
	table.rightAlign(1, 2, 3, 4)
	for _, group := range seen.EgressByWorkload() {
		table.row("  " + group.Name + " (" + group.Kind + ")")
		for _, e := range group.Rows {
			table.row("    "+e.Destination, strconv.Itoa(e.Port), fmt.Sprintf("%.0f", e.Connections),
				color.graded(app.Unanswering, e.Unanswered/e.Connections, partOf(e.Unanswered, e.Connections)),
				color.graded(app.Resetting, e.Reset/e.Connections, partOf(e.Reset, e.Connections)))
		}
	}
	return table.print()
}

func printDrops(out io.Writer, color palette, seen app.Traffic) error {
	heading := color.section("✗", "DROPPED")
	if len(seen.Drops) == 0 {
		_, err := fmt.Fprintln(out, heading+"  "+color.none())
		return err
	}
	fmt.Fprintf(out, "%s  %s\n", heading, color.red(fmt.Sprintf("%.0f connections", seen.Dropped())))
	table := newTable(out, "  WORKLOAD · DESTINATION", "PORT", "REASON", "COUNT")
	table.rightAlign(1, 3)
	for _, group := range seen.DropsByWorkload() {
		table.row("  " + group.Name + " (" + group.Kind + ")")
		for _, d := range group.Rows {
			table.row("    "+d.Destination, strconv.Itoa(d.Port), d.Reason, fmt.Sprintf("%.0f", d.Count))
		}
	}
	return table.print()
}

func (p palette) section(arrow, name string) string {
	if !p {
		return name
	}
	return p.bold(arrow + " " + name)
}

func (p palette) none() string {
	if !p {
		return "none"
	}
	return p.green("✓ none")
}

func (p palette) graded(g app.Graded, value float64, text string) string {
	if severity, ok := g.Of(value); ok {
		return p.severe(severity, text)
	}
	return text
}

func (p palette) errorRate(fraction float64) string {
	return p.graded(app.ServerErroring, fraction, fmt.Sprintf("%.1f%%", fraction*100))
}

func (p palette) latency(s float64) string { return p.graded(app.Slow, s, seconds(s)) }

func partOf(part, of float64) string {
	if part == 0 {
		return "0"
	}
	return fmt.Sprintf("%.0f (%s)", part, percent(part/of))
}

func seconds(s float64) string {
	return time.Duration(s * float64(time.Second)).Round(time.Millisecond).String()
}

func window(since time.Duration) string {
	return strings.TrimSuffix(strings.TrimSuffix(since.String(), "0s"), "0m")
}
