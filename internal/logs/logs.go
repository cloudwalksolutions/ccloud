package logs

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
	return fmt.Sprintf("the logs gateway answered %d %s: %s", r.Status, http.StatusText(r.Status), r.Reason)
}

func (g Gateway) Read(ctx context.Context, query url.Values) ([]Entry, error) {
	address := g.URL.ResolveReference(&url.URL{Path: "/v1/logs", RawQuery: query.Encode()})
	req := (&http.Request{Method: http.MethodGet, URL: address, Header: http.Header{
		"Cf-Access-Token": {g.AccessToken},
		"Authorization":   {"Bearer " + g.IDToken},
	}}).WithContext(ctx)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, Refused{Status: resp.StatusCode, Reason: strings.TrimSpace(string(body))}
	}
	var answer struct {
		Entries []Entry `json:"entries"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&answer); err != nil {
		return nil, fmt.Errorf("the logs gateway answered something other than logs: %w", err)
	}
	return answer.Entries, nil
}
