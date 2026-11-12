// Command feedback-loop runs the steps of the self-improvement loop (#364)
// that read feedback: ingest (#368), triage (#376), issue drafts and their
// human approval (#369), 「あなたの声で直りました」 (#374), audit and stop
// switches (#375). It never pushes code, never merges and never creates an
// issue without a human's `drafts approve`.
//
// Operations, environment and the order of the steps:
// docs/backend/features/feedback-loop.md.
//
//	feedback-loop ingest [-source in_app|app_store_review|google_play_review|testflight|gen_failure]
//	feedback-loop ingest-crash -issue ID -type T -count N -version V
//	feedback-loop inbox [-source S] [-status S] [-limit N]
//	feedback-loop triage [-max 200] [-batch 20]
//	feedback-loop triage-score -labels FILE
//	feedback-loop triage-eval -set FILE [-model rules|ai]
//	feedback-loop draft [-max-new 20]
//	feedback-loop drafts list|show|edit|approve|reject|publish ...
//	feedback-loop fixed [-dry-run]
//	feedback-loop switch show|set ...   feedback-loop audit [-summary]
//
// Environment: GOOGLE_CLOUD_PROJECT (required), FIRESTORE_DATABASE,
// FIRESTORE_EMULATOR_HOST (local: make -C backend run-feedback-loop),
// APP_ENV / ALLOW_SHARED_PROD_DB (internal/fsenv), LOOP_* (the doc above).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/fsenv"
	"github.com/jphacks/kz_2601/backend/internal/loop"
	"github.com/jphacks/kz_2601/backend/internal/loop/draft"
	"github.com/jphacks/kz_2601/backend/internal/loop/fixed"
	"github.com/jphacks/kz_2601/backend/internal/loop/ingest"
	"github.com/jphacks/kz_2601/backend/internal/loop/triage"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

func main() {
	if len(os.Args) < 2 {
		_, _ = io.WriteString(os.Stderr, usageText)
		os.Exit(2)
	}
	target := fsenv.FromEnv(os.Getenv)
	if target.Project == "" {
		fmt.Fprintln(os.Stderr, "feedback-loop: GOOGLE_CLOUD_PROJECT is required")
		os.Exit(2)
	}
	if _, err := fsenv.Check(target, os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, "feedback-loop: refusing to start:", err)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	st, err := store.NewFirestore(ctx, target.Project, target.Database)
	if err != nil {
		fmt.Fprintln(os.Stderr, "feedback-loop: firestore:", err)
		os.Exit(1)
	}
	defer func() { _ = st.Close() }()
	fmt.Fprintln(os.Stderr, "feedback-loop: firestore", target.String())
	lf := loop.NewFirestore(st.Client())
	a := &app{
		loop:     lf,
		control:  lf,
		sources:  lf,
		feedback: st,
		users:    st,
		devices:  st,
		consent:  fixed.StoreConsent{C: st},
		getenv:   os.Getenv,
		now:      time.Now,
		out:      &strings.Builder{},
	}
	err = a.run(ctx, os.Args[1:])
	_, _ = os.Stdout.WriteString(a.out.String())
	if err != nil {
		fmt.Fprintln(os.Stderr, "feedback-loop:", err)
		os.Exit(1)
	}
}

const usageText = `usage: feedback-loop <command> [flags]
  ingest [-source in_app|app_store_review|google_play_review|testflight|gen_failure]   copy new feedback into the inbox (masked)
  ingest-crash -issue ID -type T -count N -version V   record one crash spike
  inbox [-source S] [-status S] [-limit N]   every source in one list (metadata only, never the text)
  triage [-max 200] [-batch 20]              classify, cluster and prioritise new items (LLM, no tools)
  triage-score -labels FILE                  agreement with a human's labels (lines: itemId,category)
  triage-eval -set FILE [-model rules|ai]    triage a labelled set in memory, print agreement (category, priority, bundles)
  draft [-max-new 20]                        build issue drafts from triaged clusters (private queue)
  drafts list|show|edit|approve|reject|publish   the human review of drafts (see: drafts)
  fixed [-dry-run]                           tell senders when a merged PR fixed their issue (#374)
  notices -uid UID                           one user's 「あなたの声で直りました」 notices
  purge [-dry-run]                           erase expired copies, copies whose feedback is gone, deleted accounts' copies
  switch show | switch set -by NAME <stage|all> <on|off>   stop switches (#375)
  audit [-since 168h] [-draft ID] [-item ID] [-summary]   the audit log / weekly review

Every automatic stage runs only while LOOP_<STAGE>=on (intake, triage, draft,
publish, notify) and loop_control/switches does not say off.
`

