package secretmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type Client struct {
	URL, Token string
}

type StatusError struct {
	Code int
}

func (e StatusError) Error() string { return fmt.Sprintf("secret manager answered %d", e.Code) }

func (c Client) Access(ctx context.Context, project, name string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/v1/projects/%s/secrets/%s/versions/latest:access", c.URL, project, name), nil)
	var response *http.Response
	if err == nil {
		request.Header.Set("Authorization", "Bearer "+c.Token)
		response, err = http.DefaultClient.Do(request)
	}
	if err != nil {
		return "", err
	}
	defer func() { _ = response.Body.Close() }()
	var version struct {
		Payload struct {
			Data []byte `json:"data"`
		} `json:"payload"`
	}
	if response.StatusCode != http.StatusOK {
		return "", StatusError{Code: response.StatusCode}
	}
	err = json.NewDecoder(response.Body).Decode(&version)
	return string(version.Payload.Data), err
}
