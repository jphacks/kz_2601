package extapi

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/fsenv"
)

type stores interface {
	ConnectionStore
	StateStore
}

// eachStore runs f on Memory and, with FIRESTORE_EMULATOR_HOST, on Firestore
// (the local emulator only).
func eachStore(t *testing.T, f func(t *testing.T, st stores)) {
	t.Run("memory", func(t *testing.T) { f(t, NewMemory()) })
	t.Run("firestore", func(t *testing.T) {
		if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
			t.Skip("FIRESTORE_EMULATOR_HOST not set")
		}
		c, err := fsenv.NewClient(context.Background(), fsenv.Target{Project: "demo-kz2601", Database: "(default)"})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = c.Close() })
		f(t, NewFirestore(c))
	})
}

func TestStores(t *testing.T) {
	eachStore(t, func(t *testing.T, st stores) {
		ctx := context.Background()
		// the emulator keeps data between runs
		u := "u-" + strings.ReplaceAll(t.Name(), "/", "_") + "-" + time.Now().Format("150405.000000000")
		now := time.Now().UTC().Truncate(time.Millisecond)
		if _, ok, err := st.Get(ctx, u, FakeOAuth); err != nil || ok {
			t.Fatalf("empty get %v %v", ok, err)
		}
		c := Connection{Provider: FakeOAuth, Status: StatusActive, Sealed: []byte{1, 2, 3}, Scopes: []string{"a"}, ConnectedAt: now, LastUsedAt: now, DeleteAt: now.Add(-time.Second)}
		if err := st.Put(ctx, u, c); err != nil {
			t.Fatal(err)
		}
		got, ok, err := st.Get(ctx, u, FakeOAuth)
		if err != nil || !ok || string(got.Sealed) != string(c.Sealed) || !got.DeleteAt.Equal(c.DeleteAt) || got.Status != StatusActive {
			t.Fatalf("get %+v %v %v", got, ok, err)
		}
		if l, err := st.List(ctx, u); err != nil || len(l) != 1 {
			t.Fatalf("list %+v %v", l, err)
		}
		due, err := st.Due(ctx, now, 1000)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, r := range due {
			found = found || (r.UID == u && r.Provider == FakeOAuth)
		}
		if !found {
			t.Fatalf("due %+v", due)
		}
		if err := st.Delete(ctx, u, FakeOAuth); err != nil {
			t.Fatal(err)
		}
		if _, ok, _ := st.Get(ctx, u, FakeOAuth); ok {
			t.Fatal("deleted connection read")
		}

		state := "s-" + u
		if err := st.PutState(ctx, state, OAuthState{UID: u, Provider: FakeOAuth, Verifier: "v", ExpireAt: now.Add(StateTTL)}); err != nil {
			t.Fatal(err)
		}
		s, ok, err := st.TakeState(ctx, state)
		if err != nil || !ok || s.UID != u || s.Verifier != "v" {
			t.Fatalf("take %+v %v %v", s, ok, err)
		}
		if _, ok, err := st.TakeState(ctx, state); err != nil || ok {
			t.Fatalf("state taken twice %v %v", ok, err)
		}

		// DeleteStates takes only uid's states
		for _, k := range []string{"s1-" + u, "s2-" + u} {
			if err := st.PutState(ctx, k, OAuthState{UID: u, Provider: FakeOAuth, Verifier: "v", ExpireAt: now.Add(StateTTL), CreatedAt: now}); err != nil {
				t.Fatal(err)
			}
		}
		if err := st.PutState(ctx, "s3-"+u, OAuthState{UID: u + "-other", Provider: FakeOAuth, Verifier: "v", ExpireAt: now.Add(StateTTL)}); err != nil {
			t.Fatal(err)
		}
		if err := st.DeleteStates(ctx, u); err != nil {
			t.Fatal(err)
		}
		for _, k := range []string{"s1-" + u, "s2-" + u} {
			if _, ok, err := st.TakeState(ctx, k); err != nil || ok {
				t.Fatalf("%s kept %v %v", k, ok, err)
			}
		}
		if s, ok, err := st.TakeState(ctx, "s3-"+u); err != nil || !ok || s.UID != u+"-other" {
			t.Fatalf("another uid's state %+v %v %v", s, ok, err)
		}
	})
}
