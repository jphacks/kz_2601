package main

import (
	"flag"
	"io"
	"testing"
	"time"
)

func TestParseFlags_S7Backend010(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		args    []string
		want    options
		wantErr bool
	}{
		{"defaults", nil, options{dryRun: false, limit: 1000, window: 35 * 24 * time.Hour}, false},
		{"all set", []string{"-dry-run", "-limit", "5", "-window", "48h"}, options{dryRun: true, limit: 5, window: 48 * time.Hour}, false},
		{"dry-run=false", []string{"-dry-run=false"}, options{limit: 1000, window: 35 * 24 * time.Hour}, false},
		{"limit not a number", []string{"-limit", "many"}, options{}, true},
		{"window without unit", []string{"-window", "35"}, options{}, true},
		{"window not a duration", []string{"-window", "a month"}, options{}, true},
		{"dry-run not a bool", []string{"-dry-run=maybe"}, options{}, true},
		{"unknown flag", []string{"-apply"}, options{}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fs := flag.NewFlagSet("appstore-reconcile", flag.ContinueOnError)
			fs.SetOutput(io.Discard)
			got, err := parseFlags(fs, tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseFlags(%q) = %+v, want an error", tc.args, got)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("parseFlags(%q) = %+v, %v; want %+v", tc.args, got, err, tc.want)
			}
		})
	}
}

// Positional args are left for the caller (the job takes none today).
func TestParseFlags_S7Backend010_Rest(t *testing.T) {
	t.Parallel()
	fs := flag.NewFlagSet("appstore-reconcile", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if _, err := parseFlags(fs, []string{"-limit", "3", "extra"}); err != nil {
		t.Fatal(err)
	}
	if fs.NArg() != 1 || fs.Arg(0) != "extra" {
		t.Fatalf("rest = %q, want [extra]", fs.Args())
	}
}
