package app

import (
	"cmp"
	"math"
	"slices"
	"strings"
)

type Host struct {
	Host              string  `json:"host"`
	Service           string  `json:"service"`
	RequestsPerSecond float64 `json:"requestsPerSecond"`
	ServerErrors      float64 `json:"serverErrors"`
	P95Seconds        float64 `json:"p95Seconds"`
}

type Ingress struct {
	RequestsPerSecond float64 `json:"requestsPerSecond"`
	ServerErrors      float64 `json:"serverErrors"`
	P95Seconds        float64 `json:"p95Seconds"`
	Hosts             []Host  `json:"hosts"`
}

type Egress struct {
	Workload    Workload `json:"workload"`
	Destination string   `json:"destination"`
	Port        int      `json:"port"`
	Connections float64  `json:"connections"`
	Unanswered  float64  `json:"unanswered"`
	Reset       float64  `json:"reset"`
}

type Drop struct {
	Workload    Workload `json:"workload"`
	Destination string   `json:"destination"`
	Port        int      `json:"port"`
	Reason      string   `json:"reason"`
	Count       float64  `json:"count"`
}

type Traffic struct {
	Ingress Ingress  `json:"ingress"`
	Egress  []Egress `json:"egress"`
	Drops   []Drop   `json:"drops"`
}

type EnvironmentTraffic struct {
	Name string `json:"name"`
	Traffic
}

func (t Traffic) Hosts() int {
	hosts := map[string]bool{}
	for _, e := range t.Egress {
		hosts[e.Destination] = true
	}
	return len(hosts)
}

func (t Traffic) Connections() (opened, unanswered float64) {
	for _, e := range t.Egress {
		opened += e.Connections
		unanswered += e.Unanswered
	}
	return opened, unanswered
}

func (t Traffic) Dropped() float64 {
	var dropped float64
	for _, d := range t.Drops {
		dropped += d.Count
	}
	return dropped
}

var (
	ServerErroring = Graded{Warn: math.SmallestNonzeroFloat64, Bad: 0.01}
	Slow           = Graded{Warn: 0.5, Bad: 2}
	Unanswering    = Graded{Warn: math.SmallestNonzeroFloat64, Bad: 0.1}
	Resetting      = Graded{Warn: 0.05, Bad: 0.2}
)

type ByWorkload[T any] struct {
	Workload
	Rows []T
}

func (t Traffic) EgressByWorkload() []ByWorkload[Egress] {
	return grouped(t.Egress, func(e Egress) (Workload, float64) { return e.Workload, e.Connections })
}

func (t Traffic) DropsByWorkload() []ByWorkload[Drop] {
	return grouped(t.Drops, func(d Drop) (Workload, float64) { return d.Workload, d.Count })
}

func grouped[T any](rows []T, of func(T) (Workload, float64)) []ByWorkload[T] {
	var groups []ByWorkload[T]
	for _, row := range rows {
		w, _ := of(row)
		i := slices.IndexFunc(groups, func(g ByWorkload[T]) bool { return g.Workload == w })
		if i < 0 {
			groups = append(groups, ByWorkload[T]{Workload: w})
			i = len(groups) - 1
		}
		groups[i].Rows = append(groups[i].Rows, row)
	}
	for _, g := range groups {
		slices.SortStableFunc(g.Rows, func(a, b T) int {
			_, countA := of(a)
			_, countB := of(b)
			return cmp.Compare(countB, countA)
		})
	}
	slices.SortFunc(groups, func(a, b ByWorkload[T]) int { return strings.Compare(a.Name, b.Name) })
	return groups
}
