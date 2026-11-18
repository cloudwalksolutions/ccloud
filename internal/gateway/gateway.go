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

type Page struct {
	Entries   []Entry `json:"entries"`
	Truncated bool    `json:"truncated"`
}

func (g Gateway) Read(ctx context.Context, query url.Values) (Page, error) {
	var page Page
	err := g.get(ctx, &url.URL{Path: "/v1/logs", RawQuery: query.Encode()}, "logs", &page)
	return page, err
}

func (g Gateway) Containers(ctx context.Context, query url.Values) ([]string, error) {
	var answer struct {
		Containers []string `json:"containers"`
	}
	err := g.get(ctx, &url.URL{Path: "/v1/logs/containers", RawQuery: query.Encode()}, "containers", &answer)
	return answer.Containers, err
}

func (g Gateway) Environments(ctx context.Context, app string) ([]Environment, error) {
	var answer struct {
		Environments []Environment `json:"environments"`
	}
	err := g.get(ctx, &url.URL{Path: "/v1/apps/" + url.PathEscape(app)}, "environments", &answer)
	return answer.Environments, err
}

var ErrNotLogs = errors.New("the gateway answered something other than logs")

func (g Gateway) Stream(ctx context.Context, query url.Values, each func(Entry) error) (string, error) {
	resp, err := g.open(ctx, &url.URL{Path: "/v1/logs/stream", RawQuery: query.Encode()})
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

func (g Gateway) get(ctx context.Context, path *url.URL, what string, into any) error {
	resp, err := g.open(ctx, path)
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
