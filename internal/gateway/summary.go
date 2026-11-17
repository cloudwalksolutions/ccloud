package gateway

import (
	"context"
	"net/url"
	"strconv"
)

type Check struct {
	Flow   string `json:"flow"`
	Status string `json:"status"`
	Run    string `json:"run"`
}

type Pull struct {
	Number         int     `json:"number"`
	Title          string  `json:"title"`
	Author         string  `json:"author"`
	Branch         string  `json:"branch"`
	OpenedHoursAgo float64 `json:"openedHoursAgo"`
	Draft          bool    `json:"draft"`
	Reviewed       bool    `json:"reviewed"`
	Environment    string  `json:"environment"`
	Checks         []Check `json:"checks"`
}

type PullFlow struct {
	Merged           int     `json:"merged"`
	OpenToMergeHours float64 `json:"openToMergeHours"`
	FirstReviewHours float64 `json:"firstReviewHours"`
}

type Delivery struct {
	Deploys          int     `json:"deploys"`
	MergeToProdHours float64 `json:"mergeToProdHours"`
}

type CIHealth struct {
	Flow             string  `json:"flow"`
	Runs             int     `json:"runs"`
	MedianSeconds    float64 `json:"medianSeconds"`
	FirstTryPassRate float64 `json:"firstTryPassRate"`
	Reruns           int     `json:"reruns"`
}

type Summary struct {
	Pulls    []Pull     `json:"pulls"`
	Flow     PullFlow   `json:"flow"`
	Delivery Delivery   `json:"delivery"`
	CI       []CIHealth `json:"ci"`
}

func (g Gateway) Summary(ctx context.Context, app string, weeks int) (Summary, error) {
	var answer Summary
	query := url.Values{"weeks": {strconv.Itoa(weeks)}}
	err := g.get(ctx, &url.URL{Path: "/v1/apps/" + url.PathEscape(app) + "/summary", RawQuery: query.Encode()}, "a summary", &answer)
	return answer, err
}