// app holds what the commands use (tests build it with in-memory parts).
type app struct {
	loop    loop.Store
	control loop.Control
	// sources checks feedback/{id} still exists (purge); nil = skip that check.
	sources  loop.SourceChecker
	feedback store.FeedbackReader
	users    ingest.Users
	// appStore and testFlight, when set, replace the App Store Connect
	// fetchers (tests).
	appStore   ingest.Fetcher
	testFlight ingest.Fetcher
	// googlePlay, when set, replaces the Play Developer API fetcher (tests).
	googlePlay ingest.Fetcher
	// model, when set, replaces the Claude client (tests use a fake).
	model triage.Model
	// publisher, when set, replaces the GitHub client (tests).
	publisher draft.Publisher
	// issues and pusher, when set, replace GitHub and APNs (tests).
	issues  fixed.Issues
	pusher  fixed.Pusher
	consent fixed.Consent
	devices interface {
		ListDevices(ctx context.Context, uid string) ([]store.Device, error)
	}
	getenv func(string) string
	now    func() time.Time
	out    *strings.Builder // printed when the command ends
	// logger receives the loop.audit lines (nil = slog.Default).
	logger *slog.Logger
}

func (a *app) run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		_, _ = io.WriteString(a.out, usageText)
		return errors.New("no command")
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "ingest":
		return a.ingest(ctx, rest)
	case "ingest-crash":
		return a.ingestCrash(ctx, rest)
	case "inbox":
		return a.inbox(ctx, rest)
	case "triage":
		return a.triage(ctx, rest)
	case "triage-score":
		return a.triageScore(ctx, rest)
	case "triage-eval":
		return a.triageEval(ctx, rest)
	case "draft":
		return a.draftBuild(ctx, rest)
	case "drafts":
		return a.drafts(ctx, rest)
	case "fixed":
		return a.fixedRun(ctx, rest)
	case "notices":
		return a.notices(ctx, rest)
	case "purge":
		return a.purge(ctx, rest)
	case "switch":
		return a.switches(ctx, rest)
	case "audit":
		return a.audit(ctx, rest)
	}
	_, _ = io.WriteString(a.out, usageText)
	return fmt.Errorf("unknown command %q", cmd)
}

func (a *app) gate() *loop.Gate {
	return &loop.Gate{Control: a.control, Getenv: a.getenv, Now: a.now, Logger: a.logger}
}

// allow checks a stage's switches and records a refusal.
func (a *app) allow(ctx context.Context, stage string) error {
	g := a.gate()
	if err := g.Allow(ctx, stage); err != nil {
		return g.Stopped(ctx, stage, err)
	}
	return nil
}

func (a *app) runner() *ingest.Runner {
	g := a.gate()
	return &ingest.Runner{Store: a.loop, Policy: ingest.PolicyFromEnv(a.getenv), Users: a.users, Now: a.now,
		OnItem: func(ctx context.Context, it loop.Item) error { return g.Record(ctx, ingestEvent(it)) }}
}

// ingestEvent is the audit event of one stored item. The feedback id is
// kept here (private) so a feedback can be followed to its issue (#375).
func ingestEvent(it loop.Item) loop.Event {
	e := loop.Event{Stage: loop.StageIntake, Action: "ingested", ItemID: it.ID, Reason: string(it.Source)}
	if it.Source == loop.SourceInApp {
		e.FeedbackID = it.SourceID
	}
	if it.Status == loop.ItemHeld {
		e.Action, e.Reason = "held", it.HoldReason
	}
	return e
}

func (a *app) ingest(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("ingest", flag.ContinueOnError)
	only := fs.String("source", "", "only this source (in_app / app_store_review / google_play_review / testflight / gen_failure); default all that are on")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := a.allow(ctx, loop.StageIntake); err != nil {
		return err
	}
	var fetchers []ingest.Fetcher
	if *only == "" || *only == string(loop.SourceInApp) {
		fetchers = append(fetchers, ingest.InApp{Reader: a.feedback})
	}
	optional := []struct {
		src   loop.Source
		off   error
		fetch func() (ingest.Fetcher, error)
	}{
		{loop.SourceStoreReview, ingest.ErrAppStoreOff, a.appStoreFetcher},
		{loop.SourceGooglePlayReview, ingest.ErrGooglePlayOff, a.googlePlayFetcher},
		{loop.SourceTestFlight, ingest.ErrTestFlightOff, a.testFlightFetcher},
		{loop.SourceGenFailure, ingest.ErrGenFailOff, a.genFailFetcher},
	}
	for _, o := range optional {
		if *only != "" && *only != string(o.src) {
			continue
		}
		f, err := o.fetch()
		switch {
		case errors.Is(err, o.off):
			if *only != "" {
				return err
			}
		case err != nil:
			return err
		default:
			fetchers = append(fetchers, f)
		}
	}
	if len(fetchers) == 0 {
		return fmt.Errorf("unknown source %q", *only)
	}
	r := a.runner()
	for _, f := range fetchers {
		st, err := r.Run(ctx, f)
		if err != nil {
			return err
		}
		fmt.Fprintf(a.out, "ingest %s: fetched %d, new %d (held from AI %d), skipped %d\n", f.Source(), st.Fetched, st.Created, st.Held, st.Skipped)
	}
	return nil
}

