package cmd

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
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
		signedIn, err := sessionOf(c).gateway(c, home, region)
		var seen gateway.Traffic
		if err == nil {
			seen, err = signedIn.Network(c.Context(), app, env, since)
		}
		if err != nil {
			return fmt.Errorf("asking %s about %s's %s traffic: %w", region.Name, app, env, err)
		}
		return printTraffic(c.OutOrStdout(), seen, since)
	})
	command.Flags().StringP("env", "e", "", "the environment: prod, stage, ci, or a feature environment's name")
	command.Flags().Duration("since", time.Hour, "how far back to look")
	return command
}

func printTraffic(out io.Writer, seen gateway.Traffic, since time.Duration) error {
	window := strings.TrimSuffix(strings.TrimSuffix(since.String(), "0s"), "0m")
	color := paletteFor(out)
	table := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	in := seen.Ingress
	if in.RequestsPerSecond == 0 {
		fmt.Fprintf(table, "Ingress over the last %s: no requests.\n", window)
	} else {
		fmt.Fprintf(table, "Ingress over the last %s: %.2f requests/s, %.1f%% 5xx, p95 %s.\n", window,
			in.RequestsPerSecond, in.ServerErrors*100, time.Duration(in.P95Seconds*float64(time.Second)).Round(time.Millisecond))
	}
	if len(seen.Egress) == 0 {
		fmt.Fprintf(table, "No connections to the internet in the last %s.\n", window)
	} else {
		fmt.Fprintln(table, "\n"+color.paint("01", "DESTINATION")+"\tPORT\tCONNECTIONS\tUNANSWERED\tRESET")
		for _, e := range seen.Egress {
			var flags []string
			if e.Unanswered/e.Connections >= 0.1 {
				flags = append(flags, fmt.Sprintf("%s unanswered", percent(e.Unanswered/e.Connections)))
			}
			if e.Reset > 0 {
				flags = append(flags, fmt.Sprintf("%.0f reset", e.Reset))
			}
			fmt.Fprintf(table, "%s\t%d\t%.0f\t%.0f\t%.0f%s\n", e.Destination, e.Port, e.Connections, e.Unanswered, e.Reset, color.flags(flags))
		}
	}
	if len(seen.Drops) == 0 {
		fmt.Fprintf(table, "No connections dropped in the last %s.\n", window)
	} else {
		fmt.Fprintln(table, "\n"+color.paint("01", "DROPPED TO")+"\tPORT\tREASON\tCOUNT")
		for _, d := range seen.Drops {
			fmt.Fprintf(table, "%s\t%d\t%s\t%.0f\n", d.Destination, d.Port, d.Reason, d.Count)
		}
	}
	return table.Flush()
}
