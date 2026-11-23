package gateway

import (
	"context"
	"net/url"
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
	Namespace  string      `json:"namespace"`
	Containers []Container `json:"containers"`
	Nodes      []Node      `json:"nodes"`
}

func (g Gateway) Resources(ctx context.Context, app, env string) ([]Usage, error) {
	var answer struct {
		Environments []Usage `json:"environments"`
	}
	query := url.Values{}
	if env != "" {
		query.Set("env", env)
	}
	err := g.get(ctx, &url.URL{Path: "/v1/apps/" + url.PathEscape(app) + "/resources", RawQuery: query.Encode()}, "resources", &answer)
	return answer.Environments, err
}
