// Package github is the loop's small GitHub client. It holds only what the
// loop needs: create an issue or add a comment to one the loop filed (after
// a human approved the draft, #369) and read how an issue was closed (#374). It cannot merge, label existing
// issues, push or change settings, and it is given a token scoped to
// issues only (docs/backend/features/feedback-loop.md).
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Client talks to the GitHub REST/GraphQL API.
type Client struct {
	Token string
	// BaseURL: "" = https://api.github.com
	BaseURL string
	HTTP    *http.Client
}

var reRepo = regexp.MustCompile(`^[A-Za-z0-9_.\-]+/[A-Za-z0-9_.\-]+$`)

// ValidRepo reports whether repo is "owner/name".
func ValidRepo(repo string) bool { return reRepo.MatchString(repo) }

func (c *Client) base() string {
	if c.BaseURL != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}
	return "https://api.github.com"
}

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	if c.Token == "" {
		return errors.New("github: no token")
	}
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base()+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("github: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		return fmt.Errorf("github: %s %s: status %d", method, path, resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(out)
}

// Issue is a created issue.
type Issue struct {
	Number int    `json:"number"`
	URL    string `json:"html_url"`
}

// CreateIssue creates one issue in repo ("owner/name").
func (c *Client) CreateIssue(ctx context.Context, repo, title, body string, labels []string) (*Issue, error) {
	if !ValidRepo(repo) {
		return nil, fmt.Errorf("github: bad repo %q", repo)
	}
	in := map[string]any{"title": title, "body": body}
	if len(labels) > 0 {
		in["labels"] = labels
	}
	var out Issue
	if err := c.do(ctx, http.MethodPost, "/repos/"+repo+"/issues", in, &out); err != nil {
		return nil, err
	}
	if out.Number <= 0 {
		return nil, errors.New("github: no issue number in the response")
	}
	return &out, nil
}

// CommentIssue adds body as a comment to issue n in repo (a draft whose
// cluster already has an issue, #369). The returned Issue is that issue,
// with URL pointing at the new comment.
func (c *Client) CommentIssue(ctx context.Context, repo string, n int, body string) (*Issue, error) {
	if !ValidRepo(repo) {
		return nil, fmt.Errorf("github: bad repo %q", repo)
	}
	if n <= 0 {
		return nil, fmt.Errorf("github: bad issue number %d", n)
	}
	var out struct {
		ID  int64  `json:"id"`
		URL string `json:"html_url"`
	}
	if err := c.do(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/issues/%d/comments", repo, n), map[string]any{"body": body}, &out); err != nil {
		return nil, err
	}
	if out.ID <= 0 {
		return nil, errors.New("github: no comment id in the response")
	}
	return &Issue{Number: n, URL: out.URL}, nil
}
