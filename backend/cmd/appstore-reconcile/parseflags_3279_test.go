package main

import (
	"flag"
	"io"
	"testing"
	"time"
)

func TestParseFlags_3279(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want options
	}{
		{"defaults", nil, options{dryRun: false, limit: 1000, window: 35 * 24 * time.Hour}},
		{"dry run", []string{"-dry-run"}, options{dryRun: true, limit: 1000, window: 35 * 24 * time.Hour}},
		{"all set", []string{"-dry-run=false", "-limit", "5", "-window", "48h"}, options{limit: 5, window: 48 * time.Hour}},
		{"zero limit kept", []string{"-limit=0"}, options{limit: 0, window: 35 * 24 * time.Hour}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fs := flag.NewFlagSet("appstore-reconcile", flag.ContinueOnError)
			fs.SetOutput(io.Discard)
			got, err := parseFlags(fs, tc.args)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestParseFlagsRejectsBadValues_3279(t *testing.T) {
	for _, args := range [][]string{
		{"-limit", "many"},
		{"-limit", "1.5"},
		{"-window", "35"}, // a duration needs a unit
		{"-window", "a week"},
		{"-dry-run=maybe"},
		{"-apply"}, // not a flag of this command
	} {
		fs := flag.NewFlagSet("appstore-reconcile", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		if _, err := parseFlags(fs, args); err == nil {
			t.Errorf("%q: no error", args)
		}
	}
}
