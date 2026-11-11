// Command coin-initial-backfill records coin_initial_grants for the accounts
// that had the initial coin grant before those records were written (#1429).
// For each Firebase Auth user with an Apple / Google sign-in whose wallet
// has the initial grant, it writes the claims of the user's sign-in subjects
// with the same key as the grant (coins.InitialKey via coins.BackfillInitial).
// A subject already claimed by another account is left as it is. Run once
// by a person (docs/infra/coin-initial-backfill.md).
//
// Users come from Firebase Auth (Admin SDK, the default) or from a file of
// `firebase auth:export <file> --format=json` (-export). Dry run by default:
// counts only. -apply writes. Running it again finds the claims already
// there. The output is counts only: no uid, subject or email.
//
// Environment:
//
//	GOOGLE_CLOUD_PROJECT            required (Firestore and Firebase Auth project)
//	FIRESTORE_DATABASE              database ID (default "(default)"; internal/fsenv)
//	APP_ENV / ALLOW_SHARED_PROD_DB  stg guard (internal/fsenv)
//	PROD_FIRESTORE_PROJECT / PROD_FIRESTORE_DATABASE  what counts as production
//	FIRESTORE_EMULATOR_HOST         local emulator
//	FIREBASE_AUTH_EMULATOR_HOST     local Auth emulator (needed with the
//	                                Firestore emulator unless -export is given)
//
// The production database (fsenv.IsProdDatabase) is refused unless
// -allow-prod-db is given. stg and prod share that database.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	firebase "firebase.google.com/go/v4"
	fbauth "firebase.google.com/go/v4/auth"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/coins"
	"github.com/jphacks/kz_2601/backend/internal/fsenv"
)

func main() {
	apply := flag.Bool("apply", false, "write the claims (default: dry run, write nothing)")
	export := flag.String("export", "", "read users from this `firebase auth:export --format=json` file instead of Firebase Auth")
	allowProd := flag.Bool("allow-prod-db", false, "allow running against the production Firestore database")
	flag.Parse()

	target := fsenv.FromEnv(os.Getenv)
	if err := guard(target, *allowProd, *export != "", os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, "coin-initial-backfill: refusing to start:", err)
		os.Exit(2)
	}
	fmt.Printf("target: %s (emulator=%t) mode=%s source=%s\n", target, target.Emulator != "", mode(*apply), source(*export))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	users, err := loadUsers(ctx, target.Project, *export)
	if err != nil {
		fmt.Fprintln(os.Stderr, "coin-initial-backfill: reading users failed:", err)
		os.Exit(1)
	}
	c, err := fsenv.NewClient(ctx, target)
	if err != nil {
		fmt.Fprintln(os.Stderr, "coin-initial-backfill: firestore init failed:", err)
		os.Exit(1)
	}
	defer func() { _ = c.Close() }()
	if code := run(ctx, coins.NewFirestore(c), users, *apply, time.Now(), os.Stdout); code != 0 {
		os.Exit(code)
	}
}

func mode(apply bool) string {
	if apply {
		return "APPLY"
	}
	return "dry-run"
}

func source(export string) string {
	if export != "" {
		return "export file"
	}
	return "firebase auth"
}

// guard refuses a target this tool must not touch: the production database
// without -allow-prod-db, and the Firestore emulator with the real Firebase
// Auth.
func guard(t fsenv.Target, allowProd, fromExport bool, getenv func(string) string) error {
	if t.Project == "" {
		return errors.New("GOOGLE_CLOUD_PROJECT is required")
	}
	if _, err := fsenv.Check(t, getenv); err != nil {
		return err
	}
	if t.Emulator != "" && !fromExport && getenv("FIREBASE_AUTH_EMULATOR_HOST") == "" {
		return errors.New("the Firestore emulator needs FIREBASE_AUTH_EMULATOR_HOST or -export")
	}
	if fsenv.IsProdDatabase(t, getenv) && !allowProd {
		return fmt.Errorf("%s is the production Firestore database; pass -allow-prod-db only when that is intended (docs/infra/coin-initial-backfill.md)", t)
	}
	return nil
}

