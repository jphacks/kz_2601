package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strings"

	"github.com/jphacks/kz_2601/backend/internal/loop"
	"github.com/jphacks/kz_2601/backend/internal/loop/fixed"
	"github.com/jphacks/kz_2601/backend/internal/loop/github"
	"github.com/jphacks/kz_2601/backend/internal/runner/notify/apns"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// envNotifyPush turns on the push part of 「あなたの声で直りました」; the
// in-app notice needs only LOOP_NOTIFY. APNs settings are the runner's
// (APNS_AUTH_KEY / APNS_KEY_ID / APNS_TEAM_ID / APNS_TOPIC, from Secret
// Manager) plus LOOP_APNS_ENVIRONMENT (production by default).
const envNotifyPush = "LOOP_NOTIFY_PUSH"

func (a *app) fixedIssues() (fixed.Issues, error) {
	if a.issues != nil {
		return a.issues, nil
	}
	tok := a.getenv(envGitHubToken)
	if tok == "" {
		return nil, errors.New(envGitHubToken + " is not set (reading how issues were closed)")
	}
	return &github.Client{Token: tok}, nil
}

func (a *app) fixedPusher() (fixed.Pusher, fixed.Consent, error) {
	if a.getenv(envNotifyPush) != "on" {
		return nil, nil, nil
	}
	if a.pusher != nil {
		return a.pusher, a.consent, nil
	}
	if a.devices == nil || a.consent == nil {
		return nil, nil, errors.New("push needs the device and consent stores")
	}
	env := strings.ToLower(strings.TrimSpace(a.getenv("LOOP_APNS_ENVIRONMENT")))
	host := apns.HostProduction
	switch env {
	case "", store.DeviceProduction:
		env = store.DeviceProduction
	case store.DeviceSandbox:
		host = apns.HostDevelopment
	default:
		return nil, nil, fmt.Errorf("LOOP_APNS_ENVIRONMENT must be production or sandbox")
	}
	keyID, teamID, topic := a.getenv("APNS_KEY_ID"), a.getenv("APNS_TEAM_ID"), a.getenv("APNS_TOPIC")
	raw := strings.TrimSpace(a.getenv("APNS_AUTH_KEY"))
	if raw == "" || keyID == "" || teamID == "" || topic == "" {
		return nil, nil, fmt.Errorf("%s=on needs APNS_AUTH_KEY, APNS_KEY_ID, APNS_TEAM_ID and APNS_TOPIC", envNotifyPush)
	}
	key, err := apns.ParseKey([]byte(raw))
	if err != nil {
		return nil, nil, fmt.Errorf("APNS_AUTH_KEY: %w", err)
	}
	c, err := apns.New(apns.Config{KeyID: keyID, TeamID: teamID, Topic: topic, Key: key, Host: host})
	if err != nil {
		return nil, nil, err
	}
	return fixed.APNsPusher{Sender: c, Devices: a.devices, Environment: env}, a.consent, nil
}

// fixedRun finds published drafts whose issue a merged PR fixed, tells the
// senders, and prints a release-note draft for a person to check (#374).
func (a *app) fixedRun(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("fixed", flag.ContinueOnError)
	dry := fs.Bool("dry-run", false, "only list what was fixed; write and send nothing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !*dry {
		if err := a.allow(ctx, loop.StageNotify); err != nil {
			return err
		}
	}
	issues, err := a.fixedIssues()
	if err != nil {
		return err
	}
	pusher, consent, err := a.fixedPusher()
	if err != nil {
		return err
	}
	g := a.gate()
	n := &fixed.Notifier{Store: a.loop, Issues: issues, Users: a.users, Pusher: pusher, Consent: consent, Now: a.now, DryRun: *dry,
		OnEvent: func(ctx context.Context, e loop.Event) error { return g.Record(ctx, e) }}
	res, err := n.Run(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "fixed: checked %d, fixed %d, notices %d, pushes %d, skipped %d%s\n",
		res.Checked, len(res.Fixed), res.Notices, res.Pushes, res.Skipped, map[bool]string{true: " (dry run)"}[*dry])
	for _, f := range res.Fixed {
		fmt.Fprintf(a.out, "  %s %s#%d by PR #%d\n", f.DraftID, f.Repo, f.Issue, f.PR)
	}
	if notes := fixed.ReleaseNotes(res.Fixed); notes != "" {
		fmt.Fprintf(a.out, "\nrelease notes draft (check before using):\n\n%s", notes)
	}
	return nil
}

// notices lists one user's notices (operator check; the app reads them
// through an API that is not built yet).
func (a *app) notices(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("notices", flag.ContinueOnError)
	uid := fs.String("uid", "", "the user")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *uid == "" {
		return errors.New("notices needs -uid")
	}
	ns, err := a.loop.ListNotices(ctx, *uid, 50)
	if err != nil {
		return err
	}
	for _, n := range ns {
		fmt.Fprintf(a.out, "%s %s pushed=%v %s / %s\n", n.CreatedAt.In(jst).Format("2006-01-02 15:04"), n.ID, n.Pushed, n.Title, n.Body)
	}
	fmt.Fprintf(a.out, "%d notice(s)\n", len(ns))
	return nil
}
