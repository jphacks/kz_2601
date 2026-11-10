// Command feedback-loop runs the steps of the self-improvement loop (#364)
// that read feedback: ingest (#368), triage (#376), issue drafts and their
// human approval (#369), 「あなたの声で直りました」 (#374), audit and stop
// switches (#375). It never pushes code, never merges and never creates an
// issue without a human's `drafts approve`.
//
// Operations, environment and the order of the steps:
// docs/backend/features/feedback-loop.md.
//
//	feedback-loop ingest [-source in_app|app_store_review]
//	feedback-loop ingest-crash -issue ID -type T -count N -version V
//	feedback-loop triage [-max 200] [-batch 20]
//	feedback-loop triage-score -labels FILE
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
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/fsenv"
	"github.com/jphacks/kz_2601/backend/internal/loop"
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
	a := &app{
		loop:     loop.NewFirestore(st.Client()),
		feedback: st,
		users:    st,
		getenv:   os.Getenv,
		now:      time.Now,
		out:      os.Stdout,
	}
	if err := a.run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "feedback-loop:", err)
		os.Exit(1)
	}
}

const usageText = `usage: feedback-loop <command> [flags]
  ingest [-source in_app|app_store_review]   copy new feedback into the inbox (masked)
  ingest-crash -issue ID -type T -count N -version V   record one crash spike
  triage [-max 200] [-batch 20]              classify, cluster and prioritise new items (LLM, no tools)
  triage-score -labels FILE                  agreement with a human's labels (lines: itemId,category)
`

// app holds what the commands use (tests build it with in-memory parts).
type app struct {
	loop     loop.Store
	feedback store.FeedbackReader
	users    ingest.Users
	// appStore, when set, replaces the App Store Connect fetcher (tests).
	appStore ingest.Fetcher
	// model, when set, replaces the Claude client (tests use a fake).
	model  triage.Model
	getenv func(string) string
	now    func() time.Time
	out    io.Writer
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
	case "triage":
		return a.triage(ctx, rest)
	case "triage-score":
		return a.triageScore(ctx, rest)
	}
	_, _ = io.WriteString(a.out, usageText)
	return fmt.Errorf("unknown command %q", cmd)
}

func (a *app) runner() *ingest.Runner {
	return &ingest.Runner{Store: a.loop, Policy: ingest.PolicyFromEnv(a.getenv), Users: a.users, Now: a.now}
}

func (a *app) ingest(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("ingest", flag.ContinueOnError)
	only := fs.String("source", "", "only this source (in_app / app_store_review); default all that are on")
	if err := fs.Parse(args); err != nil {
		return err
	}
	var fetchers []ingest.Fetcher
	if *only == "" || *only == string(loop.SourceInApp) {
		fetchers = append(fetchers, ingest.InApp{Reader: a.feedback})
	}
	if *only == "" || *only == string(loop.SourceStoreReview) {
		f, err := a.appStoreFetcher()
		switch {
		case errors.Is(err, ingest.ErrAppStoreOff):
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
	spike := ingest.CrashSpike{IssueID: strings.TrimSpace(*issue), Type: strings.TrimSpace(*typ), Count: *count,
		AppVersion: strings.TrimSpace(*version), At: a.now().UTC()}
	// a hand-recorded spike is not bound by the source cursor
	it := ingest.Normalize(ingest.CrashRaw(spike), ingest.PolicyFromEnv(a.getenv), spike.At)
	created, err := a.loop.PutItem(ctx, it)
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "ingest-crash: %s created=%v\n", it.ID, created)
	return nil
}
