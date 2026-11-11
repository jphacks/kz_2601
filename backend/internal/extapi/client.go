package extapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Client is appconfig's side of the runner's /internal/ext/* routes. HTTP
// carries the service-to-service auth (an OIDC ID token for the runner's
// URL on Cloud Run); appconfig never holds a provider key or a token.
type Client struct {
	// BaseURL is the runner's origin (EXT_RUNNER_URL), without a trailing /.
	BaseURL string
	HTTP    *http.Client
}

// RunnerError is a runner answer with a reason (StatusOf(Reason) status).
type RunnerError struct {
	Status int
	Reason string
}

func (e *RunnerError) Error() string { return fmt.Sprintf("runner %d: %s", e.Status, e.Reason) }

// ErrRunnerAnswer: the runner answered something that is not its JSON.
var ErrRunnerAnswer = errors.New("extapi: unexpected runner answer")

// maxRunnerAnswer bounds what appconfig reads back (an op's answer is
// already capped at MaxBody before it is shaped into rows).
const maxRunnerAnswer = 2 << 20

func (c *Client) post(ctx context.Context, path string, body, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxRunnerAnswer+1))
	if err != nil {
		return err
	}
	if len(raw) > maxRunnerAnswer {
		return fmt.Errorf("%w: body over %d bytes", ErrRunnerAnswer, maxRunnerAnswer)
	}
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Reason string `json:"reason"`
		}
		if json.Unmarshal(raw, &e) != nil || e.Reason == "" {
			return fmt.Errorf("%w: status %d", ErrRunnerAnswer, resp.StatusCode)
		}
		return &RunnerError{Status: resp.StatusCode, Reason: e.Reason}
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("%w: %v", ErrRunnerAnswer, err)
	}
	return nil
}

// Call is POST /internal/ext/call.
func (c *Client) Call(ctx context.Context, b CallBody) (CallAnswer, error) {
	var out CallAnswer
	err := c.post(ctx, "/internal/ext/call", b, &out)
	return out, err
}

// Start is POST /internal/ext/connections/start: the consent URL.
func (c *Client) Start(ctx context.Context, uid, provider string, scopes []string) (string, error) {
	var out struct {
		AuthURL string `json:"authUrl"`
	}
	if err := c.post(ctx, "/internal/ext/connections/start", ConnBody{UID: uid, Provider: provider, Scopes: scopes}, &out); err != nil {
		return "", err
	}
	if out.AuthURL == "" {
		return "", fmt.Errorf("%w: no authUrl", ErrRunnerAnswer)
	}
	return out.AuthURL, nil
}

// Revoke is POST /internal/ext/connections/revoke.
func (c *Client) Revoke(ctx context.Context, uid, provider string) error {
	var out map[string]bool
	return c.post(ctx, "/internal/ext/connections/revoke", ConnBody{UID: uid, Provider: provider}, &out)
}

// List is POST /internal/ext/connections/list (no tokens).
func (c *Client) List(ctx context.Context, uid string) ([]ConnectionView, error) {
	var out struct {
		Connections []ConnectionView `json:"connections"`
	}
	if err := c.post(ctx, "/internal/ext/connections/list", ConnBody{UID: uid}, &out); err != nil {
		return nil, err
	}
	if out.Connections == nil {
		out.Connections = []ConnectionView{}
	}
	return out.Connections, nil
}
