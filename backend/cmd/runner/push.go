package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	firebase "firebase.google.com/go/v4"
	fbauth "firebase.google.com/go/v4/auth"

	"github.com/jphacks/kz_2601/backend/internal/runner/notify"
	"github.com/jphacks/kz_2601/backend/internal/runner/notify/apns"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// pushConfig is the APNs side of the runner (#269). Sender nil = log only.
type pushConfig struct {
	Sender      notify.Sender
	Environment string // store.DeviceProduction / store.DeviceSandbox
	Topic       string
}

// pushFromEnv reads the APNs settings. Without APNS_AUTH_KEY the runner
// logs notifications only. The key comes from Secret Manager (--set-secrets);
// it is never logged.
//
//	APNS_AUTH_KEY            the .p8 contents (PKCS#8 PEM, ES256)
//	APNS_KEY_ID              the key's id (required with the key)
//	APNS_TEAM_ID             the Apple Developer team id (required with the key)
//	APNS_TOPIC               the app's bundle id (required with the key)
//	RUNNER_APNS_ENVIRONMENT  production (default) / sandbox: the APNs host, and
//	                         the device environment whose tokens are sent
func pushFromEnv(getenv func(string) string) (pushConfig, error) {
	env := strings.ToLower(strings.TrimSpace(getenv("RUNNER_APNS_ENVIRONMENT")))
	host := apns.HostProduction
	switch env {
	case "", store.DeviceProduction:
		env = store.DeviceProduction
	case store.DeviceSandbox:
		host = apns.HostDevelopment
	default:
		return pushConfig{}, fmt.Errorf("RUNNER_APNS_ENVIRONMENT must be production or sandbox, got %q", env)
	}
	cfg := pushConfig{Environment: env}
	rawKey := strings.TrimSpace(getenv("APNS_AUTH_KEY"))
	if rawKey == "" {
		return cfg, nil
	}
	keyID, teamID := strings.TrimSpace(getenv("APNS_KEY_ID")), strings.TrimSpace(getenv("APNS_TEAM_ID"))
	cfg.Topic = strings.TrimSpace(getenv("APNS_TOPIC"))
	if keyID == "" || teamID == "" || cfg.Topic == "" {
		return pushConfig{}, errors.New("APNS_AUTH_KEY is set: APNS_KEY_ID, APNS_TEAM_ID and APNS_TOPIC are required too")
	}
	key, err := apns.ParseKey([]byte(rawKey))
	if err != nil {
		// apns.ParseKey's errors carry no key material
		return pushConfig{}, fmt.Errorf("APNS_AUTH_KEY: %w", err)
	}
	c, err := apns.New(apns.Config{KeyID: keyID, TeamID: teamID, Topic: cfg.Topic, Key: key, Host: host})
	if err != nil {
		return pushConfig{}, err
	}
	cfg.Sender = c
	return cfg, nil
}

// userGetter is the part of the Firebase Auth client firebaseAge uses.
type userGetter interface {
	GetUser(ctx context.Context, uid string) (*fbauth.UserRecord, error)
}

// maxAgeCache bounds firebaseAge's cache (creation times never change).
const maxAgeCache = 10000

// firebaseAge implements notify.AccountAge with the Firebase Auth creation
// time (#339: an account younger than 24h cannot notify others). An unknown
// user is unknown (counts as new); other errors retry the run.
type firebaseAge struct {
	users userGetter

	mu    sync.Mutex
	cache map[string]time.Time
}

func (a *firebaseAge) CreatedAt(ctx context.Context, uid string) (time.Time, bool, error) {
	a.mu.Lock()
	at, ok := a.cache[uid]
	a.mu.Unlock()
	if ok {
		return at, true, nil
	}
	u, err := a.users.GetUser(ctx, uid)
	switch {
	case fbauth.IsUserNotFound(err):
		return time.Time{}, false, nil
	case err != nil:
		return time.Time{}, false, fmt.Errorf("firebase auth: %w", err)
	case u == nil || u.UserMetadata == nil || u.UserMetadata.CreationTimestamp <= 0:
		return time.Time{}, false, nil
	}
	at = time.UnixMilli(u.UserMetadata.CreationTimestamp)
	a.mu.Lock()
	if a.cache == nil || len(a.cache) >= maxAgeCache {
		a.cache = map[string]time.Time{}
	}
	a.cache[uid] = at
	a.mu.Unlock()
	return at, true, nil
}

// ageFromEnv reads RUNNER_ACCOUNT_AGE: "firebase" = Firebase Auth's creation
// time (the runner SA needs roles/firebaseauth.viewer; locally the SDK
// follows FIREBASE_AUTH_EMULATOR_HOST); "" = unknown, so nobody can notify
// others (only the actor's own notifications are sent).
func ageFromEnv(ctx context.Context, getenv func(string) string, project string) (notify.AccountAge, error) {
	switch v := strings.ToLower(strings.TrimSpace(getenv("RUNNER_ACCOUNT_AGE"))); v {
	case "":
		return nil, nil
	case "firebase":
		app, err := firebase.NewApp(ctx, &firebase.Config{ProjectID: project})
		if err != nil {
			return nil, fmt.Errorf("firebase init: %w", err)
		}
		users, err := app.Auth(ctx)
		if err != nil {
			return nil, fmt.Errorf("firebase auth init: %w", err)
		}
		return &firebaseAge{users: users}, nil
	default:
		return nil, fmt.Errorf("RUNNER_ACCOUNT_AGE must be firebase or empty, got %q", v)
	}
}
