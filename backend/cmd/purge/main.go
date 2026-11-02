// Command purge erases accounts that DELETE /v1/me soft-deleted more than
// -older-than ago (#458, privacy policy §4), then in-app feedback older than
// -feedback-older-than (#319, 180 days). Meant for a Cloud Run Job run once a
// day by Cloud Scheduler (docs/infra/account-purge.md).
//
// Per account: store.PurgeAccount (data) -> entitlement PurgeUser (Pro
// documents and App Store transaction bindings, #314) -> Firebase Auth user
// deletion -> store.MarkAccountPurged (tombstone). A failure leaves the account a
// candidate, so the next run retries it. Exit status 1 when any account failed.
//
// Environment:
//
//	GOOGLE_CLOUD_PROJECT          required (Firestore and Firebase Auth project)
//	FIRESTORE_DATABASE            database ID (default "(default)"; internal/fsenv)
//	APP_ENV / ALLOW_SHARED_PROD_DB  stg guard (internal/fsenv)
//	FIRESTORE_EMULATOR_HOST       local emulator (make run-purge)
//	FIREBASE_AUTH_EMULATOR_HOST   local Auth emulator
//
// stg and prod share one Firestore database: schedule this job in one
// environment only.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	firebase "firebase.google.com/go/v4"
	fbauth "firebase.google.com/go/v4/auth"

	"github.com/jphacks/kz_2601/backend/internal/entitlement"
	"github.com/jphacks/kz_2601/backend/internal/fsenv"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

func main() {
	var opts options
	flag.BoolVar(&opts.dryRun, "dry-run", false, "only list the accounts that would be purged")
	flag.IntVar(&opts.limit, "limit", 50, "max accounts per run")
	flag.DurationVar(&opts.olderThan, "older-than", store.PurgeAfter, "purge accounts deleted longer ago than this")
	flag.BoolVar(&opts.deleteAuth, "delete-auth", true, "also delete the Firebase Auth user")
	flag.DurationVar(&opts.feedbackOlderThan, "feedback-older-than", store.FeedbackRetention, "erase feedback created longer ago than this")
	flag.IntVar(&opts.feedbackLimit, "feedback-limit", 500, "max feedback documents erased per run")
	flag.Parse()

	logger := httpx.NewLogger()
	slog.SetDefault(logger)
	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if project == "" {
		logger.Error("purge: GOOGLE_CLOUD_PROJECT is required")
		os.Exit(2)
	}
	target := fsenv.FromEnv(os.Getenv)
	shared, err := fsenv.Check(target, os.Getenv)
	if err != nil {
		logger.Error("purge: refusing to start", slog.Any("error", err))
		os.Exit(2)
	}
	if shared {
		logger.Warn("purge: stg shares the production Firestore (ALLOW_SHARED_PROD_DB=1)")
	}
	logger.Info("purge: firestore", slog.String("target", target.String()))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.NewFirestore(ctx, target.Project, target.Database)
	if err != nil {
		logger.Error("purge: firestore init failed", slog.Any("error", err))
		os.Exit(1)
	}
	defer func() { _ = st.Close() }()
	opts.entitlements = entitlement.NewFirestore(st.Client())

	var users authDeleter
	if opts.deleteAuth && !opts.dryRun {
		app, err := firebase.NewApp(ctx, &firebase.Config{ProjectID: project})
		if err == nil {
			users, err = app.Auth(ctx)
		}
		if err != nil {
			logger.Error("purge: firebase auth init failed", slog.Any("error", err))
			os.Exit(1)
		}
	}
	if failed := run(ctx, st, users, opts, logger, time.Now()); failed > 0 {
		os.Exit(1)
	}
}

type options struct {
	dryRun            bool
	limit             int
	olderThan         time.Duration
	deleteAuth        bool
	feedbackOlderThan time.Duration
	feedbackLimit     int
	// entitlements erases users/{uid}/entitlements and the uid's App Store
	// bindings (so the Apple ID can be restored on a new account); nil = skip.
	entitlements entitlementPurger
}

// entitlementPurger is the part of entitlement.Store purge uses.
type entitlementPurger interface {
	PurgeUser(ctx context.Context, uid string) (int, error)
}

// purgeStore is what run needs from the store.
type purgeStore interface {
	store.AccountPurger
	store.FeedbackPurger
}

