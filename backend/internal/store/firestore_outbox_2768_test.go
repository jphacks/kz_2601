package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestOutboxDocRoundTripKeepsEvent(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	want := OutboxEvent{
		EventID:        "ev-1",
		Env:            "stg",
		Kind:           "record_created",
		AppID:          "APP1",
		Scope:          "private",
		UID:            "u-1",
		Entity:         "note",
		RecordID:       "r-1",
		Author:         "u-2",
		Version:        7,
		RuleIDs:        []string{"rule-a", "rule-b"},
		ChangedFields:  []string{"title"},
		Data:           map[string]any{"title": "a"},
		Prev:           map[string]any{"title": "b"},
		ActorUID:       "u-3",
		ActorAnonymous: true,
		TimeZone:       "Asia/Tokyo",
		DueAt:          created.Add(time.Hour),
		Charge:         json.RawMessage(`{"ai":1}`),
		AvailableAt:    created.Add(2 * time.Hour),
		Attempts:       3,
		Deferrals:      1,
		Status:         "pending",
		CreatedAt:      created,
		ExpireAt:       created.Add(72 * time.Hour),
	}

	d := toOutboxDoc(&want)
	got := d.toEvent(want.Env, want.EventID)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("toEvent(toOutboxDoc(ev)) = %+v, want %+v", got, want)
	}
}

func TestOutboxDocRoundTripKeepsEmptyEvent(t *testing.T) {
	want := OutboxEvent{EventID: "ev-0", Env: "dev"}

	d := toOutboxDoc(&want)
	got := d.toEvent(want.Env, want.EventID)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("toEvent(toOutboxDoc(ev)) = %+v, want %+v", got, want)
	}
}

func TestOutboxDocToEventTakesEnvAndIDFromArguments(t *testing.T) {
	d := outboxDoc{Kind: "schedule"}

	got := d.toEvent("prod", "ev-9")
	if got.Env != "prod" || got.EventID != "ev-9" {
		t.Errorf("toEvent env/id = %q/%q, want %q/%q", got.Env, got.EventID, "prod", "ev-9")
	}
}

func TestOutboxDocRawOrNil(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want json.RawMessage
	}{
		{name: "empty is nil", in: "", want: nil},
		{name: "object is kept", in: `{"ai":2}`, want: json.RawMessage(`{"ai":2}`)},
		{name: "null is kept", in: "null", want: json.RawMessage("null")},
		{name: "blank is kept", in: " ", want: json.RawMessage(" ")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := rawOrNil(tc.in)
			if tc.want == nil {
				if got != nil {
					t.Errorf("rawOrNil(%q) = %q, want nil", tc.in, got)
				}
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("rawOrNil(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestMapOutboxErr(t *testing.T) {
	other := errors.New("boom")
	deadline := status.Error(codes.DeadlineExceeded, "late")

	for _, tc := range []struct {
		name string
		in   error
		want error
	}{
		{name: "nil stays nil", in: nil, want: nil},
		{name: "already exists becomes ErrAlreadyExists", in: status.Error(codes.AlreadyExists, "dup"), want: ErrAlreadyExists},
		{name: "other grpc code is kept", in: deadline, want: deadline},
		{name: "plain error is kept", in: other, want: other},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := mapOutboxErr(tc.in); !errors.Is(got, tc.want) {
				t.Errorf("mapOutboxErr(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestMapOutboxErrKeepsWrappedAlreadyExists(t *testing.T) {
	wrapped := fmt.Errorf("create: %w", status.Error(codes.AlreadyExists, "dup"))

	if got := mapOutboxErr(wrapped); !errors.Is(got, ErrAlreadyExists) {
		t.Errorf("mapOutboxErr(wrapped) = %v, want ErrAlreadyExists", got)
	}
}
