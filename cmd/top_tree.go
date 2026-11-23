package cmd

import (
	"fmt"
	"io"
	"slices"
	"strconv"

	"github.com/cloudwalksolutions/ccloud/internal/app"
)

type branch struct {
	label    string
	cells    []string
	issues   string
	children []*branch
}

func printTree(out io.Writer, named string, usages []regionUsage, verbose bool) error {
	color := paletteFor(out)
	slices.SortStableFunc(usages, func(a, b regionUsage) int {
		return app.Environment{Name: a.usage.Name}.Compare(app.Environment{Name: b.usage.Name})
	})
	regions := map[string]bool{}
	for _, u := range usages {
		regions[u.region.Name] = true
	}
	var all tally
	root := &branch{label: named}
	for _, u := range usages {
		label := u.usage.Name
		if len(regions) > 1 {
			label += " · " + u.region.Name
		}
		environment, used := environmentBranch(label, u.usage, color, verbose)
		all.merge(used)
		root.children = append(root.children, environment)
	}
	root.cells = all.cells(color)
	table := newTable(out, "APP · ENV · WORKLOAD", "PODS", "CPU", "%REQ", "%LIM", "MEM", "%REQ", "%LIM", "ISSUES")
	table.rightAlign(1, 2, 3, 4, 5, 6, 7)
	root.render(table, "", "", bool(color))
	if err := table.print(); err != nil || !color {
		return err
	}
	legend := fmt.Sprintf("memory: yellow ≥%s of its limit, red ≥%s · dim: no limit set · -v shows each container",
		percent(app.MemoryPressure.Warn), percent(app.MemoryPressure.Bad))
	_, err := fmt.Fprintln(out, color.severe(app.Hint, legend))
	return err
}

func environmentBranch(label string, usage app.Usage, color palette, verbose bool) (*branch, tally) {
	var used tally
	environment := &branch{label: label}
	for _, w := range usage.Workloads() {
		workload, load := workloadBranch(w, color, verbose)
		used.merge(load)
		environment.children = append(environment.children, workload)
	}
	for _, n := range usage.Nodes {
		if issues := n.Issues(); len(issues) > 0 {
			environment.children = append(environment.children, &branch{label: n.Name + " (node)", issues: color.issues(issues)})
		}
	}
	environment.cells = used.cells(color)
	return environment, used
}

func workloadBranch(w app.WorkloadUsage, color palette, verbose bool) (*branch, tally) {
	var load tally
	var issues []app.Issue
	workload := &branch{label: w.Name + " (" + w.Kind + ")"}
	for _, c := range w.Containers {
		load.add(c)
		issues = append(issues, c.Issues()...)
		if verbose {
			var one tally
			one.add(c)
			cells := one.cells(color)
			cells[0] = ""
			workload.children = append(workload.children, &branch{label: c.Pod + "/" + c.Container + " on " + c.Node, cells: cells, issues: color.issues(c.Issues())})
		}
	}
	workload.cells, workload.issues = load.cells(color), color.issues(issues)
	return workload, load
}

func (b *branch) render(t *table, lead, indent string, drawn bool) {
	cells := b.cells
	if cells == nil {
		cells = make([]string, 7)
	}
	t.row(append(append([]string{lead + b.label}, cells...), b.issues)...)
	for i, child := range b.children {
		switch last := i == len(b.children)-1; {
		case !drawn:
			child.render(t, indent+"  ", indent+"  ", drawn)
		case last:
			child.render(t, indent+"└─ ", indent+"   ", drawn)
		default:
			child.render(t, indent+"├─ ", indent+"│  ", drawn)
		}
	}
}

type measure struct {
	used, request, usedOfRequest, limit, usedOfLimit float64
}

func (m *measure) add(a app.Amount) {
	m.merge(measure{used: a.Used})
	if a.Request != nil {
		m.merge(measure{request: *a.Request, usedOfRequest: a.Used})
	}
	if a.Limit != nil {
		m.merge(measure{limit: *a.Limit, usedOfLimit: a.Used})
	}
}

func (m *measure) merge(other measure) {
	m.used += other.used
	m.request += other.request
	m.usedOfRequest += other.usedOfRequest
	m.limit += other.limit
	m.usedOfLimit += other.usedOfLimit
}

type tally struct {
	pods        map[string]bool
	cpu, memory measure
}

func (t *tally) add(c app.Container) {
	t.merge(tally{pods: map[string]bool{c.Pod: true}})
	t.cpu.add(c.CPU)
	t.memory.add(c.Memory)
}

func (t *tally) merge(other tally) {
	if t.pods == nil {
		t.pods = map[string]bool{}
	}
	for pod := range other.pods {
		t.pods[pod] = true
	}
	t.cpu.merge(other.cpu)
	t.memory.merge(other.memory)
}

func (t tally) cells(color palette) []string {
	return []string{strconv.Itoa(len(t.pods)),
		millicores(t.cpu.used), share(t.cpu.usedOfRequest, t.cpu.request), share(t.cpu.usedOfLimit, t.cpu.limit),
		memory(t.memory.used), share(t.memory.usedOfRequest, t.memory.request), color.pressure(t.memory.usedOfLimit, t.memory.limit)}
}

func share(used, of float64) string {
	if of == 0 {
		return "—"
	}
	return percent(used / of)
}

func (p palette) pressure(used, limit float64) string {
	if limit == 0 {
		return "—"
	}
	severity, _ := app.MemoryPressure.Of(used / limit)
	return p.severe(severity, percent(used/limit))
}

func millicores(cores float64) string {
	if cores < 1 {
		return fmt.Sprintf("%.0fm", cores*1000)
	}
	return fmt.Sprintf("%.1f", cores)
}
