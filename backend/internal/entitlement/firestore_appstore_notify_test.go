package entitlement

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/fsenv"
)

// notifyStoresBoth runs f on Memory and, with FIRESTORE_EMULATOR_HOST, on
// Firestore (#2564). fs is nil on Memory.
func notifyStoresBoth(t *testing.T, f func(t *testing.T, st NotifyStore, fs *Firestore)) {
	t.Run("memory", func(t *testing.T) { f(t, NewMemory(), nil) })
	t.Run("firestore", func(t *testing.T) {
		if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
			t.Skip("FIRESTORE_EMULATOR_HOST not set")
		}
		c, err := fsenv.NewClient(context.Background(), fsenv.Target{Project: "demo-kz2601", Database: "(default)"})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = c.Close() })
		fs := NewFirestore(c)
		f(t, fs, fs)
	})
}

// #2564: a new notification is unseen; once recorded it is seen, and a second
// record of the same UUID is not an error and does not replace the first.
func TestAppStoreNotificationRecordDedupe(t *testing.T) {
	notifyStoresBoth(t, func(t *testing.T, st NotifyStore, fs *Firestore) {
		ctx := t.Context()
		at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
		first := NotificationRecord{
			UUID: uid(t, "note"), Type: "DID_RENEW", Environment: "Sandbox",
			OriginalTransactionID: uid(t, "otid"), Outcome: OutcomeApplied,
			SignedAt: at, ProcessedAt: at, ExpireAt: at.Add(NotificationRetention),
		}
		other := uid(t, "note-other")

		if seen, err := st.NotificationSeen(ctx, first.UUID); err != nil || seen {
			t.Fatalf("before record: %v %v", seen, err)
		}
		if err := st.RecordNotification(ctx, first); err != nil {
			t.Fatal(err)
		}
		if seen, err := st.NotificationSeen(ctx, first.UUID); err != nil || !seen {
			t.Fatalf("after record: %v %v", seen, err)
		}
		if seen, err := st.NotificationSeen(ctx, other); err != nil || seen {
			t.Fatalf("other uuid: %v %v", seen, err)
		}

		again := first
		again.Outcome = OutcomeDuplicate
		if err := st.RecordNotification(ctx, again); err != nil {
			t.Fatalf("record twice: %v", err)
		}
		if seen, err := st.NotificationSeen(ctx, first.UUID); err != nil || !seen {
			t.Fatalf("after second record: %v %v", seen, err)
		}
		if fs == nil {
			return
		}
		s, err := fs.c.Collection(notificationsCollection).Doc(first.UUID).Get(ctx)
		if err != nil {
			t.Fatal(err)
		}
		d := s.Data()
		if d["outcome"] != OutcomeApplied || d["notificationType"] != "DID_RENEW" || d["originalTransactionId"] != first.OriginalTransactionID {
			t.Fatalf("stored record: %v", d)
		}
		if exp, ok := d["expireAt"].(time.Time); !ok || !exp.Equal(first.ExpireAt) {
			t.Fatalf("expireAt: %v", d["expireAt"])
		}
	})
}

// #2564: Bindings lists App Store bindings by OriginalTransactionID, after the
// given one, up to limit.
func TestAppStoreBindingsListing(t *testing.T) {
	notifyStoresBoth(t, func(t *testing.T, st NotifyStore, _ *Firestore) {
		ctx := t.Context()
		// ids share a per-run prefix: the emulator's collection is shared across runs
		p := uid(t, "bind")
		for _, s := range []string{"c", "a", "b"} {
			if _, err := st.Apply(ctx, "u-"+s+p, Pro{OriginalTransactionID: p + "-" + s, TransactionID: p + "-" + s + "1"}, ""); err != nil {
				t.Fatal(err)
			}
		}

		cases := []struct {
			name  string
			after string
			limit int
			want  []Binding
		}{
			{"first page", p, 2, []Binding{{p + "-a", "u-a" + p}, {p + "-b", "u-b" + p}}},
			{"next page", p + "-b", 2, []Binding{{p + "-c", "u-c" + p}}},
			{"one", p + "-a", 1, []Binding{{p + "-b", "u-b" + p}}},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				got, err := st.Bindings(ctx, c.after, c.limit)
				if err != nil {
					t.Fatal(err)
				}
				// keep only this run's bindings (others may sort after ours)
				var mine []Binding
				for _, b := range got {
					if strings.HasPrefix(b.OriginalTransactionID, p) {
						mine = append(mine, b)
					}
				}
				if len(mine) != len(c.want) {
					t.Fatalf("Bindings(%q, %d) = %+v, want %+v", c.after, c.limit, got, c.want)
				}
				for i := range mine {
					if mine[i] != c.want[i] {
						t.Fatalf("Bindings(%q, %d)[%d] = %+v, want %+v", c.after, c.limit, i, mine[i], c.want[i])
					}
				}
			})
		}
	})
}
