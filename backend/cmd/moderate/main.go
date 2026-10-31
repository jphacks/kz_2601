// Command moderate is the operator's tool for reports and unpublishing (#321,
// App Review 1.2). Runbook: docs/infra/moderation.md.
//
//	moderate reports [-all] [-limit N]          list reports (open only unless -all)
//	moderate resolve <reportId> [-status done|dismissed]
//	moderate hide-listing <publicAppId> -reason "..."   stop a 探す listing
//	moderate unhide-listing <publicAppId>
//	moderate hide-app <appId> -reason "..."     stop a shared app (apps_public): records, share links, joining
//	moderate unhide-app <appId>
//	moderate revoke-share <token>               stop one share link
//	moderate delete-record <appId> <entityId> <recordId>   remove a post in a shared app
//
// Every command that writes only prints what it would do unless -yes is given.
// -reason is shown to the author (write it for them, in Japanese).
//
// Environment: as cmd/purge (GOOGLE_CLOUD_PROJECT, FIRESTORE_DATABASE,
// APP_ENV / ALLOW_SHARED_PROD_DB, FIRESTORE_EMULATOR_HOST for local runs).
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
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	if os.Getenv("GOOGLE_CLOUD_PROJECT") == "" {
		fmt.Fprintln(os.Stderr, "moderate: GOOGLE_CLOUD_PROJECT is required")
		os.Exit(2)
	}
	target := fsenv.FromEnv(os.Getenv)
	if _, err := fsenv.Check(target, os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, "moderate: refusing to start:", err)
		os.Exit(2)
	}
	fmt.Fprintln(os.Stderr, "moderate: firestore", target.String())
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	st, err := store.NewFirestore(ctx, target.Project, target.Database)
	if err != nil {
		fmt.Fprintln(os.Stderr, "moderate: firestore init failed:", err)
		os.Exit(1)
	}
	defer func() { _ = st.Close() }()
	if err := run(ctx, st, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "moderate:", err)
		os.Exit(1)
	}
}

const usage = `usage: moderate <reports|resolve|hide-listing|unhide-listing|hide-app|unhide-app|revoke-share|delete-record> [args] [-yes] [-reason "..."]`

// operator is the part of store.Store moderate uses.
type operator interface {
	store.Moderation
	RevokeShare(ctx context.Context, token string) error
	DeleteRecord(ctx context.Context, rs store.RecordSet, id string) error
}

var errUsage = errors.New(usage)

func run(ctx context.Context, st operator, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errUsage
	}
	cmd := args[0]
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	yes := fs.Bool("yes", false, "really write")
	reason := fs.String("reason", "", "reason shown to the author (hide-*)")
	limit := fs.Int("limit", 50, "max reports")
	all := fs.Bool("all", false, "include handled reports")
	status := fs.String("status", "done", "done | dismissed")
	// flags may come before or after the positional arguments
	var pos []string
	rest := args[1:]
	for {
		if err := fs.Parse(rest); err != nil {
			return fmt.Errorf("%w: %v", errUsage, err)
		}
		if fs.NArg() == 0 {
			break
		}
		pos = append(pos, fs.Arg(0))
		rest = fs.Args()[1:]
	}
	need := func(n int) error {
		if len(pos) != n {
			return errUsage
		}
		return nil
	}
	write := func(desc string, fn func() error) error {
		if !*yes {
			_, _ = fmt.Fprintf(out, "dry run: would %s (add -yes)\n", desc)
			return nil
		}
		if err := fn(); err != nil {
			return fmt.Errorf("%s: %w", desc, err)
		}
		_, _ = fmt.Fprintf(out, "done: %s\n", desc)
		return nil
	}

	switch cmd {
	case "reports":
		rs, err := st.ListReports(ctx, *limit)
		if err != nil {
			return err
		}
		for _, r := range rs {
			if !*all && r.Status != "open" {
				continue
			}
			_, _ = fmt.Fprintf(out, "%s\t%s\t%s\t%s\t%s\t%s\n", r.CreatedAt.Format("2006-01-02 15:04Z07:00"), r.ID, r.Status, r.Kind, r.TargetID, r.Reason)
		}
		return nil
	case "resolve":
		if err := need(1); err != nil {
			return err
		}
		if *status != "done" && *status != "dismissed" {
			return errUsage
		}
		return write(fmt.Sprintf("mark report %s %s", pos[0], *status), func() error { return st.SetReportStatus(ctx, pos[0], *status) })
	case "hide-listing", "hide-app":
		if err := need(1); err != nil {
			return err
		}
		if *reason == "" {
			return errors.New("-reason is required (it is shown to the author)")
		}
		if cmd == "hide-listing" {
			return write(fmt.Sprintf("hide listing %s (reason %q)", pos[0], *reason), func() error { return st.SetCatalogHidden(ctx, pos[0], true, *reason) })
		}
		return write(fmt.Sprintf("hide shared app %s (reason %q)", pos[0], *reason), func() error { return st.SetPublicAppHidden(ctx, pos[0], true, *reason) })
	case "unhide-listing":
		if err := need(1); err != nil {
			return err
		}
		return write("unhide listing "+pos[0], func() error { return st.SetCatalogHidden(ctx, pos[0], false, "") })
	case "unhide-app":
		if err := need(1); err != nil {
			return err
		}
		return write("unhide shared app "+pos[0], func() error { return st.SetPublicAppHidden(ctx, pos[0], false, "") })
	case "revoke-share":
		if err := need(1); err != nil {
			return err
		}
		return write("revoke share "+pos[0], func() error { return st.RevokeShare(ctx, pos[0]) })
	case "delete-record":
		if err := need(3); err != nil {
			return err
		}
		return write(fmt.Sprintf("delete record %s/%s/%s", pos[0], pos[1], pos[2]), func() error {
			return st.DeleteRecord(ctx, store.PublicRecords(pos[0], pos[1]), pos[2])
		})
	}
	return errUsage
}
