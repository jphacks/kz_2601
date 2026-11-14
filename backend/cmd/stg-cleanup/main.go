// Command stg-cleanup erases test data from Firestore (#293): the apps
// cmd/smoke made (appIds smoke-<unix>[-training|-walk|-probe|-noauth]), the
// rate-limit counters stg wrote (rateLimits/stg_*) and the data of stg test
// users whose uids are listed explicitly. Run by a person
// (docs/infra/stg-cleanup.md), never on a schedule.
//
// Dry run by default: prints what would be deleted and how many documents.
// -apply deletes, and needs -expect=<targets the dry run printed> so that a
// changed database or a changed uid list stops the run.
//
// Targets are only what the flags name. There is no "everything that looks
// like a test" selector: a real user's data is reachable only by listing its uid.
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
// -allow-prod-db is given. Until stg gets its own database (#288), stg data
// lives in the production database, so cleaning it needs that flag.
package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"

	"github.com/jphacks/kz_2601/backend/internal/entitlement"
	"github.com/jphacks/kz_2601/backend/internal/fsenv"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// maxUIDs caps one run's explicit uid list.
const maxUIDs = 100

var uidRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

type options struct {
	smoke  bool
	rates  bool
	uids   []string
	apply  bool
	expect int // -1 = not given
}

func main() {
	var (
		opts      options
		uidList   = flag.String("uids", "", "comma-separated stg test user uids to erase")
		uidFile   = flag.String("uids-file", "", "file with one test user uid per line (# comments)")
		allowProd = flag.Bool("allow-prod-db", false, "allow running against the production Firestore database")
	)
	flag.BoolVar(&opts.smoke, "smoke", false, "erase apps cmd/smoke made (smoke-<unix>...)")
	flag.BoolVar(&opts.rates, "stg-ratelimits", false, "erase rate-limit counters written by APP_ENV=stg (rateLimits/stg_*)")
	flag.BoolVar(&opts.apply, "apply", false, "delete (default: dry run, delete nothing)")
	flag.IntVar(&opts.expect, "expect", -1, "with -apply: the target count the dry run printed")
	flag.Parse()

	uids, err := readUIDs(*uidList, *uidFile)
	if err == nil {
		opts.uids = uids
		err = validate(opts)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "stg-cleanup:", err)
		os.Exit(2)
	}
	target := fsenv.FromEnv(os.Getenv)
	if err := guard(target, *allowProd, os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, "stg-cleanup: refusing to start:", err)
		os.Exit(2)
	}
	fmt.Printf("target: %s (emulator=%t) mode=%s\n", target, target.Emulator != "", mode(opts.apply))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	st, err := store.NewFirestore(ctx, target.Project, target.Database)
	if err != nil {
		fmt.Fprintln(os.Stderr, "stg-cleanup: firestore init failed:", err)
		os.Exit(1)
	}
	defer func() { _ = st.Close() }()
	if code := run(ctx, st, entitlement.NewFirestore(st.Client()), opts, os.Stdout); code != 0 {
		os.Exit(code)
	}
}

func mode(apply bool) string {
	if apply {
		return "APPLY"
	}
	return "dry-run"
}

// guard refuses a target this tool must not touch.
func guard(t fsenv.Target, allowProd bool, getenv func(string) string) error {
	if t.Project == "" {
		return errors.New("GOOGLE_CLOUD_PROJECT is required")
	}
	if _, err := fsenv.Check(t, getenv); err != nil {
		return err
	}
	if fsenv.IsProdDatabase(t, getenv) && !allowProd {
		return fmt.Errorf("%s is the production Firestore database; pass -allow-prod-db only when that is intended (docs/infra/stg-cleanup.md)", t)
	}
	return nil
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
			return nil, fmt.Errorf("malformed uid %q", s)
		}
		if seen[s] {
			return nil, fmt.Errorf("duplicate uid %q", s)
		}
		seen[s] = true
		out = append(out, s)
	}
	if len(out) > maxUIDs {
		return nil, fmt.Errorf("%d uids: at most %d per run", len(out), maxUIDs)
	}
	return out, nil
}

func validate(o options) error {
	if !o.smoke && !o.rates && len(o.uids) == 0 {
		return errors.New("nothing selected: pass -smoke, -stg-ratelimits and/or -uids / -uids-file")
	}
	if o.apply && o.expect < 0 {
		return errors.New("-apply needs -expect=<targets> from the dry run")
	}
	return nil
}

// cleanupStore is what run needs (store.Firestore).
type cleanupStore interface {
	FindSmokeData(ctx context.Context) (store.SmokeData, error)
	PurgeSmokeApp(ctx context.Context, uid, appID string) (store.PurgeResult, error)
	DeleteSmokeOwner(ctx context.Context, appID string) error
	SummarizeTestUser(ctx context.Context, uid string) (store.TestUserSummary, error)
	PurgeTestUser(ctx context.Context, uid string) (store.AccountPurgeResult, error)
	DeleteUserTree(ctx context.Context, uid string) (int, error)
	FindStgRateLimits(ctx context.Context) ([]string, error)
	DeleteStgRateLimits(ctx context.Context, ids []string) error
}

type entitlementPurger interface {
	PurgeUser(ctx context.Context, uid string) (int, error)
}

