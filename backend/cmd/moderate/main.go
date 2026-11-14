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
// Runner stops (#346; -env is the runner's outbox env: prod, stg, local):
//
//	moderate stops -env E                                   list the stops of env E
//	moderate stop-app <appId> -env E -reason "..." [-for 24h] [-force]
//	    the app's server rules and scheduled runs stop (the creator cannot lift it);
//	    -for: a pause that ends by itself (else until resumed);
//	    -force: also refuse the writes that would trigger the rules (403 app_stopped)
//	moderate resume-app <appId> -env E
//	moderate stop-all -env E -reason "..." [-for D] [-force]   every app of env E (no new revision)
//	moderate resume-all -env E
//	moderate stop-log [appId] -env E [-limit N]             who set / lifted which stop, newest first
//	moderate stop-rule <appId> <ruleId> -env E -reason "..."
//	moderate resume-rule <appId> <ruleId> -env E
//	moderate pause-notify <appId> -env E -reason "..." [-for 72h]   rules run, notifications are not sent
//	moderate resume-notify <appId> -env E                   also lifts the automatic pause (reports)
//
// The runner commands refuse the production Firestore database unless
// -allow-prod-db is given (as cmd/stg-cleanup).
//
// Every command that writes only prints what it would do unless -yes is given.
// -reason is shown to the author (write it for them, in Japanese). The
// runner commands record -operator (default $USER) in the stop log.
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
	if err := run(ctx, st, os.Args[1:], os.Stdout, fsenv.IsProdDatabase(target, os.Getenv)); err != nil {
		fmt.Fprintln(os.Stderr, "moderate:", err)
		os.Exit(1)
	}
}

const usage = `usage: moderate <reports|resolve|hide-listing|unhide-listing|hide-app|unhide-app|revoke-share|delete-record|stops|stop-log|stop-app|resume-app|stop-all|resume-all|stop-rule|resume-rule|pause-notify|resume-notify> [args] [-yes] [-reason "..."] [-env E] [-for D] [-force] [-operator NAME] [-allow-prod-db]`

// operator is the part of store.Store moderate uses.
type operator interface {
	store.Moderation
	store.RunnerStops
	RevokeShare(ctx context.Context, token string) error
	DeleteRecord(ctx context.Context, rs store.RecordSet, id string) error
}

var errUsage = errors.New(usage)

// run runs one command. prodDB: the target is the production database
// (the runner commands need -allow-prod-db there).
func run(ctx context.Context, st operator, args []string, out io.Writer, prodDB bool) error {
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
	env := fs.String("env", "", "runner env (stop commands): prod | stg | local")
	pauseFor := fs.Duration("for", defaultNotifyPause, "pause-notify / stop-app / stop-all: how long")
	force := fs.Bool("force", false, "stop-app / stop-all: also refuse the rule-triggering writes")
	operatorName := fs.String("operator", os.Getenv("USER"), "runner commands: who is operating (stop log)")
	allowProd := fs.Bool("allow-prod-db", false, "allow the runner commands on the production database")
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

	if runnerCommands[cmd] {
		if !envRe.MatchString(*env) {
			return fmt.Errorf("%w: -env must name the runner env (prod, stg, local)", errUsage)
		}
		if prodDB && !*allowProd {
			return errors.New("this is the production Firestore database; pass -allow-prod-db only when that is intended (docs/ops/incident-runbook.md 手順 F)")
		}
		a := stopArgs{env: *env, reason: *reason, pauseFor: *pauseFor, force: *force, operator: *operatorName, limit: *limit, yes: *yes}
		fs.Visit(func(f *flag.Flag) { a.forSet = a.forSet || f.Name == "for" })
		return runStops(ctx, st, cmd, pos, a, out)
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
