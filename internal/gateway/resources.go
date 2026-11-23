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
	err := g.get(ctx, appPath(named, "/resources"), query, "resources", &answer)
	return answer.Environments, err
}
