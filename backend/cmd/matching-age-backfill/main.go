// Command matching-age-backfill sets ownerAgeConfirmed on the profile records
// of matching apps whose author made the 18+ declaration (#507) but that were
// posted before the server wrote that flag. Profiles without the flag are
// still shown when their author declared (the server reads users/{uid}), so
// this only saves those reads; profiles of authors who have not declared are
// left as they are. Run once by a person (docs/infra/matching-age-backfill.md).
//
// Dry run by default: prints counts only. -apply writes the flags. Running it
// again finds nothing new to set.
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
// -allow-prod-db is given. stg and prod share that database.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/jphacks/kz_2601/backend/internal/fsenv"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

func main() {
	apply := flag.Bool("apply", false, "write the flags (default: dry run, write nothing)")
	allowProd := flag.Bool("allow-prod-db", false, "allow running against the production Firestore database")
	flag.Parse()

	target := fsenv.FromEnv(os.Getenv)
	if err := guard(target, *allowProd, os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, "matching-age-backfill: refusing to start:", err)
		os.Exit(2)
	}
	fmt.Printf("target: %s (emulator=%t) mode=%s\n", target, target.Emulator != "", mode(*apply))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	st, err := store.NewFirestore(ctx, target.Project, target.Database)
	if err != nil {
		fmt.Fprintln(os.Stderr, "matching-age-backfill: firestore init failed:", err)
		os.Exit(1)
	}
	defer func() { _ = st.Close() }()
	if code := run(ctx, st, *apply, os.Stdout); code != 0 {
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
		return fmt.Errorf("%s is the production Firestore database; pass -allow-prod-db only when that is intended (docs/infra/matching-age-backfill.md)", t)
	}
	return nil
}

// backfiller is what run needs (store.Firestore).
type backfiller interface {
	BackfillProfileAgeFlags(ctx context.Context, dryRun bool) (store.ProfileAgeBackfill, error)
}

// run backfills (or counts) and prints the counts. Returns the exit code.
func run(ctx context.Context, st backfiller, apply bool, w io.Writer) int {
	c, err := st.BackfillProfileAgeFlags(ctx, !apply)
	verb := "would set"
	if apply {
		verb = "set"
	}
	_, _ = fmt.Fprintf(w, "matching apps: %d\nprofile records: %d\nalready flagged: %d\n%s flag: %d\nauthor not declared (left hidden): %d\n",
		c.Apps, c.Profiles, c.Marked, verb, c.ToMark, c.Undeclared)
	if err != nil {
		_, _ = fmt.Fprintf(w, "ERROR: %v (counts above are up to the failure; run again)\n", err)
		return 1
	}
	return 0
}
