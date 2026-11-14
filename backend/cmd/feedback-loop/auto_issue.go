package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strconv"
	"strings"

	"github.com/jphacks/kz_2601/backend/internal/loop"
	"github.com/jphacks/kz_2601/backend/internal/loop/draft"
	"github.com/jphacks/kz_2601/backend/internal/loop/github"
)

// Caps of auto-issue (docs/backend/features/feedback-loop.md).
const (
	envIssueAutoMaxPerRun = "LOOP_ISSUE_AUTO_MAX_PER_RUN" // default draft.DefaultAutoMaxPerRun
	envIssueAutoMaxPerDay = "LOOP_ISSUE_AUTO_MAX_PER_DAY" // default draft.DefaultAutoMaxPerDay
)

// autoIssue files the checked drafts as issues without a person (owner
// decision 2026-11-14, #365). It runs only while LOOP_ISSUE_AUTO=on and
// LOOP_PUBLISH=on and neither stage (nor all) is off in Firestore, needs
// LOOP_ISSUE_REPO (no dry run by omission: -dry-run is explicit), and stops
// at the per-run and per-day caps. Every post and skip goes to loop_audit.
func (a *app) autoIssue(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("auto-issue", flag.ContinueOnError)
	dry := fs.Bool("dry-run", false, "only list the drafts that would be filed; approve and post nothing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("auto-issue takes no arguments: %v", fs.Args())
	}
	maxRun, err := capFromEnv(a.getenv, envIssueAutoMaxPerRun, draft.DefaultAutoMaxPerRun)
	if err != nil {
		return err
	}
	maxDay, err := capFromEnv(a.getenv, envIssueAutoMaxPerDay, draft.DefaultAutoMaxPerDay)
	if err != nil {
		return err
	}
	if err := a.allow(ctx, loop.StageIssueAuto); err != nil {
		return err
	}
	r, err := a.reviewer()
	if err != nil {
		return err
	}
	if r.Repo == "" && !*dry {
		return errors.New("auto-issue needs " + envIssueRepo + " (owner/name); use -dry-run to only list")
	}
	g := a.gate()
	f := &draft.AutoFiler{Reviewer: r, MaxPerRun: maxRun, MaxPerDay: maxDay, DryRun: *dry,
		OnFiled: func(ctx context.Context, d *loop.Draft, iss *github.Issue) error {
			if err := g.Record(ctx, loop.Event{Stage: loop.StageIssueAuto, Action: "draft_approved", DraftID: d.ID, Reason: d.Priority, BodyHash: d.BodyHash}); err != nil {
				return err
			}
			return a.afterPost(ctx, "auto-filed", "", d, iss, nil)
		},
		OnSkipped: func(ctx context.Context, d loop.Draft, reason string, err error) error {
			if err != nil {
				fmt.Fprintf(a.out, "auto-issue: %s skipped (%s): %v\n", d.ID, reason, err)
			}
			return g.Record(ctx, loop.Event{Stage: loop.StageIssueAuto, Action: "auto_skipped", DraftID: d.ID, Reason: reason})
		},
	}
	res, err := f.Run(ctx)
	if err != nil {
		return err
	}
	if *dry {
		fmt.Fprintf(a.out, "auto-issue (dry run): would file %d (%s), skipped %d, filed in the last 24h %d\n",
			len(res.Would), strings.Join(res.Would, " "), res.Skipped, res.FiledToday)
		return nil
	}
	summary := fmt.Sprintf("filed=%d appended=%d skipped=%d failed=%d left=%d today_before=%d max_run=%d max_day=%d",
		res.Filed, res.Appended, res.Skipped, res.Failed, res.Left, res.FiledToday, maxRun, maxDay)
	if err := g.Record(ctx, loop.Event{Stage: loop.StageIssueAuto, Action: "auto_run", Reason: summary}); err != nil {
		return err
	}
	fmt.Fprintf(a.out, "auto-issue: new %d, appended %d, skipped %d, failed %d, left for next run %d (filed in the last 24h before this run: %d; caps %d/run, %d/day)\n",
		res.Filed, res.Appended, res.Skipped, res.Failed, res.Left, res.FiledToday, maxRun, maxDay)
	if res.Failed > 0 {
		return fmt.Errorf("auto-issue: %d post(s) failed (the drafts stay approved by the loop; the next run retries)", res.Failed)
	}
	return nil
}

// capFromEnv reads a positive integer cap ("" = def).
func capFromEnv(getenv func(string) string, key string, def int) (int, error) {
	v := strings.TrimSpace(getenv(key))
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s: want a positive integer, got %q", key, v)
	}
	return n, nil
}