// run prints the plan, then (with -apply) deletes it. Returns the exit code.
func run(ctx context.Context, st cleanupStore, ent entitlementPurger, o options, w io.Writer) int {
	out := func(format string, a ...any) { _, _ = fmt.Fprintf(w, format, a...) }
	var smoke store.SmokeData
	if o.smoke {
		var err error
		if smoke, err = st.FindSmokeData(ctx); err != nil {
			out("ERROR find smoke data: %v\n", err)
			return 1
		}
	}
	var rates []string
	if o.rates {
		var err error
		if rates, err = st.FindStgRateLimits(ctx); err != nil {
			out("ERROR find stg rate limits: %v\n", err)
			return 1
		}
	}
	users := make([]store.TestUserSummary, 0, len(o.uids))
	for _, uid := range o.uids {
		s, err := st.SummarizeTestUser(ctx, uid)
		if err != nil {
			out("ERROR summarize uid %s: %v\n", uid, err)
			return 1
		}
		users = append(users, s)
	}

	docs := 0
	if o.smoke {
		out("== smoke apps: %d, app_owners: %d\n", len(smoke.Apps), len(smoke.Owners))
		for _, a := range smoke.Apps {
			n := total(a.Docs)
			docs += n
			out("  app %s user=%s docs=%d %s\n", a.AppID, uidHash(a.UID), n, breakdown(a.Docs))
		}
		for _, id := range smoke.Owners {
			docs++
			out("  app_owners/%s\n", id)
		}
	}
	if o.rates {
		// counters carry uids in their ids: print the count only
		docs += len(rates)
		out("== stg rate-limit counters: %d\n", len(rates))
	}
	if len(users) > 0 {
		out("== test users: %d\n", len(users))
		for _, u := range users {
			n := total(u.Docs)
			docs += n
			note := ""
			if n == 0 && !u.Exists {
				note = " (no data: typo?)"
			}
			if u.Docs["entitlements"] > 0 || u.Docs["appstore_transactions"] > 0 {
				note += " (has Pro entitlement / App Store binding: check it is a test purchase)"
			}
			out("  uid %s status=%q docs=%d %s%s\n", u.UID, u.Status, n, breakdown(u.Docs), note)
		}
	}
	targets := len(smoke.Apps) + len(smoke.Owners) + len(rates) + len(users)
	out("== targets: %d (documents counted: %d)\n", targets, docs)
	if !o.apply {
		out("dry run: nothing deleted. To delete exactly this: add -apply -expect=%d\n", targets)
		return 0
	}
	if o.expect != targets {
		out("ERROR -expect=%d but %d targets now: run the dry run again and check\n", o.expect, targets)
		return 1
	}

	failed := 0
	fail := func(what string, err error) {
		failed++
		out("FAIL %s: %v\n", what, err)
	}
	for _, a := range smoke.Apps {
		if ctx.Err() != nil {
			break
		}
		if _, err := st.PurgeSmokeApp(ctx, a.UID, a.AppID); err != nil {
			fail("app "+a.AppID, err)
			continue
		}
		out("deleted app %s user=%s\n", a.AppID, uidHash(a.UID))
	}
	for _, id := range smoke.Owners {
		if ctx.Err() != nil {
			break
		}
		if err := st.DeleteSmokeOwner(ctx, id); err != nil {
			fail("app_owners/"+id, err)
			continue
		}
		out("deleted app_owners/%s\n", id)
	}
	if len(rates) > 0 && ctx.Err() == nil {
		if err := st.DeleteStgRateLimits(ctx, rates); err != nil {
			fail("stg rate-limit counters", err)
		} else {
			out("deleted stg rate-limit counters: %d\n", len(rates))
		}
	}
	for _, u := range users {
		if ctx.Err() != nil {
			break
		}
		res, err := st.PurgeTestUser(ctx, u.UID)
		ents := 0
		if err == nil {
			ents, err = ent.PurgeUser(ctx, u.UID)
		}
		left := 0
		if err == nil {
			left, err = st.DeleteUserTree(ctx, u.UID)
		}
		if err != nil {
			fail("uid "+u.UID, err)
			continue
		}
		out("deleted uid %s apps=%d shares=%d listings=%d feedback=%d reports=%d entitlements=%d rest=%d keptMirrors=%d postsScrubbed=%d\n",
			u.UID, res.Apps, res.Shares, res.Listings, res.Feedback, res.Reports, ents, left, res.KeptMirrors, res.PostsScrubbed)
	}
	if err := ctx.Err(); err != nil {
		out("ERROR interrupted: %v\n", err)
		return 1
	}
	out("== done: failed=%d\n", failed)
	if failed > 0 {
		return 1
	}
	return 0
}

func total(m map[string]int) int {
	n := 0
	for _, c := range m {
		n += c
	}
	return n
}

func breakdown(m map[string]int) string {
	parts := make([]string, 0, len(m))
	for _, k := range store.SortedKeys(m) {
		parts = append(parts, fmt.Sprintf("%s=%d", k, m[k]))
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// uidHash names a discovered (smoke) uid without printing it.
func uidHash(uid string) string {
	sum := sha256.Sum256([]byte("stg-cleanup:" + uid))
	return hex.EncodeToString(sum[:6])
}
