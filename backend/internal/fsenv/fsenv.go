// Package fsenv is the single place that decides which Firestore database a
// backend binary connects to, and refuses a staging process that would use
// the production database (#290).
//
// Environment:
//
//	GOOGLE_CLOUD_PROJECT     project (empty = no Firestore; callers fall back)
//	FIRESTORE_DATABASE       database ID (default "(default)")
//	APP_ENV                  stg | prod (deploy-cloudrun.yml); "staging" also counts as stg
//	PROD_FIRESTORE_PROJECT   production identity for the guard (default ProdProject)
//	PROD_FIRESTORE_DATABASE  production identity for the guard (default "(default)")
//	ALLOW_SHARED_PROD_DB=1   let a stg process use the production database
//	                         (set on stg until the split, docs/decisions 288)
//	FIRESTORE_EMULATOR_HOST  emulator: the guard does not apply
package fsenv

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
)

const (
	// DefaultDatabase is Firestore's default database ID.
	DefaultDatabase = firestore.DefaultDatabaseID
	// ProdProject is the production project (decision 288 / A2 keeps it as prod).
	ProdProject = "jphacks-kz2601"
	// OverrideEnv accepts a stg process on the production database.
	OverrideEnv = "ALLOW_SHARED_PROD_DB"
)

// Target is where a process connects.
type Target struct {
	Project  string
	Database string
	AppEnv   string
	Emulator string
}

// String is a non-secret identifier for logs.
func (t Target) String() string { return t.Project + "/" + t.Database }

// FromEnv reads the target. getenv is os.Getenv outside tests.
func FromEnv(getenv func(string) string) Target {
	return Target{
		Project:  getenv("GOOGLE_CLOUD_PROJECT"),
		Database: orDefault(getenv("FIRESTORE_DATABASE"), DefaultDatabase),
		AppEnv:   getenv("APP_ENV"),
		Emulator: getenv("FIRESTORE_EMULATOR_HOST"),
	}
}

// IsStaging reports whether appEnv names the staging environment.
func IsStaging(appEnv string) bool { return appEnv == "stg" || appEnv == "staging" }

// IsProd reports whether a staging process would use the production
// project+database (real Firestore only; the emulator never counts).
func IsProd(t Target, getenv func(string) string) bool {
	if t.Project == "" || t.Emulator != "" || !IsStaging(t.AppEnv) {
		return false
	}
	return t.Project == orDefault(getenv("PROD_FIRESTORE_PROJECT"), ProdProject) &&
		t.Database == orDefault(getenv("PROD_FIRESTORE_DATABASE"), DefaultDatabase)
}

// Check returns an error when a staging process would use the production
// database and ALLOW_SHARED_PROD_DB is not "1". The second value is true when
// it passed only because of the override (callers log a warning).
func Check(t Target, getenv func(string) string) (sharedWithProd bool, err error) {
	if !IsProd(t, getenv) {
		return false, nil
	}
	if getenv(OverrideEnv) == "1" {
		return true, nil
	}
	return false, fmt.Errorf("APP_ENV=%s would use the production Firestore %s; point GOOGLE_CLOUD_PROJECT / FIRESTORE_DATABASE at the staging database, or set %s=1 to accept sharing", t.AppEnv, t, OverrideEnv)
}

// NewClient opens a client for t (honours FIRESTORE_EMULATOR_HOST).
func NewClient(ctx context.Context, t Target) (*firestore.Client, error) {
	return firestore.NewClientWithDatabase(ctx, t.Project, t.Database)
}

func orDefault(v, d string) string {
	if v == "" {
		return d
	}
	return v
}
