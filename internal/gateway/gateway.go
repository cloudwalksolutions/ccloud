package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Entry struct {
	Time      time.Time `json:"time"`
	Namespace string    `json:"namespace"`
	Pod       string    `json:"pod"`
	Container string    `json:"container"`
	Line      string    `json:"line"`
}

type Gateway struct {
	URL         *url.URL
	AccessToken string
	IDToken     string
}

type Refused struct {
	Status int
	Reason string
}

func (r Refused) Error() string {
	return fmt.Sprintf("the gateway answered %d %s: %s", r.Status, http.StatusText(r.Status), r.Reason)
}

type Environment struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	URL       string `json:"url,omitempty"`
}

func (g Gateway) Read(ctx context.Context, query url.Values) ([]Entry, error) {
	var answer struct {
		Entries []Entry `json:"entries"`
	}
	err := g.get(ctx, &url.URL{Path: "/v1/logs", RawQuery: query.Encode()}, "logs", &answer)
	return answer.Entries, err
}

func (g Gateway) Environments(ctx context.Context, app string) ([]Environment, error) {
	var answer struct {
		Environments []Environment `json:"environments"`
	}
	err := g.get(ctx, &url.URL{Path: "/v1/apps/" + url.PathEscape(app)}, "environments", &answer)
	return answer.Environments, err
}

func (g Gateway) get(ctx context.Context, path *url.URL, what string, into any) error {
	req := (&http.Request{Method: http.MethodGet, URL: g.URL.ResolveReference(path), Header: http.Header{
		"Cf-Access-Token": {g.AccessToken},
		"Authorization":   {"Bearer " + g.IDToken},
	}}).WithContext(ctx)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return Refused{Status: resp.StatusCode, Reason: strings.TrimSpace(string(body))}
	}
	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		return fmt.Errorf("the gateway answered something other than %s: %w", what, err)
	}
	return nil
}
