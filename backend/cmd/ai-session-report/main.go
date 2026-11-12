// Command ai-session-report finds where the exchange with the AI goes wrong:
// it cuts each listed user's activity into sessions (one app being made or
// changed), follows request → clarify turns → generation → validation
// failures / regeneration → quick undo / quick re-request → abandonment, and
// ranks the sessions by how strongly they suggest the AI guessed wrong.
// Run by a person (docs/ops/ai-session-report.md), never on a schedule.
//
// Read-only. Firestore is read through fsReader (Get / queries only); the
// logs come from a file the operator exported with `gcloud logging read`
// (this tool never calls the Logging API).
//
// Targets are only the uids given (-uids / -uids-file). There is no "all
// users" session report. The log-only modes (modes.go: `ai-session-report
// <mode> -logs FILE`) count every user's log lines but print counts only.
//
// What users said (the request text kept in versions/{n}.source, feedback
// messages, names in the DSL) goes only to the report file, written outside
// any git repository (default ~/Desktop/JPHACKS2026/reports, mode 0600).
// stdout prints counts and the file path only.
//
// Environment:
//
//	GOOGLE_CLOUD_PROJECT            required
//	FIRESTORE_DATABASE              database ID (default "(default)"; internal/fsenv)
//	APP_ENV / ALLOW_SHARED_PROD_DB  stg guard (internal/fsenv)
//	PROD_FIRESTORE_PROJECT / PROD_FIRESTORE_DATABASE  what counts as production
//	FIRESTORE_EMULATOR_HOST         local emulator
//
// The production database (fsenv.IsProdDatabase) is refused unless
// -allow-prod-db is given.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/fsenv"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

// maxUIDs caps one run's uid list: this is a look at a few consenting users.
const maxUIDs = 20

var uidRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

type options struct {
	uids    []string
	logs    []string
	outDir  string
	format  string // md | json | both
	days    int
	until   time.Time
	cfg     config
	nowFunc func() time.Time
}

func main() {
	if handled, err := runLogMode(os.Args[1:], os.Stdout); handled {
		if err != nil {
			fmt.Fprintln(os.Stderr, "ai-session-report:", err)
			os.Exit(2)
		}
		return
	}
	var (
		o         options
		uidList   = flag.String("uids", "", "comma-separated uids to report on (required, or -uids-file)")
		uidFile   = flag.String("uids-file", "", "file with one uid per line (# comments)")
		logList   = flag.String("logs", "", "comma-separated `gcloud logging read --format=json` exports (optional)")
		until     = flag.String("until", "", "end of the period, RFC3339 (default now)")
		allowProd = flag.Bool("allow-prod-db", false, "allow reading the production Firestore database")
		printFlt  = flag.Bool("print-filter", false, "print the `gcloud logging read` filter for the uids and period, then exit (no Firestore)")
		services  = flag.String("services", "ai-prod,appconfig-prod", "Cloud Run services for -print-filter")
	)
	flag.StringVar(&o.outDir, "out-dir", defaultOutDir(), "where to write the report (must be outside any git repository)")
	flag.StringVar(&o.format, "format", "md", "md | json | both")
	flag.IntVar(&o.days, "days", 14, "period length in days, ending at -until")
	flag.IntVar(&o.cfg.Top, "top", 20, "sessions listed in detail")
	flag.DurationVar(&o.cfg.Gap, "gap", 30*time.Minute, "a pause longer than this starts a new session")
	flag.DurationVar(&o.cfg.QuickUndo, "quick-undo", 10*time.Minute, "restore / manual edit within this after an AI version counts as a miss")
	flag.DurationVar(&o.cfg.QuickFix, "quick-fix", 5*time.Minute, "AI edit within this after an AI version counts as a possible miss")
	flag.DurationVar(&o.cfg.Slow, "slow", 60*time.Second, "generation slower than this is flagged")
	flag.IntVar(&o.cfg.ManyClarify, "many-clarify", 3, "clarify turns before one request that count as many")
	flag.Parse()
	if flag.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "ai-session-report: unknown mode %q (log modes: %s)\n", flag.Arg(0), logModeNames())
		os.Exit(2)
	}
	o.cfg.RecordLimit = 1000

	err := func() error {
		uids, err := readUIDs(*uidList, *uidFile)
		if err != nil {
			return err
		}
		o.uids = uids
		o.logs = splitList(*logList)
		o.until = time.Now()
		if *until != "" {
			if o.until, err = time.Parse(time.RFC3339, *until); err != nil {
				return fmt.Errorf("-until: %w", err)
			}
		}
		return validate(o)
	}()
	if err != nil {
		fmt.Fprintln(os.Stderr, "ai-session-report:", err)
		os.Exit(2)
	}
	if *printFlt {
		fmt.Println(logFilter(o.uids, splitList(*services), o.until.Add(-time.Duration(o.days)*24*time.Hour), o.until))
		return
	}
	target := fsenv.FromEnv(os.Getenv)
	if err := guard(target, *allowProd, os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, "ai-session-report: refusing to start:", err)
		os.Exit(2)
	}
	fmt.Printf("target: %s (emulator=%t) users=%d days=%d\n", target, target.Emulator != "", len(o.uids), o.days)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	c, err := fsenv.NewClient(ctx, target)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ai-session-report: firestore init failed:", err)
		os.Exit(1)
	}
	defer func() { _ = c.Close() }()
	if err := run(ctx, fsReader{c: c}, o, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "ai-session-report:", err)
		os.Exit(1)
	}
}

func defaultOutDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Desktop", "JPHACKS2026", "reports")
}

