package main

import (
	"flag"
	"io"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/store"
)

func dryRun(o options) options {
	o.dryRun = true
	return o
}

func TestParseFlags(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want options
	}{
		// The defaults the table in docs/infra/account-purge.md documents: the
		// Cloud Run Job runs with no flags once dry-run is taken off its args.
		{"defaults", nil, opts()},
		// PURGE_FLAGS in mk/purge.mk, and the args of the first Cloud Run Job run.
		{"dry run", []string{"-dry-run"}, dryRun(opts())},
		{"explicit dry run false", []string{"-dry-run=false"}, opts()},
		{"first cloud run job run", []string{"-dry-run", "-older-than=696h"}, func() options {
			o := dryRun(opts())
			o.olderThan = 696 * time.Hour
			return o
		}()},
		{"auth deletion off", []string{"-delete-auth=false"}, func() options {
			o := opts()
			o.deleteAuth = false
			return o
		}()},
		{"smallest limits", []string{"-limit=1", "-feedback-limit=1"}, func() options {
			o := opts()
			o.limit, o.feedbackLimit = 1, 1
			return o
		}()},
		{"retention floor", []string{"-older-than=24h", "-feedback-older-than=24h"}, func() options {
			o := opts()
			o.olderThan, o.feedbackOlderThan = minRetention, minRetention
			return o
		}()},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o, err := parse(t, c.args)
			if err != nil {
				t.Fatal(err)
			}
			if o != c.want {
				t.Fatalf("options = %+v, want %+v", o, c.want)
			}
		})
	}
}

func TestParseFlagsRefuses(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"older-than below the floor", []string{"-older-than=1h"}},
		{"older-than just below the floor", []string{"-older-than=23h59m"}},
		{"older-than negative", []string{"-older-than=-720h"}},
		{"feedback-older-than below the floor", []string{"-feedback-older-than=23h"}},
		{"limit unbounded", []string{"-limit=0"}},
		{"limit negative", []string{"-limit=-1"}},
		{"feedback-limit unbounded", []string{"-feedback-limit=0"}},
		{"feedback-limit negative", []string{"-feedback-limit=-10"}},
		{"unknown flag", []string{"-nope"}},
		{"duration without a unit", []string{"-older-than=720"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if o, err := parse(t, c.args); err == nil {
				t.Fatalf("accepted %v: %+v", c.args, o)
			}
		})
	}
}

// Defaults that a run with no flags must keep: anything shorter would erase
// data the privacy policy promises to hold (#458, #319).
func TestDefaultsClearTheRetentionFloor(t *testing.T) {
	o, err := parse(t, nil)
	if err != nil {
		t.Fatal(err)
	}
	if o.olderThan != store.PurgeAfter || o.feedbackOlderThan != store.FeedbackRetention {
		t.Fatalf("defaults = %s / %s, want %s / %s", o.olderThan, o.feedbackOlderThan, store.PurgeAfter, store.FeedbackRetention)
	}
	if o.olderThan < minRetention || o.feedbackOlderThan < minRetention {
		t.Fatalf("defaults below %s: %+v", minRetention, o)
	}
}

func parse(t *testing.T, args []string) (options, error) {
	t.Helper()
	fs := flag.NewFlagSet("purge", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return parseFlags(fs, args)
}
