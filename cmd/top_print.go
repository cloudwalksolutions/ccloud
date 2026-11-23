package cmd

import (
	"fmt"
	"math"
	"strings"

	"github.com/cloudwalksolutions/ccloud/internal/gateway"
)

type severity int

const (
	hint severity = iota
	warn
	bad
)

type issue struct {
	severity severity
	text     string
}

func severityOf(value, warnAt, badAt float64) severity {
	switch {
	case value >= badAt:
		return bad
	case value >= warnAt:
		return warn
	}
	return hint
}

func containerIssues(c gateway.Container) []issue {
	var issues []issue
	if c.Waiting != "" {
		issues = append(issues, issue{bad, "not running: " + c.Waiting})
	}
	if c.OOMKills > 0 {
		issues = append(issues, issue{bad, fmt.Sprintf("OOMKilled %.0f×", c.OOMKills)})
	}
	if c.Memory.Limit != nil {
		if used := c.Memory.Used / *c.Memory.Limit; used >= 0.75 {
			issues = append(issues, issue{severityOf(used, 0.75, 0.9), fmt.Sprintf("memory %s of limit", percent(used))})
		}
	}
	if c.Throttled != nil && *c.Throttled >= 0.05 {
		issues = append(issues, issue{severityOf(*c.Throttled, 0.05, 0.25), "throttled " + percent(*c.Throttled)})
	}
	if c.Restarts >= 1 {
		issues = append(issues, issue{severityOf(c.Restarts, 1, 3), fmt.Sprintf("restarted %.0f×", c.Restarts)})
	}
	if c.CPU.Limit == nil {
		issues = append(issues, issue{hint, "no CPU limit"})
	}
	if c.Memory.Limit == nil {
		issues = append(issues, issue{hint, "no memory limit"})
	}
	return issues
}

func nodeIssues(n gateway.Node) []issue {
	var issues []issue
	if n.CPUWait > 0.9 {
		issues = append(issues, issue{warn, "CPU wait " + percent(n.CPUWait)})
	}
	if n.MemoryStall > 0.1 {
		issues = append(issues, issue{warn, "memory stall " + percent(n.MemoryStall)})
	}
	for _, condition := range n.Conditions {
		issues = append(issues, issue{bad, condition})
	}
	return issues
}

func (p palette) severe(s severity, text string) string {
	return p.paint(map[severity]string{hint: "2", warn: "33", bad: "31"}[s], text)
}

func (p palette) issues(issues []issue) string {
	seen := map[string]bool{}
	var painted []string
	for _, i := range issues {
		if !seen[i.text] {
			seen[i.text] = true
			painted = append(painted, p.severe(i.severity, i.text))
		}
	}
	return strings.Join(painted, ", ")
}

func percent(fraction float64) string { return fmt.Sprintf("%.0f%%", math.Round(fraction*100)) }

func memory(bytes float64) string {
	if bytes >= 1<<30 {
		return fmt.Sprintf("%.1fGi", bytes/(1<<30))
	}
	return fmt.Sprintf("%.0fMi", bytes/(1<<20))
}
