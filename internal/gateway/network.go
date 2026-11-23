package gateway

import (
	"context"
	"net/url"
	"time"

	"github.com/cloudwalksolutions/ccloud/internal/app"
)

func (g Gateway) Network(ctx context.Context, appName, env string, since time.Duration) (app.Traffic, error) {
	var answer app.Traffic
	query := url.Values{"env": {env}, "since": {since.String()}}
	err := g.get(ctx, appPath(appName, "/network"), query, "network traffic", &answer)
	return answer, err
}

func (g Gateway) EveryNetwork(ctx context.Context, appName string, since time.Duration) ([]app.EnvironmentTraffic, error) {
	var answer struct {
		Environments []app.EnvironmentTraffic `json:"environments"`
	}
	err := g.get(ctx, appPath(appName, "/network"), url.Values{"since": {since.String()}}, "network traffic", &answer)
	return answer.Environments, err
}
