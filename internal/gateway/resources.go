package gateway

import (
	"context"
	"net/url"

	"github.com/cloudwalksolutions/ccloud/internal/app"
)

func (g Gateway) Resources(ctx context.Context, named, env string) ([]app.Usage, error) {
	var answer struct {
		Environments []app.Usage `json:"environments"`
	}
	query := url.Values{}
	if env != "" {
		query.Set("env", env)
	}
	err := g.get(ctx, &url.URL{Path: "/v1/apps/" + url.PathEscape(named) + "/resources", RawQuery: query.Encode()}, "resources", &answer)
	return answer.Environments, err
}
