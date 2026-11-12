package calls

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
)

// emulatorStore opens a fresh project on the emulator so runs do not share data.
func emulatorStore(t *testing.T) *Firestore {
	t.Helper()
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set")
	}
	project := "demo-calls-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	c, err := firestore.NewClientWithDatabase(context.Background(), project, firestore.DefaultDatabaseID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return NewFirestore(c)
}

// sessionView is what the comparison looks at (times as micros: Firestore
// keeps microseconds and returns UTC).
func sessionView(s Session) string {
	us := func(t time.Time) int64 {
		if t.IsZero() {
			return 0
		}
		return t.UnixMicro()
	}
	var charges []string
	for k, v := range s.Charges {
		charges = append(charges, k+"="+strconv.FormatInt(v, 10))
	}
	slices.Sort(charges)
	return fmt.Sprintf("%s uid=%s app=%s room=%s video=%t blocks=%v created=%d joined=%d ended=%d paid=%d settled=%t min=%d charges=%s del=%d",
		s.ID, s.UID, s.AppID, s.Room, s.Video, s.Blocks, us(s.CreatedAt), us(s.JoinedAt), us(s.EndedAt), us(s.PaidUntil),
		s.Settled, s.Minutes, strings.Join(charges, ","), us(s.DeleteAt))
}

// storeScenario runs the same steps on a Store and returns one line per step.
func storeScenario(t *testing.T, st Store) []string {
	t.Helper()
	ctx := context.Background()
	base := time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)
	mk := func(id, room string, n int, settled bool) Session {
		return Session{
			ID: id, UID: "u-" + id, AppID: "app1", Room: room, Video: n%2 == 0,
			Blocks:    []Block{{HoldID: "h-" + id, Amount: int64(10 * (n + 1))}},
			CreatedAt: base.Add(time.Duration(n) * time.Minute),
			Settled:   settled,
			DeleteAt:  base.Add(Retention),
		}
	}
	var out []string
	note := func(step string, err error) {
		switch {
		case err == nil:
			out = append(out, step+": ok")
		case errors.Is(err, ErrNotFound):
			out = append(out, step+": not found")
		default:
			out = append(out, step+": error")
		}
	}

	for i, s := range []Session{mk("a1", "roomA", 0, false), mk("a2", "roomA", 1, false), mk("a3", "roomA", 2, true), mk("b1", "roomB", 3, false)} {
		note("create "+s.ID+" #"+strconv.Itoa(i), st.Create(ctx, s))
	}
	note("create a1 again", st.Create(ctx, mk("a1", "roomA", 9, false)))

	s, err := st.Get(ctx, "a1")
	note("get a1", err)
	out = append(out, "get a1 = "+sessionView(s))
	_, err = st.Get(ctx, "missing")
	note("get missing", err)

	upd, err := st.Update(ctx, "a2", func(s *Session) error {
		s.Settled = true
		s.Minutes = 3
		s.Charges = map[string]int64{"h-a2": 15}
		s.EndedAt = base.Add(5 * time.Minute)
		return nil
	})
	note("update a2", err)
	out = append(out, "update a2 = "+sessionView(upd))
	s, err = st.Get(ctx, "a2")
	note("get a2 after update", err)
	out = append(out, "get a2 after update = "+sessionView(s))

	_, err = st.Update(ctx, "missing", func(*Session) error { return nil })
	note("update missing", err)
	_, err = st.Update(ctx, "a1", func(*Session) error { return errors.New("refused") })
	note("update a1 refused", err)
	s, _ = st.Get(ctx, "a1")
	out = append(out, "a1 after refused = "+sessionView(s))

	for _, room := range []string{"roomA", "roomB", "roomC"} {
		open, err := st.Open(ctx, room)
		note("open "+room, err)
		var ids []string
		for _, s := range open {
			ids = append(ids, s.ID)
		}
		slices.Sort(ids) // Firestore Open has no order
		out = append(out, "open "+room+" = "+strings.Join(ids, ","))
	}
	return out
}

func TestFirestoreStoreMatchesMemory(t *testing.T) {
	want := storeScenario(t, NewMemory())
	// The memory run itself is the spec: pin the parts the issue names.
	for _, line := range []string{"create a1 again: error", "get missing: not found", "update missing: not found",
		"update a1 refused: error", "open roomA = a1", "open roomB = b1", "open roomC = "} {
		if !slices.Contains(want, line) {
			t.Errorf("memory run lacks %q", line)
		}
	}
	got := storeScenario(t, emulatorStore(t))
	if len(got) != len(want) {
		t.Fatalf("steps: got %d, want %d\n%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("step %d:\n firestore %s\n memory    %s", i, got[i], want[i])
		}
	}
}
