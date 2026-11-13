package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"
)

// OpenIssue is what the loop reads of an open issue (#2288): never the body.
type OpenIssue struct {
	Number int
	Title  string
	URL    string
	Labels []string
}

// OpenIssuesOptions bounds OpenIssues.
type OpenIssuesOptions struct {
	// Limit: open issues read at most (excluded ones count too). 0 = DefaultOpenIssuesLimit.
	Limit int
	// ExcludeLabels: issues with any of these (the loop's LOOP_ISSUE_LABELS) are left out. Case is ignored.
	ExcludeLabels []string
	// ExcludeNumbers: issues the loop filed (drafts' issue numbers).
	ExcludeNumbers []int
}

const (
	DefaultOpenIssuesLimit = 500
	MaxOpenIssuesLimit     = 2000
	openIssuesPage         = 100
	maxTitleRunes          = 256
)

// Only number, title, url and label names: the body is never requested.
const openIssuesQuery = `query($owner: String!, $name: String!, $first: Int!, $after: String) {
  repository(owner: $owner, name: $name) {
    issues(states: OPEN, first: $first, after: $after, orderBy: {field: CREATED_AT, direction: DESC}) {
      pageInfo { hasNextPage endCursor }
      nodes { number title url labels(first: 20) { nodes { name } } }
    }
  }
}`

type openIssuesResponse struct {
	Data struct {
		Repository *struct {
			Issues struct {
				PageInfo struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
				Nodes []struct {
					Number int    `json:"number"`
					Title  string `json:"title"`
					URL    string `json:"url"`
					Labels struct {
						Nodes []struct {
							Name string `json:"name"`
						} `json:"nodes"`
					} `json:"labels"`
				} `json:"nodes"`
			} `json:"issues"`
		} `json:"repository"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

type issuesCache struct {
	mu   sync.Mutex
	byID map[string][]OpenIssue
}

// OpenIssues lists open issues of repo, newest first, without the ones the
// loop filed. The result is cached on c for the rest of the run (one draft
// run reads the repo once however many clusters it has).
func (c *Client) OpenIssues(ctx context.Context, repo string, opt OpenIssuesOptions) ([]OpenIssue, error) {
	if !ValidRepo(repo) {
		return nil, fmt.Errorf("github: bad repo %q", repo)
	}
	limit := opt.Limit
	if limit <= 0 {
		limit = DefaultOpenIssuesLimit
	}
	limit = min(limit, MaxOpenIssuesLimit)
	skipLabel := map[string]bool{}
	for _, l := range opt.ExcludeLabels {
		if l = strings.ToLower(strings.TrimSpace(l)); l != "" {
			skipLabel[l] = true
		}
	}
	key := cacheKey(repo, limit, skipLabel, opt.ExcludeNumbers)

	c.cache.mu.Lock()
	defer c.cache.mu.Unlock()
	if got, ok := c.cache.byID[key]; ok {
		return slices.Clone(got), nil
	}

	owner, name, _ := strings.Cut(repo, "/")
	out := []OpenIssue{}
	after, read := "", 0
	for read < limit {
		vars := map[string]any{"owner": owner, "name": name, "first": min(openIssuesPage, limit-read)}
		if after != "" {
			vars["after"] = after
		}
		var resp openIssuesResponse
		if err := c.do(ctx, http.MethodPost, "/graphql", map[string]any{"query": openIssuesQuery, "variables": vars}, &resp); err != nil {
			return nil, err
		}
		if len(resp.Errors) > 0 {
			return nil, errors.New("github: graphql error")
		}
		r := resp.Data.Repository
		if r == nil {
			return nil, fmt.Errorf("github: %s not found", repo)
		}
		for _, n := range r.Issues.Nodes {
			read++
			if n.Number <= 0 || slices.Contains(opt.ExcludeNumbers, n.Number) {
				continue
			}
			iss := OpenIssue{Number: n.Number, Title: clip(n.Title), URL: n.URL}
			skip := false
			for _, l := range n.Labels.Nodes {
				skip = skip || skipLabel[strings.ToLower(l.Name)]
				iss.Labels = append(iss.Labels, l.Name)
			}
			if !skip {
				out = append(out, iss)
			}
		}
		if !r.Issues.PageInfo.HasNextPage || r.Issues.PageInfo.EndCursor == "" || len(r.Issues.Nodes) == 0 {
			break
		}
		after = r.Issues.PageInfo.EndCursor
	}
	if c.cache.byID == nil {
		c.cache.byID = map[string][]OpenIssue{}
	}
	c.cache.byID[key] = out
	return slices.Clone(out), nil
}

func cacheKey(repo string, limit int, labels map[string]bool, nums []int) string {
	ls := make([]string, 0, len(labels))
	for l := range labels {
		ls = append(ls, l)
	}
	slices.Sort(ls)
	ns := slices.Clone(nums)
	slices.Sort(ns)
	return fmt.Sprintf("%s|%d|%s|%v", repo, limit, strings.Join(ls, ","), ns)
}

func clip(s string) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= maxTitleRunes {
		return s
	}
	return string([]rune(s)[:maxTitleRunes])
}
