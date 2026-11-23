package gateway

import (
	"context"
	"net/url"
	"time"
)

type Ingress struct {
	RequestsPerSecond float64 `json:"requestsPerSecond"`
	ServerErrors      float64 `json:"serverErrors"`
	P95Seconds        float64 `json:"p95Seconds"`
}

type Egress struct {
	Destination string  `json:"destination"`
	Port        int     `json:"port"`
	Connections float64 `json:"connections"`
	Unanswered  float64 `json:"unanswered"`
	Reset       float64 `json:"reset"`
}

type Drop struct {
	Destination string  `json:"destination"`
	Port        int     `json:"port"`
	Reason      string  `json:"reason"`
	Count       float64 `json:"count"`
}

type Traffic struct {
	Ingress Ingress  `json:"ingress"`
	Egress  []Egress `json:"egress"`
	Drops   []Drop   `json:"drops"`
}

func (g Gateway) Network(ctx context.Context, app, env string, since time.Duration) (Traffic, error) {
	var answer Traffic
	query := url.Values{"env": {env}, "since": {since.String()}}
	err := g.get(ctx, appPath(app, "/network"), query, "network traffic", &answer)
	return answer, err
}
