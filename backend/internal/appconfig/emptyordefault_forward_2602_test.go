package appconfig

import (
	"context"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestEmptyOrDefault_2602(t *testing.T) {
	tests := []struct {
		name string
		f    dsl.Field
		v    any
		want bool
	}{
		{"nil", dsl.Field{ID: "a", Type: dsl.FieldString}, nil, true},
		{"empty string", dsl.Field{ID: "a", Type: dsl.FieldString}, "", true},
		{"false", dsl.Field{ID: "a", Type: dsl.FieldBoolean}, false, true},
		{"true without default", dsl.Field{ID: "a", Type: dsl.FieldBoolean}, true, false},
		{"string without default", dsl.Field{ID: "a", Type: dsl.FieldString}, "x", false},
		{"zero number without default", dsl.Field{ID: "a", Type: dsl.FieldNumber}, float64(0), false},
		{"integer equal to default", dsl.Field{ID: "a", Type: dsl.FieldInteger, DefaultValue: float64(5)}, float64(5), true},
		{"integer different from default", dsl.Field{ID: "a", Type: dsl.FieldInteger, DefaultValue: float64(5)}, float64(6), false},
		{"true equal to default true", dsl.Field{ID: "a", Type: dsl.FieldBoolean, DefaultValue: true}, true, true},
		{"enum equal to default", dsl.Field{ID: "a", Type: dsl.FieldEnum, Options: []string{"a", "b"}, DefaultValue: "a"}, "a", true},
		{"enum different from default", dsl.Field{ID: "a", Type: dsl.FieldEnum, Options: []string{"a", "b"}, DefaultValue: "a"}, "b", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := tc.f
			if got := emptyOrDefault(&f, tc.v); got != tc.want {
				t.Fatalf("emptyOrDefault(%v)=%v want %v", tc.v, got, tc.want)
			}
		})
	}
}

func TestForwardRecords_2602(t *testing.T) {
	t.Run("relays each signal as the entity name", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		src := make(chan struct{})
		changed := make(chan string, 2)
		broken := make(chan struct{}, 1)
		done := make(chan struct{})
		go func() { forwardRecords(ctx, "notes", src, changed, broken); close(done) }()
		src <- struct{}{}
		src <- struct{}{}
		for i := 0; i < 2; i++ {
			select {
			case e := <-changed:
				if e != "notes" {
					t.Fatalf("entity %q", e)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("no change forwarded")
			}
		}
		cancel()
		waitDone2602(t, done)
		if len(broken) != 0 {
			t.Fatal("broken must not fire on cancel")
		}
	})

	t.Run("closed source reports broken", func(t *testing.T) {
		src := make(chan struct{})
		close(src)
		broken := make(chan struct{}, 1)
		done := make(chan struct{})
		go func() { forwardRecords(context.Background(), "e", src, make(chan string), broken); close(done) }()
		select {
		case <-broken:
		case <-time.After(2 * time.Second):
			t.Fatal("broken not signalled")
		}
		waitDone2602(t, done)
	})

	t.Run("closed source after cancel stays quiet", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		src := make(chan struct{})
		close(src)
		broken := make(chan struct{}, 1)
		forwardRecords(ctx, "e", src, make(chan string), broken)
		if len(broken) != 0 {
			t.Fatal("broken must not fire once ctx is done")
		}
	})

	t.Run("cancel unblocks a stuck send", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		src := make(chan struct{}, 1)
		src <- struct{}{}
		done := make(chan struct{})
		go func() { forwardRecords(ctx, "e", src, make(chan string), make(chan struct{})); close(done) }()
		time.Sleep(20 * time.Millisecond)
		cancel()
		waitDone2602(t, done)
	})

	t.Run("cancel unblocks a stuck broken send", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		src := make(chan struct{})
		close(src)
		done := make(chan struct{})
		go func() { forwardRecords(ctx, "e", src, make(chan string), make(chan struct{})); close(done) }()
		time.Sleep(20 * time.Millisecond)
		cancel()
		waitDone2602(t, done)
	})
}

func waitDone2602(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("forwardRecords did not return")
	}
}
