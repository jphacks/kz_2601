package store

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestSameUniqueValue(t *testing.T) {
	cases := []struct {
		a, b any
		want bool
	}{
		{1, 1.0, true},
		{int64(2), 2, true},
		{2.5, 2.5, true},
		{1, 2, false},
		{1, "1", false},
		{"1", 1, false},
		{math.NaN(), math.NaN(), false},
		{"a", "a", true},
		{nil, nil, true},
		{nil, "", false},
		{true, true, true},
		{[]any{"x"}, []any{"x"}, true},
	}
	for _, c := range cases {
		if got := sameUniqueValue(c.a, c.b); got != c.want {
			t.Errorf("sameUniqueValue(%#v, %#v) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestUniqueClaimMatches(t *testing.T) {
	r := &Record{Author: "u1", Data: map[string]any{"code": "A1", "n": int64(3)}}
	if (UniqueClaim{Key: "k"}).matches(r) {
		t.Fatal("a claim without Match values must not match")
	}
	if !(UniqueClaim{Match: map[string]any{"code": "A1", "n": 3.0, UniqueAuthorKey: "u1"}}).matches(r) {
		t.Fatal("all values equal must match")
	}
	if (UniqueClaim{Match: map[string]any{"code": "A1", UniqueAuthorKey: "u2"}}).matches(r) {
		t.Fatal("another author must not match")
	}
	if (UniqueClaim{Match: map[string]any{"missing": "x"}}).matches(r) {
		t.Fatal("a missing field must not match")
	}
}

func newUniqueMemory(t *testing.T) (*Memory, RecordSet) {
	t.Helper()
	m := NewMemory()
	initial, _ := json.Marshal(dsl.Initial())
	if _, _, err := m.EnsureApp(context.Background(), "u1", "a1", initial, "x"); err != nil {
		t.Fatal(err)
	}
	return m, PrivateRecords("u1", "a1", "member")
}

func claim(code string) UniqueClaim {
	return UniqueClaim{Key: "k-" + code, Match: map[string]any{"code": code}}
}

func TestMemoryCreateRecordUnique(t *testing.T) {
	ctx := context.Background()
	m, rs := newUniqueMemory(t)

	r, created, err := m.CreateRecordUnique(ctx, rs, Record{ID: "r1", Data: map[string]any{"code": "A"}}, claim("A"), nil)
	if err != nil || !created || r.UniqueKey != "k-A" {
		t.Fatalf("create: %v %v %+v", err, created, r)
	}
	if _, _, err := m.CreateRecordUnique(ctx, rs, Record{ID: "r2", Data: map[string]any{"code": "A"}}, claim("A"), nil); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("same key: %v", err)
	}
	// Same id is idempotent and never reports a duplicate of itself.
	if _, created, err := m.CreateRecordUnique(ctx, rs, Record{ID: "r1"}, claim("A"), nil); err != nil || created {
		t.Fatalf("retry: %v %v", err, created)
	}
	// An empty key reserves nothing.
	for _, id := range []string{"r3", "r4"} {
		if _, _, err := m.CreateRecordUnique(ctx, rs, Record{ID: id, Data: map[string]any{"code": ""}}, UniqueClaim{}, nil); err != nil {
			t.Fatalf("empty claim %s: %v", id, err)
		}
	}
	// A record written without a claim still blocks the same values.
	if _, _, err := m.CreateRecord(ctx, rs, Record{ID: "legacy", Data: map[string]any{"code": "B"}}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.CreateRecordUnique(ctx, rs, Record{ID: "r5", Data: map[string]any{"code": "B"}}, claim("B"), nil); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("legacy match: %v", err)
	}
	if _, _, err := m.CreateRecordUnique(ctx, PrivateRecords("u2", "a1", "member"), Record{ID: "x"}, claim("Z"), nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown app: %v", err)
	}
	// A long value is compared as is.
	long := strings.Repeat("x", 10_000)
	if _, _, err := m.CreateRecordUnique(ctx, rs, Record{ID: "r6", Data: map[string]any{"code": long}}, claim(long), nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.CreateRecordUnique(ctx, rs, Record{ID: "r7", Data: map[string]any{"code": long + "y"}}, claim(long+"y"), nil); err != nil {
		t.Fatalf("different long value: %v", err)
	}
}

func TestMemoryCreateRecordUniqueEmitFailureWritesNothing(t *testing.T) {
	ctx := context.Background()
	m, rs := newUniqueMemory(t)
	bad := func(*Record, Record) []OutboxEvent { return []OutboxEvent{{}} } // no env / event id
	if _, _, err := m.CreateRecordUnique(ctx, rs, Record{ID: "r1", Data: map[string]any{"code": "A"}}, claim("A"), bad); !errors.Is(err, ErrOutboxEvent) {
		t.Fatal("invalid outbox event must fail")
	}
	if _, err := m.GetRecord(ctx, rs, "r1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("record written despite emit failure: %v", err)
	}
	if _, _, err := m.CreateRecordUnique(ctx, rs, Record{ID: "r2", Data: map[string]any{"code": "A"}}, claim("A"), nil); err != nil {
		t.Fatalf("failed write must not keep the claim: %v", err)
	}
}

func TestMemoryUpdateRecordUnique(t *testing.T) {
	ctx := context.Background()
	m, rs := newUniqueMemory(t)
	for _, c := range []string{"A", "B"} {
		if _, _, err := m.CreateRecordUnique(ctx, rs, Record{ID: "r" + c, Data: map[string]any{"code": c}}, claim(c), nil); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.UpdateRecordUnique(ctx, rs, "rA", map[string]any{"code": "B"}, claim("B"), nil); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("taking another record's values: %v", err)
	}
	// Keeping its own key (other fields change) is not a duplicate.
	if _, err := m.UpdateRecordUnique(ctx, rs, "rA", map[string]any{"code": "A", "note": "n"}, claim("A"), nil); err != nil {
		t.Fatalf("own key: %v", err)
	}
	r, err := m.UpdateRecordUnique(ctx, rs, "rA", map[string]any{"code": "C"}, claim("C"), nil)
	if err != nil || r.UniqueKey != "k-C" {
		t.Fatalf("move claim: %v %+v", err, r)
	}
	// The released key is free again.
	if _, err := m.UpdateRecordUnique(ctx, rs, "rB", map[string]any{"code": "A"}, claim("A"), nil); err != nil {
		t.Fatalf("released key: %v", err)
	}
	if _, err := m.UpdateRecordUnique(ctx, rs, "nope", map[string]any{}, claim("Z"), nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing record: %v", err)
	}
	if _, err := m.UpdateRecordUnique(ctx, PrivateRecords("u2", "a1", "member"), "rA", nil, claim("Z"), nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown app: %v", err)
	}
	bad := func(*Record, Record) []OutboxEvent { return []OutboxEvent{{}} }
	if _, err := m.UpdateRecordUnique(ctx, rs, "rA", map[string]any{"code": "D"}, claim("D"), bad); !errors.Is(err, ErrOutboxEvent) {
		t.Fatal("invalid outbox event must fail")
	}
	got, _ := m.GetRecord(ctx, rs, "rA")
	if got.UniqueKey != "k-C" || got.Data["code"] != "C" {
		t.Fatalf("failed update changed the record: %+v", got)
	}
}
