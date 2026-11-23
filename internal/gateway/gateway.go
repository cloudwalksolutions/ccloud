package gateway

import (
	"bufio"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cloudwalksolutions/ccloud/internal/app"
)

type Entry struct {
	Time      time.Time `json:"time"`
	Namespace string    `json:"namespace"`
	Pod       string    `json:"pod"`
	Container string    `json:"container"`
	Stream    string    `json:"stream"`
	Level     string    `json:"level"`
	Line      string    `json:"line"`
}

type Gateway struct {
	URL         *url.URL
	AccessToken string
	IDToken     string
	Timeout     time.Duration
}

type Refused struct {
	Status int
	Reason string
}

func (r Refused) Error() string {
	return fmt.Sprintf("the gateway answered %d %s: %s", r.Status, http.StatusText(r.Status), r.Reason)
}

type Page struct {
	Entries   []Entry `json:"entries"`
	Truncated bool    `json:"truncated"`
}

func (g Gateway) Read(ctx context.Context, query url.Values) (Page, error) {
	var page Page
	err := g.get(ctx, "/v1/logs", query, "logs", &page)
	return page, err
}

func (g Gateway) Containers(ctx context.Context, query url.Values) ([]string, error) {
	var answer struct {
		Containers []string `json:"containers"`
	}
	err := g.get(ctx, "/v1/logs/containers", query, "containers", &answer)
	return answer.Containers, err
}

func (g Gateway) Environments(ctx context.Context, named string) ([]app.Environment, error) {
	var answer struct {
		Environments []app.Environment `json:"environments"`
	}
	err := g.get(ctx, appPath(named, ""), nil, "environments", &answer)
	return answer.Environments, err
}

func appPath(named, rest string) string { return "/v1/apps/" + url.PathEscape(named) + rest }

func endpoint(path string, query url.Values) *url.URL {
	return &url.URL{Path: path, RawQuery: query.Encode()}
}

var ErrNotLogs = errors.New("the gateway answered something other than logs")

func (g Gateway) Stream(ctx context.Context, query url.Values, each func(Entry) error) (string, error) {
	resp, err := g.open(ctx, endpoint("/v1/logs/stream", query))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	lines := bufio.NewScanner(resp.Body)
	lines.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for lines.Scan() {
		var line struct {
			Entry
			End string `json:"end"`
		}
		if err := json.Unmarshal(lines.Bytes(), &line); err != nil {
			return "", fmt.Errorf("%w: %v", ErrNotLogs, err)
		}
		if line.End != "" {
			return line.End, nil
		}
		if line.Time.IsZero() {
			continue
		}
		if err := each(line.Entry); err != nil {
			return "", err
		}
	}
	return "", cmp.Or(lines.Err(), io.ErrUnexpectedEOF)
}

func (g Gateway) timeout() time.Duration {
	return cmp.Or(g.Timeout, 15*time.Second)
}

func (g Gateway) get(ctx context.Context, path string, query url.Values, what string, into any) error {
	timeout := g.timeout()
	answering, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	resp, err := g.open(answering, endpoint(path, query))
	if ctx.Err() == nil && errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("the gateway did not answer within %s", timeout)
	}
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		return fmt.Errorf("the gateway answered something other than %s: %w", what, err)
	}
	return nil
}

func (g Gateway) open(ctx context.Context, path *url.URL) (*http.Response, error) {
	req := (&http.Request{Method: http.MethodGet, URL: g.URL.ResolveReference(path), Header: http.Header{
		"Cf-Access-Token": {g.AccessToken},
		"Authorization":   {"Bearer " + g.IDToken},
	}}).WithContext(ctx)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, Refused{Status: resp.StatusCode, Reason: strings.TrimSpace(string(body))}
	}
	return resp, nil
}
