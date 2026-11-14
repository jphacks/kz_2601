package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/entitlement"
	"github.com/jphacks/kz_2601/backend/internal/fsenv"
)

// TestProPeriodEnd: nil without Firestore; with the emulator, zero for a
// user who is not Pro and the expiresAt for one who is.
func TestProPeriodEnd(t *testing.T) {
	t.Run("no firestore", func(t *testing.T) {
		if proPeriodEnd(nil, nil) != nil {
			t.Fatal("proPeriodEnd(nil) must be nil")
		}
	})
	t.Run("emulator", func(t *testing.T) {
		if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
			t.Skip("FIRESTORE_EMULATOR_HOST not set")
		}
		t.Parallel()
		ctx := context.Background()
		c, err := fsenv.NewClient(ctx, fsenv.Target{Project: "demo-kz2601", Database: "(default)"})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = c.Close() })
		ents := entitlement.NewFirestore(c)
		run := strings.ReplaceAll(time.Now().Format("150405.000000000"), ".", "")
		now := time.Now().UTC().Truncate(time.Second)
		apply := func(uid string, p entitlement.Pro) {
			t.Helper()
			if _, err := ents.Apply(ctx, uid, p, ""); err != nil {
				t.Fatalf("Apply %s: %v", uid, err)
			}
		}
		pro := "pro016-" + run
		expires := now.Add(30 * 24 * time.Hour)
		apply(pro, entitlement.Pro{ProductID: "pro.monthly", OriginalTransactionID: "otid016-a-" + run,
			TransactionID: "tx016-a-" + run, Environment: "Production", ExpiresAt: expires, UpdatedAt: now})
		lapsed := "lapsed016-" + run
		apply(lapsed, entitlement.Pro{ProductID: "pro.monthly", OriginalTransactionID: "otid016-b-" + run,
			TransactionID: "tx016-b-" + run, Environment: "Production", ExpiresAt: now.Add(-time.Hour), UpdatedAt: now})
		sandbox := "sandbox016-" + run
		apply(sandbox, entitlement.Pro{ProductID: "pro.monthly", OriginalTransactionID: "otid016-c-" + run,
			TransactionID: "tx016-c-" + run, Environment: "Sandbox", ExpiresAt: expires, UpdatedAt: now})

		end := proPeriodEnd(ents, []string{"Production"})
		if end == nil {
			t.Fatal("proPeriodEnd(ents) must not be nil")
		}
		for _, tc := range []struct {
			uid  string
			want time.Time
		}{
			{pro, expires},
			{"nobody016-" + run, time.Time{}},
			{lapsed, time.Time{}},
			{sandbox, time.Time{}},
		} {
			got, err := end(ctx, tc.uid)
			if err != nil {
				t.Fatalf("%s: %v", tc.uid, err)
			}
			if !got.Equal(tc.want) {
				t.Errorf("%s: end = %v, want %v", tc.uid, got, tc.want)
			}
		}
	})
}
