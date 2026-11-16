package cmd

import (
	"fmt"
	"io"
	"math"
	"strings"
	"text/tabwriter"

	"github.com/cloudwalksolutions/ccloud/internal/gateway"
)

func printContainers(out io.Writer, usages []regionUsage) error {
	color := paletteFor(out)
	table := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, color.paint("01", "CONTAINER")+"\tCPU USED/REQ/LIMIT\tMEMORY USED/LIMIT\tRESTARTS\tNODE\tFLAGS")
	var nodes []gateway.Node
	for _, u := range usages {
		for _, c := range u.usage.Containers {
			fmt.Fprintf(table, "%s/%s\t%s/%s/%s\t%s/%s\t%.0f\t%s%s\n", c.Pod, c.Container,
				cores(&c.CPU.Used), cores(c.CPU.Request), cores(c.CPU.Limit),
				memory(&c.Memory.Used), memory(c.Memory.Limit), c.Restarts, c.Node, color.flags(containerFlags(c)))
		}
		nodes = append(nodes, u.usage.Nodes...)
	}
	for _, n := range nodes {
		if pressure := nodeFlags(n); len(pressure) > 0 {
			fmt.Fprintf(table, "\n%s is under pressure (the node, not your limits): %s\n", n.Name, strings.Join(pressure, ", "))
		}
	}
	return table.Flush()
}

func printEnvironments(out io.Writer, usages []regionUsage) error {
	color := paletteFor(out)
	table := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, color.paint("01", "ENV")+"\tREGION\tPODS\tCPU USED/REQ\tMEMORY USED/LIMIT\tFLAGS")
	for _, u := range usages {
		var cpu, requested, used, limit float64
		pods := map[string]bool{}
		containers := 0
		for _, c := range u.usage.Containers {
			pods[c.Pod] = true
			cpu += c.CPU.Used
			used += c.Memory.Used
			if c.CPU.Request != nil {
				requested += *c.CPU.Request
			}
			if c.Memory.Limit != nil {
				limit += *c.Memory.Limit
			}
			if len(containerFlags(c)) > 0 {
				containers++
			}
		}
		pressured := 0
		for _, n := range u.usage.Nodes {
			if len(nodeFlags(n)) > 0 {
				pressured++
			}
		}
		fmt.Fprintf(table, "%s\t%s\t%d\t%.2f/%.2f\t%s/%s%s\n", u.usage.Name, u.region.Name, len(pods), cpu, requested,
			memory(&used), memory(&limit), color.flags(counted(containers, "container", counted(pressured, "node", nil))))
	}
	fmt.Fprintln(table, "ccloud top -e <env> shows each container and why it is flagged.")
	return table.Flush()
}

func counted(n int, thing string, rest []string) []string {
	switch n {
	case 0:
		return rest
	case 1:
		return append([]string{"1 " + thing}, rest...)
	}
	return append([]string{fmt.Sprintf("%d %ss", n, thing)}, rest...)
}

func containerFlags(c gateway.Container) []string {
	var flags []string
	if c.CPU.Limit == nil {
		flags = append(flags, "no CPU limit")
	}
	if c.Memory.Limit == nil {
		flags = append(flags, "no memory limit")
	}
	if c.Throttled != nil && *c.Throttled >= 0.25 {
		flags = append(flags, fmt.Sprintf("throttled %s", percent(*c.Throttled)))
	}
	if c.Memory.Limit != nil && c.Memory.Used / *c.Memory.Limit >= 0.9 {
		flags = append(flags, fmt.Sprintf("memory %s of limit", percent(c.Memory.Used / *c.Memory.Limit)))
	}
	if c.OOMKills > 0 {
		flags = append(flags, fmt.Sprintf("OOMKilled %.0f×", c.OOMKills))
	}
	if c.Restarts >= 3 {
		flags = append(flags, fmt.Sprintf("restarted %.0f×", c.Restarts))
	}
	return flags
}

func nodeFlags(n gateway.Node) []string {
	var flags []string
	if n.CPUWait > 0.9 {
		flags = append(flags, "CPU wait "+percent(n.CPUWait))
	}
	if n.MemoryStall > 0.1 {
		flags = append(flags, "memory stall "+percent(n.MemoryStall))
	}
	return append(flags, n.Conditions...)
}

func (p palette) flags(flags []string) string {
	if len(flags) == 0 {
		return ""
	}
	return "\t" + p.paint("31", "! "+strings.Join(flags, ", "))
}

func percent(fraction float64) string { return fmt.Sprintf("%.0f%%", math.Round(fraction*100)) }

func cores(value *float64) string {
	if value == nil {
		return "none"
	}
	return fmt.Sprintf("%.2f", *value)
}

func memory(value *float64) string {
	switch {
	case value == nil:
		return "none"
	case *value >= 1<<30:
		return fmt.Sprintf("%.1fGi", *value/(1<<30))
	}
	return fmt.Sprintf("%.0fMi", *value/(1<<20))
}
