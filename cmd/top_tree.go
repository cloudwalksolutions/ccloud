package cmd

import (
	"cmp"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/cloudwalksolutions/ccloud/internal/gateway"
)

type branch struct {
	label    string
	cells    []string
	issues   string
	children []*branch
}

func printTree(out io.Writer, app string, usages []regionUsage, verbose bool) error {
	color := paletteFor(out)
	slices.SortStableFunc(usages, func(a, b regionUsage) int { return byEnvironment(a.usage.Name, b.usage.Name) })
	regions := map[string]bool{}
	var all tally
	root := &branch{label: app}
	for _, u := range usages {
		regions[u.region.Name] = true
	}
	for _, u := range usages {
		label := u.usage.Name
		if len(regions) > 1 {
			label += " · " + u.region.Name
		}
		var env tally
		environment := &branch{label: label}
		for _, w := range workloadsOf(u.usage.Containers) {
			var load tally
			var issues []issue
			workload := &branch{label: w.label}
			for _, c := range w.containers {
				load.add(c)
				issues = append(issues, containerIssues(c)...)
				if verbose {
					var one tally
					one.add(c)
					cells := one.cells(color)
					cells[0] = ""
					workload.children = append(workload.children, &branch{label: c.Pod + "/" + c.Container + " on " + c.Node, cells: cells, issues: color.issues(containerIssues(c))})
				}
			}
			env.merge(load)
			workload.cells, workload.issues = load.cells(color), color.issues(issues)
			environment.children = append(environment.children, workload)
		}
		for _, n := range u.usage.Nodes {
			if issues := nodeIssues(n); len(issues) > 0 {
				environment.children = append(environment.children, &branch{label: n.Name + " (node)", issues: color.issues(issues)})
			}
		}
		environment.cells = env.cells(color)
		all.merge(env)
		root.children = append(root.children, environment)
	}
	root.cells = all.cells(color)
	table := newTable(out, "APP · ENV · WORKLOAD", "PODS", "CPU", "%REQ", "%LIM", "MEM", "%REQ", "%LIM", "ISSUES")
	table.rightAlign(1, 2, 3, 4, 5, 6, 7)
	root.render(table, "", "", bool(color))
	if err := table.print(); err != nil || !color {
		return err
	}
	_, err := fmt.Fprintln(out, color.severe(hint, "memory: yellow ≥75% of its limit, red ≥90% · dim: no limit set · -v shows each container"))
	return err
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

type workloadGroup struct {
	label      string
	containers []gateway.Container
}

func workloadsOf(containers []gateway.Container) []workloadGroup {
	groups := map[string]*workloadGroup{}
	for _, c := range containers {
		kind, name := cmp.Or(c.Workload.Kind, "Pod"), cmp.Or(c.Workload.Name, c.Pod)
		label := name + " (" + kind + ")"
		if groups[label] == nil {
			groups[label] = &workloadGroup{label: label}
		}
		groups[label].containers = append(groups[label].containers, c)
	}
	sorted := make([]workloadGroup, 0, len(groups))
	for _, g := range groups {
		sorted = append(sorted, *g)
	}
	slices.SortFunc(sorted, func(a, b workloadGroup) int { return strings.Compare(a.label, b.label) })
	return sorted
}

type measure struct {
	used, request, usedOfRequest, limit, usedOfLimit float64
}

func (m *measure) add(a gateway.Amount) {
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

func (t *tally) add(c gateway.Container) {
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
	return p.severe(severityOf(used/limit, 0.75, 0.9), percent(used/limit))
}

func millicores(cores float64) string {
	if cores < 1 {
		return fmt.Sprintf("%.0fm", cores*1000)
	}
	return fmt.Sprintf("%.1f", cores)
}

func byEnvironment(a, b string) int {
	rank := func(env string) int {
		if i := slices.Index([]string{"prod", "stage", "ci"}, env); i >= 0 {
			return i
		}
		return 3
	}
	return cmp.Or(cmp.Compare(rank(a), rank(b)), natural(a, b))
}

func natural(a, b string) int {
	nameA, numberA := trailingNumber(a)
	nameB, numberB := trailingNumber(b)
	return cmp.Or(strings.Compare(nameA, nameB), cmp.Compare(numberA, numberB))
}

func trailingNumber(name string) (string, int) {
	prefix := strings.TrimRight(name, "0123456789")
	number, _ := strconv.Atoi(name[len(prefix):])
	return prefix, number
}
