package app

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strings"
)

type Amount struct {
	Used    float64  `json:"used"`
	Request *float64 `json:"request,omitempty"`
	Limit   *float64 `json:"limit,omitempty"`
}

type Workload struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

type Container struct {
	Pod       string   `json:"pod"`
	Container string   `json:"container"`
	Node      string   `json:"node"`
	Workload  Workload `json:"workload"`
	Waiting   string   `json:"waiting,omitempty"`
	CPU       Amount   `json:"cpu"`
	Memory    Amount   `json:"memory"`
	Throttled *float64 `json:"throttled,omitempty"`
	Restarts  float64  `json:"restarts"`
	OOMKills  float64  `json:"oomKills"`
}

type Node struct {
	Name        string   `json:"name"`
	CPUWait     float64  `json:"cpuWait"`
	MemoryStall float64  `json:"memoryStall"`
	Conditions  []string `json:"conditions"`
}

type Usage struct {
	Name       string      `json:"name"`
	Containers []Container `json:"containers"`
	Nodes      []Node      `json:"nodes"`
}

type WorkloadUsage struct {
	Workload
	Containers []Container
}

func (u Usage) Workloads() []WorkloadUsage {
	var workloads []WorkloadUsage
	for _, c := range u.Containers {
		w := Workload{Kind: cmp.Or(c.Workload.Kind, "Pod"), Name: cmp.Or(c.Workload.Name, c.Pod)}
		i := slices.IndexFunc(workloads, func(known WorkloadUsage) bool { return known.Workload == w })
		if i < 0 {
			workloads = append(workloads, WorkloadUsage{Workload: w})
			i = len(workloads) - 1
		}
		workloads[i].Containers = append(workloads[i].Containers, c)
	}
	slices.SortFunc(workloads, func(a, b WorkloadUsage) int {
		return cmp.Or(strings.Compare(a.Name, b.Name), strings.Compare(a.Kind, b.Kind))
	})
	return workloads
}

type Severity int

const (
	Hint Severity = iota
	Warn
	Bad
)

type Issue struct {
	Severity Severity
	Text     string
}

type Graded struct{ Warn, Bad float64 }

var (
	MemoryPressure = Graded{Warn: 0.75, Bad: 0.9}
	Throttling     = Graded{Warn: 0.05, Bad: 0.25}
	Restarting     = Graded{Warn: 1, Bad: 3}
)

func (g Graded) Of(value float64) (Severity, bool) {
	switch {
	case value >= g.Bad:
		return Bad, true
	case value >= g.Warn:
		return Warn, true
	}
	return Hint, false
}

var containerRules = []func(Container) (Issue, bool){
	notRunning, oomKilled, memoryPressure, throttled, restarted, noCPULimit, noMemoryLimit,
}

func (c Container) Issues() []Issue { return issues(c, containerRules) }

func notRunning(c Container) (Issue, bool) {
	return Issue{Bad, "not running: " + c.Waiting}, c.Waiting != ""
}

func oomKilled(c Container) (Issue, bool) {
	return Issue{Bad, fmt.Sprintf("OOMKilled %.0f×", c.OOMKills)}, c.OOMKills > 0
}

func memoryPressure(c Container) (Issue, bool) {
	if c.Memory.Limit == nil {
		return Issue{}, false
	}
	used := c.Memory.Used / *c.Memory.Limit
	severity, ok := MemoryPressure.Of(used)
	return Issue{severity, fmt.Sprintf("memory %s of limit", percent(used))}, ok
}

func throttled(c Container) (Issue, bool) {
	if c.Throttled == nil {
		return Issue{}, false
	}
	severity, ok := Throttling.Of(*c.Throttled)
	return Issue{severity, "throttled " + percent(*c.Throttled)}, ok
}

func restarted(c Container) (Issue, bool) {
	severity, ok := Restarting.Of(c.Restarts)
	return Issue{severity, fmt.Sprintf("restarted %.0f×", c.Restarts)}, ok
}

func noCPULimit(c Container) (Issue, bool) { return Issue{Hint, "no CPU limit"}, c.CPU.Limit == nil }

func noMemoryLimit(c Container) (Issue, bool) {
	return Issue{Hint, "no memory limit"}, c.Memory.Limit == nil
}

var nodeRules = []func(Node) (Issue, bool){cpuWait, memoryStall}

func cpuWait(n Node) (Issue, bool) {
	return Issue{Warn, "CPU wait " + percent(n.CPUWait)}, n.CPUWait > 0.9
}

func memoryStall(n Node) (Issue, bool) {
	return Issue{Warn, "memory stall " + percent(n.MemoryStall)}, n.MemoryStall > 0.1
}

func (n Node) Issues() []Issue {
	found := issues(n, nodeRules)
	for _, condition := range n.Conditions {
		found = append(found, Issue{Bad, condition})
	}
	return found
}

func issues[T any](subject T, rules []func(T) (Issue, bool)) []Issue {
	var found []Issue
	for _, rule := range rules {
		if issue, ok := rule(subject); ok {
			found = append(found, issue)
		}
	}
	return found
}

func percent(fraction float64) string { return fmt.Sprintf("%.0f%%", math.Round(fraction*100)) }
