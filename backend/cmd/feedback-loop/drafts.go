package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/jphacks/kz_2601/backend/internal/loop"
	"github.com/jphacks/kz_2601/backend/internal/loop/draft"
	"github.com/jphacks/kz_2601/backend/internal/loop/github"
	"github.com/jphacks/kz_2601/backend/internal/loop/ingest"
)

// Env of the issue target. No repo = dry run: approving records the
// decision and prints the issue; nothing is posted.
const (
	envIssueRepo   = "LOOP_ISSUE_REPO"   // owner/name
	envGitHubToken = "LOOP_GITHUB_TOKEN" // issues: write only
	envIssueLabels = "LOOP_ISSUE_LABELS" // comma separated, default none
)

func (a *app) draftBuild(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("draft", flag.ContinueOnError)
	maxNew := fs.Int("max-new", 20, "max new drafts per run")
	if err := fs.Parse(args); err != nil {
		return err
	}
	res, err := (&draft.Builder{Store: a.loop, Now: a.now, MaxNew: *maxNew}).Run(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "draft: new %d, extended %d, items %d, held %d, left for next run %d\n", res.Created, res.Extended, res.Items, res.Held, res.Capped)
	return nil
}

func (a *app) reviewer() (*draft.Reviewer, error) {
	r := &draft.Reviewer{Store: a.loop, Now: a.now, Repo: strings.TrimSpace(a.getenv(envIssueRepo))}
	if r.Repo != "" && !github.ValidRepo(r.Repo) {
		return nil, fmt.Errorf("%s must be owner/name", envIssueRepo)
	}
	for l := range ingest.ParseList(a.getenv(envIssueLabels)) {
		r.Labels = append(r.Labels, l)
	}
	sort.Strings(r.Labels)
	switch {
	case a.publisher != nil:
		r.Publisher = a.publisher
	case a.getenv(envGitHubToken) != "":
		r.Publisher = &github.Client{Token: a.getenv(envGitHubToken)}
	}
	return r, nil
}

const draftsUsage = `usage: feedback-loop drafts <list|show|edit|approve|reject|publish>
  list [-status pending]
  show <id>
  edit -by NAME [-title T] [-body-file F] <id>
  approve -by NAME <id>        承認 1: posts to LOOP_ISSUE_REPO (none = dry run)
  reject -by NAME [-reason R] <id>
  publish <id>                 post an approved draft (after a dry run)
`

func (a *app) drafts(ctx context.Context, args []string) error {
	if len(args) == 0 {
		_, _ = fmt.Fprint(a.out, draftsUsage)
		return errors.New("drafts needs a subcommand")
	}
	sub, rest := args[0], args[1:]
	fs := flag.NewFlagSet("drafts "+sub, flag.ContinueOnError)
	status := fs.String("status", loop.DraftPending, "list: only this status (\"\" = all)")
	by := fs.String("by", "", "your name (recorded with the decision)")
	title := fs.String("title", "", "edit: new title")
	bodyFile := fs.String("body-file", "", "edit: file with the new body")
	reason := fs.String("reason", "", "reject: why (kept in the private queue only)")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	if sub == "list" {
		return a.draftsList(ctx, *status)
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("drafts %s needs exactly one id", sub)
	}
	id := fs.Arg(0)
	r, err := a.reviewer()
	if err != nil {
		return err
	}
	switch sub {
	case "show":
		d, err := a.loop.GetDraft(ctx, id)
		if err != nil {
			return err
		}
		reasons, err := r.CheckDraft(ctx, d)
		if err != nil {
			return err
		}
		a.printDraft(d, reasons)
		return nil
	case "edit":
		body := ""
		if *bodyFile != "" {
			b, err := os.ReadFile(*bodyFile)
			if err != nil {
				return err
			}
			body = string(b)
		}
		d, err := r.Edit(ctx, id, *by, *title, body)
		if err != nil {
			return err
		}
		fmt.Fprintf(a.out, "edited %s\n", d.ID)
		return nil
	case "reject":
		d, err := r.Reject(ctx, id, *by, *reason)
		if err != nil {
			return err
		}
		fmt.Fprintf(a.out, "rejected %s\n", d.ID)
		return nil
	case "approve":
		d, iss, err := r.Approve(ctx, id, *by)
		return a.afterPost("approved", d, iss, err)
	case "publish":
		d, iss, err := r.Publish(ctx, id)
		return a.afterPost("published", d, iss, err)
	}
	_, _ = fmt.Fprint(a.out, draftsUsage)
	return fmt.Errorf("unknown drafts command %q", sub)
}

func (a *app) afterPost(verb string, d *loop.Draft, iss *github.Issue, err error) error {
	if err != nil {
		return err
	}
	if iss == nil {
		fmt.Fprintf(a.out, "%s %s (dry run: %s is not set, nothing was posted). This is the issue that would be created:\n\n", verb, d.ID, envIssueRepo)
		fmt.Fprintf(a.out, "title: %s\n\n%s\n", d.Title, d.Body)
		return nil
	}
	fmt.Fprintf(a.out, "%s %s -> %s#%d %s\n", verb, d.ID, d.IssueRepo, iss.Number, iss.URL)
	fmt.Fprintln(a.out, "next: a person with write access adds agent:ready only if an agent should open a PR (#370)")
	return nil
}

func (a *app) draftsList(ctx context.Context, status string) error {
	ds, err := a.loop.ListDrafts(ctx, loop.DraftQuery{Status: status, Limit: 200})
	if err != nil {
		return err
	}
	for _, d := range ds {
		issue := ""
		if d.IssueNumber > 0 {
			issue = fmt.Sprintf(" -> %s#%d", d.IssueRepo, d.IssueNumber)
		}
		fmt.Fprintf(a.out, "%s  %-9s %s %-8s n=%-3d %s%s\n", d.ID, d.Status, d.Priority, d.Category, d.Count, d.Title, issue)
	}
	fmt.Fprintf(a.out, "%d draft(s)\n", len(ds))
	return nil
}

func (a *app) printDraft(d *loop.Draft, reasons []string) {
	fmt.Fprintf(a.out, "id:       %s\nstatus:   %s\ncluster:  %s\npriority: %s\ncount:    %d (%s)\n", d.ID, d.Status, d.ClusterKey, d.Priority, d.Count, strings.Join(d.Sources, ", "))
	if len(reasons) > 0 {
		fmt.Fprintf(a.out, "exit check: FAILS (%s)\n", strings.Join(reasons, ", "))
	} else {
		fmt.Fprintln(a.out, "exit check: ok")
	}
	fmt.Fprintln(a.out, "before approving: no instructions to an agent (merge, labels, secrets, settings) in the text, no billing/auth/deletion/.github work, nothing that identifies a person.")
	fmt.Fprintf(a.out, "\ntitle: %s\n\n%s\n", d.Title, d.Body)
}
