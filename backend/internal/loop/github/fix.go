package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Fix tells how an issue ended.
type Fix struct {
	// Fixed: closed as completed, and a merged PR closed it or is linked to it.
	Fixed bool
	PR    int
	At    time.Time // when the issue was closed
}

const fixQuery = `query($owner: String!, $name: String!, $n: Int!) {
  repository(owner: $owner, name: $name) {
    issue(number: $n) {
      state
      stateReason
      closedAt
      timelineItems(itemTypes: [CLOSED_EVENT], last: 1) {
        nodes { ... on ClosedEvent { closer { __typename ... on PullRequest { number merged } } } }
      }
      closedByPullRequestsReferences(first: 10, includeClosedPrs: true) {
        nodes { number merged }
      }
    }
  }
}`

type pr struct {
	Number int  `json:"number"`
	Merged bool `json:"merged"`
}

type fixResponse struct {
	Data struct {
		Repository struct {
			Issue *struct {
				State         string    `json:"state"`
				StateReason   string    `json:"stateReason"`
				ClosedAt      time.Time `json:"closedAt"`
				TimelineItems struct {
					Nodes []struct {
						Closer *struct {
							Typename string `json:"__typename"`
							pr
						} `json:"closer"`
					} `json:"nodes"`
				} `json:"timelineItems"`
				ClosedBy struct {
					Nodes []pr `json:"nodes"`
				} `json:"closedByPullRequestsReferences"`
			} `json:"issue"`
		} `json:"repository"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// IssueFix reads whether repo#n was fixed by a merged PR. PRs into a branch
// other than the default one do not close issues by themselves, so a person
// may close the issue by hand: then a merged PR linked to it counts too.
func (c *Client) IssueFix(ctx context.Context, repo string, n int) (Fix, error) {
	if !ValidRepo(repo) || n <= 0 {
		return Fix{}, fmt.Errorf("github: bad issue %s#%d", repo, n)
	}
	owner, name, _ := strings.Cut(repo, "/")
	var out fixResponse
	in := map[string]any{"query": fixQuery, "variables": map[string]any{"owner": owner, "name": name, "n": n}}
	if err := c.do(ctx, http.MethodPost, "/graphql", in, &out); err != nil {
		return Fix{}, err
	}
	if len(out.Errors) > 0 {
		return Fix{}, errors.New("github: graphql error")
	}
	iss := out.Data.Repository.Issue
	if iss == nil {
		return Fix{}, fmt.Errorf("github: %s#%d not found", repo, n)
	}
	if iss.State != "CLOSED" || iss.StateReason == "NOT_PLANNED" || iss.StateReason == "DUPLICATE" {
		return Fix{}, nil
	}
	for _, node := range iss.TimelineItems.Nodes {
		if cl := node.Closer; cl != nil && cl.Typename == "PullRequest" && cl.Merged {
			return Fix{Fixed: true, PR: cl.Number, At: iss.ClosedAt}, nil
		}
	}
	for _, p := range iss.ClosedBy.Nodes {
		if p.Merged {
			return Fix{Fixed: true, PR: p.Number, At: iss.ClosedAt}, nil
		}
	}
	return Fix{}, nil
}
