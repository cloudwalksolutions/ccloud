package cmd

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
	"github.com/cloudwalksolutions/ccloud/internal/gateway"
)

func netCmd(home cloud.Cloud) *cobra.Command {
	command := appCmd(home, &cobra.Command{
		Use:     "net --env <env> [--app <app>]",
		Short:   "An environment's requests, the internet hosts it calls and what went unanswered, and what was dropped",
		Example: "  ccloud net -e prod --since 6h",
	}, namedEnv, func(c *cobra.Command, region cloud.Region, app string) error {
		env, _ := c.Flags().GetString("env")
		since, _ := c.Flags().GetDuration("since")
		done := step(c, fmt.Sprintf("Asking %s about %s's %s traffic", region.Name, app, env))
		signedIn, err := sessionOf(c).gateway(c, home, region)
		var seen gateway.Traffic
		if err == nil {
			seen, err = signedIn.Network(c.Context(), app, env, since)
		}
		done(err, fmt.Sprintf("Read %s's %s traffic", app, env))
		if err != nil {
			return fmt.Errorf("asking %s about %s's %s traffic: %w", region.Name, app, env, err)
		}
		return printTraffic(c.OutOrStdout(), seen, since)
	})
	command.Flags().StringP("env", "e", "", envHelp)
	command.Flags().Duration("since", time.Hour, "how far back to look")
	return command
}

func printTraffic(out io.Writer, seen gateway.Traffic, since time.Duration) error {
	window := strings.TrimSuffix(strings.TrimSuffix(since.String(), "0s"), "0m")
	in := seen.Ingress
	if in.RequestsPerSecond == 0 {
		fmt.Fprintf(out, "Ingress over the last %s: no requests.\n", window)
	} else {
		fmt.Fprintf(out, "Ingress over the last %s: %.2f requests/s, %.1f%% 5xx, p95 %s.\n", window,
			in.RequestsPerSecond, in.ServerErrors*100, time.Duration(in.P95Seconds*float64(time.Second)).Round(time.Millisecond))
	}
	return errors.Join(printEgress(out, seen.Egress, window), printDrops(out, seen.Drops, window))
}

func printEgress(out io.Writer, egress []gateway.Egress, window string) error {
	if len(egress) == 0 {
		_, err := fmt.Fprintf(out, "No connections to the internet in the last %s.\n", window)
		return err
	}
	color := paletteFor(out)
	fmt.Fprintln(out)
	table := newTable(out, "DESTINATION", "PORT", "CONNECTIONS", "UNANSWERED", "RESET")
	for _, e := range egress {
		var flags []string
		if e.Unanswered/e.Connections >= 0.1 {
			flags = append(flags, fmt.Sprintf("%s unanswered", percent(e.Unanswered/e.Connections)))
		}
		if e.Reset > 0 {
			flags = append(flags, fmt.Sprintf("%.0f reset", e.Reset))
		}
		table.row(e.Destination, strconv.Itoa(e.Port), fmt.Sprintf("%.0f", e.Connections), fmt.Sprintf("%.0f", e.Unanswered),
			fmt.Sprintf("%.0f", e.Reset), color.flags(flags))
	}
	return table.print()
}

func printDrops(out io.Writer, drops []gateway.Drop, window string) error {
	if len(drops) == 0 {
		_, err := fmt.Fprintf(out, "No connections dropped in the last %s.\n", window)
		return err
	}
	fmt.Fprintln(out)
	table := newTable(out, "DROPPED TO", "PORT", "REASON", "COUNT")
	for _, d := range drops {
		table.row(d.Destination, strconv.Itoa(d.Port), d.Reason, fmt.Sprintf("%.0f", d.Count))
	}
	return table.print()
}

func (p palette) flags(flags []string) string {
	if len(flags) == 0 {
		return ""
	}
	return p.red("! " + strings.Join(flags, ", "))
}