// authDeleter is the part of the Firebase Admin auth client purge uses.
type authDeleter interface {
	DeleteUser(ctx context.Context, uid string) error
}

// run purges up to opts.limit candidates and returns how many failed. users
// may be nil (Auth deletion off). Logs carry a hash of the uid, never the uid
// or anything the user wrote.
func run(ctx context.Context, st purgeStore, users authDeleter, opts options, logger *slog.Logger, now time.Time) int {
	if opts.olderThan < 24*time.Hour {
		logger.Error("purge: -older-than below 24h refused", slog.Duration("olderThan", opts.olderThan))
		return 1
	}
	cutoff := now.Add(-opts.olderThan)
	cands, err := st.DeletedAccounts(ctx, cutoff, opts.limit)
	if err != nil {
		logger.Error("purge: list candidates failed", slog.Any("error", err))
		return 1
	}
	logger.Info("purge: start", slog.Int("candidates", len(cands)), slog.Bool("dryRun", opts.dryRun),
		slog.Int("limit", opts.limit), slog.Time("cutoff", cutoff))
	purged, failed := 0, 0
	for _, c := range cands {
		if ctx.Err() != nil {
			break
		}
		attrs := []any{slog.String("account", uidHash(c.UID)), slog.Int("deletedDaysAgo", int(now.Sub(c.DeletedAt).Hours()/24))}
		if opts.dryRun {
			logger.Info("purge: would purge", attrs...)
			continue
		}
		res, err := st.PurgeAccount(ctx, c.UID)
		entitlements := 0
		if err == nil && opts.entitlements != nil {
			entitlements, err = opts.entitlements.PurgeUser(ctx, c.UID)
		}
		if err == nil && users != nil {
			if err = users.DeleteUser(ctx, c.UID); fbauth.IsUserNotFound(err) {
				err = nil // the app deletes it on DELETE /v1/me; this is the backstop
			}
		}
		if err == nil {
			err = st.MarkAccountPurged(ctx, c.UID)
		}
		if err != nil {
			failed++
			logger.Error("purge: account failed", append(attrs, slog.Any("error", err))...)
			continue
		}
		purged++
		logger.Info("purge: account purged", append(attrs,
			slog.Int("apps", res.Apps), slog.Int("keptMirrors", res.KeptMirrors), slog.Int("shares", res.Shares),
			slog.Int("listings", res.Listings), slog.Int("releases", res.Releases), slog.Int("owners", res.Owners),
			slog.Int("installs", res.Installs), slog.Int("memberships", res.Memberships), slog.Int("marks", res.Marks),
			slog.Int("postsScrubbed", res.PostsScrubbed), slog.Int("reports", res.Reports), slog.Int("blocks", res.Blocks), slog.Int("feedback", res.Feedback),
			slog.Int("entitlements", entitlements), slog.Bool("authDeleted", users != nil))...)
	}
	if ctx.Err() == nil {
		failed += purgeFeedback(ctx, st, opts, logger, now)
	}
	if err := ctx.Err(); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("purge: interrupted", slog.Any("error", err))
	}
	logger.Info("purge: done", slog.Int("purged", purged), slog.Int("failed", failed), slog.Bool("dryRun", opts.dryRun))
	return failed
}

// uidHash lets logs of one run be correlated without writing the uid.
func uidHash(uid string) string {
	sum := sha256.Sum256([]byte("purge:" + uid))
	return hex.EncodeToString(sum[:6])
}

// purgeFeedback erases feedback past its retention (#319). Returns 1 on failure.
func purgeFeedback(ctx context.Context, st store.FeedbackPurger, opts options, logger *slog.Logger, now time.Time) int {
	if opts.feedbackOlderThan < 24*time.Hour {
		logger.Error("purge: -feedback-older-than below 24h refused", slog.Duration("feedbackOlderThan", opts.feedbackOlderThan))
		return 1
	}
	cutoff := now.Add(-opts.feedbackOlderThan)
	n, err := st.PurgeFeedbackBefore(ctx, cutoff, opts.feedbackLimit, opts.dryRun)
	if err != nil {
		logger.Error("purge: feedback failed", slog.Any("error", err))
		return 1
	}
	logger.Info("purge: feedback", slog.Int("expired", n), slog.Time("cutoff", cutoff), slog.Bool("dryRun", opts.dryRun))
	return 0
}
