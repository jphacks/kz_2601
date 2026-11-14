package main

import (
	"flag"
	"io"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/store"
)

func newFlagSet3280() *flag.FlagSet {
	fs := flag.NewFlagSet("purge", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

func TestParseFlags_3280(t *testing.T) {
	def := options{
		limit:             50,
		olderThan:         store.PurgeAfter,
		deleteAuth:        true,
		feedbackOlderThan: store.FeedbackRetention,
		feedbackLimit:     500,
	}
	for _, tc := range []struct {
		name string
		args []string
		edit func(*options)
	}{
		{"defaults", nil, func(*options) {}},
		{"dry run", []string{"-dry-run"}, func(o *options) { o.dryRun = true }},
		{"keep auth user", []string{"-delete-auth=false"}, func(o *options) { o.deleteAuth = false }},
		{"all set", []string{
			"-dry-run", "-limit", "7", "-older-than", "720h", "-delete-auth=false",
			"-feedback-older-than", "4320h", "-feedback-limit", "10",
		}, func(o *options) {
			*o = options{dryRun: true, limit: 7, olderThan: 720 * time.Hour, feedbackOlderThan: 4320 * time.Hour, feedbackLimit: 10}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := def
			tc.edit(&want)
			got, err := parseFlags(newFlagSet3280(), tc.args)
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Fatalf("got %+v, want %+v", got, want)
			}
		})
	}
}

// The defaults keep the retention the privacy policy promises.
func TestParseFlagsDefaultsFollowStore_3280(t *testing.T) {
	got, err := parseFlags(newFlagSet3280(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.olderThan != store.PurgeAfter || got.feedbackOlderThan != store.FeedbackRetention {
		t.Fatalf("olderThan=%s feedbackOlderThan=%s", got.olderThan, got.feedbackOlderThan)
	}
	if got.entitlements != nil {
		t.Fatal("entitlements set by flags")
	}
}

func TestParseFlagsRejectsBadValues_3280(t *testing.T) {
	for _, args := range [][]string{
		{"-limit", "many"},
		{"-limit", "2.5"},
		{"-older-than", "30"}, // a duration needs a unit
		{"-older-than", "30d"},
		{"-delete-auth=maybe"},
		{"-feedback-older-than", "half a year"},
		{"-feedback-limit", "-"},
		{"-window", "1h"}, // not a flag of this command
	} {
		if _, err := parseFlags(newFlagSet3280(), args); err == nil {
			t.Errorf("%q: no error", args)
		}
	}
}
