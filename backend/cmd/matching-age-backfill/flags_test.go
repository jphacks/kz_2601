package main

import (
	"flag"
	"io"
	"testing"
)

func TestParseFlagsAndMode(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want options
		mode string
	}{
		{"defaults: dry run", nil, options{}, "dry-run"},
		{"apply", []string{"-apply"}, options{apply: true}, "APPLY"},
		{"allow prod only is still a dry run", []string{"-allow-prod-db"}, options{allowProd: true}, "dry-run"},
		{"both", []string{"-apply", "-allow-prod-db"}, options{apply: true, allowProd: true}, "APPLY"},
		{"explicit false", []string{"-apply=false"}, options{}, "dry-run"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o, err := parseFlags(flag.NewFlagSet("t", flag.ContinueOnError), c.args)
			if err != nil {
				t.Fatal(err)
			}
			if o != c.want {
				t.Fatalf("options = %+v, want %+v", o, c.want)
			}
			if got := mode(o.apply); got != c.mode {
				t.Fatalf("mode = %q, want %q", got, c.mode)
			}
		})
	}
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if _, err := parseFlags(fs, []string{"-nope"}); err == nil {
		t.Error("unknown flag accepted")
	}
}