// guard refuses a database this tool must not read without being told.
func guard(t fsenv.Target, allowProd bool, getenv func(string) string) error {
	if t.Project == "" {
		return errors.New("GOOGLE_CLOUD_PROJECT is required")
	}
	if _, err := fsenv.Check(t, getenv); err != nil {
		return err
	}
	if fsenv.IsProdDatabase(t, getenv) && !allowProd {
		return fmt.Errorf("%s is the production Firestore database; pass -allow-prod-db only when that is intended (docs/ops/ai-session-report.md)", t)
	}
	return nil
}

func validate(o options) error {
	if len(o.uids) == 0 {
		return errors.New("no users: pass -uids or -uids-file (this tool never reads every user)")
	}
	if o.days < 1 || o.days > 90 {
		return fmt.Errorf("-days=%d: 1..90", o.days)
	}
	switch o.format {
	case "md", "json", "both":
	default:
		return fmt.Errorf("-format=%q: md | json | both", o.format)
	}
	if o.cfg.Gap <= 0 || o.cfg.Top < 1 {
		return errors.New("-gap and -top must be positive")
	}
	return checkOutDir(o.outDir)
}

// checkOutDir refuses an output directory inside a git work tree, so the
// report (users' words) cannot end up in a commit.
func checkOutDir(dir string) error {
	if dir == "" {
		return errors.New("-out-dir is required")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	for d := abs; ; d = filepath.Dir(d) {
		if _, err := os.Lstat(filepath.Join(d, ".git")); err == nil {
			return fmt.Errorf("-out-dir %s is inside the git repository %s: write the report outside any repository", abs, d)
		}
		if filepath.Dir(d) == d {
			return nil
		}
	}
}

// readUIDs merges -uids and -uids-file, rejecting malformed and duplicate uids.
func readUIDs(list, file string) ([]string, error) {
	raw := strings.Split(list, ",")
	if file != "" {
		f, err := os.Open(file)
		if err != nil {
			return nil, err
		}
		defer func() { _ = f.Close() }()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line, _, _ := strings.Cut(sc.Text(), "#")
			raw = append(raw, line)
		}
		if err := sc.Err(); err != nil {
			return nil, err
		}
	}
	seen := map[string]bool{}
	var out []string
	for _, s := range raw {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if !uidRe.MatchString(s) {
			return nil, errors.New("malformed uid in the list")
		}
		if seen[s] {
			return nil, errors.New("duplicate uid in the list")
		}
		seen[s] = true
		out = append(out, s)
	}
	if len(out) > maxUIDs {
		return nil, fmt.Errorf("%d uids: at most %d per run", len(out), maxUIDs)
	}
	return out, nil
}

// logFilter is the Cloud Logging filter that exports only the listed users'
// lines (by uid_hash) of the events this report reads.
func logFilter(uids, services []string, since, until time.Time) string {
	quote := func(xs []string) string {
		q := make([]string, len(xs))
		for i, x := range xs {
			q[i] = strconv.Quote(x)
		}
		return "(" + strings.Join(q, " OR ") + ")"
	}
	hashes := make([]string, len(uids))
	for i, u := range uids {
		hashes[i] = httpx.UIDHash(u)
	}
	return fmt.Sprintf(`resource.type="cloud_run_revision" AND resource.labels.service_name=%s AND jsonPayload.event=%s AND jsonPayload.uid_hash=%s AND timestamp>=%q AND timestamp<=%q`,
		quote(services), quote([]string{evGenerate, evUsage, evRequest, evOpened, evChat}), quote(hashes),
		since.UTC().Format(time.RFC3339), until.UTC().Format(time.RFC3339))
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// run builds the report and writes it under o.outDir. stdout gets counts
// and paths only.
func run(ctx context.Context, rd reader, o options, stdout io.Writer) error {
	now := time.Now()
	if o.nowFunc != nil {
		now = o.nowFunc()
	}
	cfg := o.cfg
	cfg.Until = o.until
	cfg.Since = o.until.Add(-time.Duration(o.days) * 24 * time.Hour)
	if cfg.RecordLimit <= 0 {
		cfg.RecordLimit = 1000
	}

	keep := map[string]bool{}
	for _, uid := range o.uids {
		keep[httpx.UIDHash(uid)] = true
	}
	var logs []logEvent
	for _, path := range o.logs {
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		evs, err := parseLogs(f, keep)
		_ = f.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", filepath.Base(path), err)
		}
		logs = append(logs, evs...)
	}
	if len(o.logs) == 0 {
		_, _ = fmt.Fprintln(stdout, "note: no -logs given: clarify turns, failures and latency are not counted")
	}

	rep, err := buildReport(ctx, rd, o.uids, logs, len(o.logs) > 0, cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(o.outDir, 0o700); err != nil {
		return err
	}
	base := filepath.Join(o.outDir, "ai-session-report-"+now.In(jst).Format("20060102-150405"))
	var paths []string
	write := func(ext string, fn func(io.Writer, *report, time.Time) error) error {
		path := base + ext
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		if err := fn(f, rep, now); err != nil {
			_ = f.Close()
			return err
		}
		paths = append(paths, path)
		return f.Close()
	}
	if o.format == "md" || o.format == "both" {
		if err := write(".md", renderMarkdown); err != nil {
			return err
		}
	}
	if o.format == "json" || o.format == "both" {
		if err := write(".json", renderJSON); err != nil {
			return err
		}
	}
	t := rep.totals()
	_, _ = fmt.Fprintf(stdout, "log lines (listed users): %d\nsessions: %d (flagged %d) requests: %d\n", len(logs), t.Sessions, t.Flagged, t.Requests)
	for _, p := range paths {
		_, _ = fmt.Fprintf(stdout, "wrote %s (contains users' words: do not share; delete when done)\n", p)
	}
	return nil
}