func (a *app) appStoreFetcher() (ingest.Fetcher, error) {
	if a.appStore != nil {
		if a.getenv(ingest.EnvAppStoreOn) != "on" {
			return nil, ingest.ErrAppStoreOff
		}
		return a.appStore, nil
	}
	cfg, err := ingest.AppStoreConfigFromEnv(a.getenv)
	if err != nil {
		return nil, err
	}
	return ingest.AppStoreReviews{Cfg: cfg}, nil
}

func (a *app) googlePlayFetcher() (ingest.Fetcher, error) {
	if a.getenv(ingest.EnvGooglePlayOn) != "on" {
		return nil, ingest.ErrGooglePlayOff
	}
	if a.googlePlay != nil {
		return a.googlePlay, nil
	}
	return ingest.GooglePlayConfigFromEnv(a.getenv)
}

func (a *app) testFlightFetcher() (ingest.Fetcher, error) {
	if a.getenv(ingest.EnvTestFlightOn) != "on" {
		return nil, ingest.ErrTestFlightOff
	}
	if a.testFlight != nil {
		return a.testFlight, nil
	}
	cfg, err := ingest.TestFlightConfigFromEnv(a.getenv)
	if err != nil {
		return nil, err
	}
	return ingest.TestFlightFeedback{Cfg: cfg}, nil
}

func (a *app) genFailFetcher() (ingest.Fetcher, error) {
	g, err := ingest.GenFailConfigFromEnv(a.getenv)
	if err != nil {
		return nil, err
	}
	g.Now = a.now
	return g, nil
}

func (a *app) ingestCrash(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("ingest-crash", flag.ContinueOnError)
	issue := fs.String("issue", "", "Crashlytics issue id")
	typ := fs.String("type", "", "crash type (exception / signal)")
	count := fs.Int("count", 0, "events in the spike")
	version := fs.String("version", "", "app version")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *issue == "" || *count <= 0 || *version == "" {
		return errors.New("ingest-crash needs -issue, -count > 0 and -version")
	}
	if err := a.allow(ctx, loop.StageIntake); err != nil {
		return err
	}
	spike := ingest.CrashSpike{IssueID: strings.TrimSpace(*issue), Type: strings.TrimSpace(*typ), Count: *count,
		AppVersion: strings.TrimSpace(*version), At: a.now().UTC()}
	// a hand-recorded spike is not bound by the source cursor
	it := ingest.Normalize(ingest.CrashRaw(spike), ingest.PolicyFromEnv(a.getenv), spike.At)
	created, err := a.loop.PutItem(ctx, it)
	if err != nil {
		return err
	}
	if created {
		if err := a.gate().Record(ctx, ingestEvent(it)); err != nil {
			return err
		}
	}
	fmt.Fprintf(a.out, "ingest-crash: %s created=%v\n", it.ID, created)
	return nil
}

// purge is not behind a stop switch: erasing must keep working while the
// loop is stopped (#367).
func (a *app) purge(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("purge", flag.ContinueOnError)
	dry := fs.Bool("dry-run", false, "only count")
	if err := fs.Parse(args); err != nil {
		return err
	}
	res, err := loop.Sweep(ctx, a.loop, a.sources, a.users, a.now().UTC(), *dry)
	if err != nil {
		return err
	}
	if !*dry {
		e := loop.Event{Stage: "purge", Action: "purged", Reason: fmt.Sprintf("expired=%d orphans=%d deleted=%d", res.Expired, res.Orphans, res.Deleted)}
		if err := a.gate().Record(ctx, e); err != nil {
			return err
		}
	}
	fmt.Fprintf(a.out, "purge: expired %d, orphans %d, deleted accounts %d%s\n", res.Expired, res.Orphans, res.Deleted, map[bool]string{true: " (dry run)"}[*dry])
	return nil
}