// user is one account: its uid and its provider user ids ({"apple.com": ["<sub>"]}).
type user struct {
	UID       string
	Providers map[string][]string
}

func (u user) subjects() []string { return auth.ProviderSubjects(u.Providers) }

func loadUsers(ctx context.Context, project, export string) ([]user, error) {
	if export != "" {
		f, err := os.Open(export)
		if err != nil {
			return nil, err
		}
		defer func() { _ = f.Close() }()
		return parseExport(f)
	}
	app, err := firebase.NewApp(ctx, &firebase.Config{ProjectID: project})
	if err != nil {
		return nil, err
	}
	client, err := app.Auth(ctx)
	if err != nil {
		return nil, err
	}
	return listUsers(client.Users(ctx, ""))
}

// listUsers reads every Firebase Auth user (read-only).
func listUsers(it *fbauth.UserIterator) ([]user, error) {
	var out []user
	for {
		u, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		out = append(out, fromUserInfo(u.UID, u.ProviderUserInfo))
	}
}

func fromUserInfo(uid string, infos []*fbauth.UserInfo) user {
	u := user{UID: uid, Providers: map[string][]string{}}
	for _, p := range infos {
		if p != nil {
			u.Providers[p.ProviderID] = append(u.Providers[p.ProviderID], p.UID)
		}
	}
	return u
}

// parseExport reads `firebase auth:export --format=json`:
// {"users":[{"localId":"<uid>","providerUserInfo":[{"providerId":"apple.com","rawId":"<sub>"}]}]}.
func parseExport(r io.Reader) ([]user, error) {
	var doc struct {
		Users []struct {
			LocalID   string             `json:"localId"`
			Providers []*fbauth.UserInfo `json:"providerUserInfo"`
		} `json:"users"`
	}
	if err := json.NewDecoder(r).Decode(&doc); err != nil {
		return nil, fmt.Errorf("export file: %w", err)
	}
	out := make([]user, 0, len(doc.Users))
	for _, u := range doc.Users {
		if u.LocalID == "" {
			return nil, errors.New("export file: a user without localId")
		}
		out = append(out, fromUserInfo(u.LocalID, u.Providers))
	}
	return out, nil
}

// errorCode is err without its text (grpc code, or "unknown").
func errorCode(err error) string {
	if c := status.Code(err); c != codes.Unknown {
		return c.String()
	}
	return "unknown"
}

// counts is what run prints.
type counts struct {
	users, noSubject, notGranted, recorded, otherOwner, write, claims int
}

// run backfills (or counts) every user and prints the counts. Returns the
// exit code.
func run(ctx context.Context, st coins.Store, users []user, apply bool, now time.Time, w io.Writer) int {
	var c counts
	var failed error
	for _, u := range users {
		out, n, err := coins.BackfillInitial(ctx, st, u.UID, u.subjects(), apply, now)
		if err != nil {
			failed = err
			break
		}
		c.users++
		switch out {
		case coins.BackfillNoSubject:
			c.noSubject++
		case coins.BackfillNotGranted:
			c.notGranted++
		case coins.BackfillRecorded:
			c.recorded++
		case coins.BackfillOtherOwner:
			c.otherOwner++
		case coins.BackfillWrite:
			c.write++
			c.claims += n
		}
	}
	verb := "would write"
	if apply {
		verb = "wrote"
	}
	_, _ = fmt.Fprintf(w, "users read: %d of %d\nno Apple / Google sign-in: %d\ninitial grant not given: %d\nalready recorded: %d\nsubject recorded by another account (left as is): %d\naccounts to record: %d\nclaims %s: %d\n",
		c.users, len(users), c.noSubject, c.notGranted, c.recorded, c.otherOwner, c.write, verb, c.claims)
	if failed != nil {
		// the status code only: the error text names document paths (uids)
		_, _ = fmt.Fprintf(w, "ERROR: %s (counts above are up to the failure; run again)\n", errorCode(failed))
		return 1
	}
	return 0
}
