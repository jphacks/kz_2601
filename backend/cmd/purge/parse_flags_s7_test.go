package main

import (
	"bytes"
	"errors"
	"flag"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/store"
)

func TestParseFlags_S7Backend011(t *testing.T) {
	t.Parallel()

	t.Run("defaults", func(t *testing.T) {
		t.Parallel()
		got, err := parseFlags(nil, &bytes.Buffer{})
		if err != nil {
			t.Fatal(err)
		}
		want := options{
			limit:             50,
			olderThan:         store.PurgeAfter,
			deleteAuth:        true,
			feedbackOlderThan: store.FeedbackRetention,
			feedbackLimit:     500,
		}
		if got != want {
			t.Fatalf("got %+v, want %+v", got, want)
		}
	})

	t.Run("every flag", func(t *testing.T) {
		t.Parallel()
		got, err := parseFlags([]string{
			"-dry-run", "-limit=7", "-older-than=72h", "-delete-auth=false",
			"-feedback-older-than", "48h", "-feedback-limit", "9",
		}, &bytes.Buffer{})
		if err != nil {
			t.Fatal(err)
		}
		want := options{
			dryRun:            true,
			limit:             7,
			olderThan:         72 * time.Hour,
			deleteAuth:        false,
			feedbackOlderThan: 48 * time.Hour,
			feedbackLimit:     9,
		}
		if got != want {
			t.Fatalf("got %+v, want %+v", got, want)
		}
	})

	t.Run("bad values and flags are errors", func(t *testing.T) {
		t.Parallel()
		for _, args := range [][]string{
			{"-limit=abc"},
			{"-limit="},
			{"-older-than=3"},
			{"-older-than=soon"},
			{"-feedback-older-than=x"},
			{"-feedback-limit=1.5"},
			{"-dry-run=maybe"},
			{"-delete-auth=2x"},
			{"-nope"},
			{"-limit"},
		} {
			var out bytes.Buffer
			got, err := parseFlags(args, &out)
			if err == nil {
				t.Errorf("%v: want an error, got %+v", args, got)
				continue
			}
			if got != (options{}) {
				t.Errorf("%v: options must be zero on error, got %+v", args, got)
			}
			if out.Len() == 0 {
				t.Errorf("%v: the error must be written to out", args)
			}
		}
	})

	t.Run("help is flag.ErrHelp and lists every flag", func(t *testing.T) {
		t.Parallel()
		var out bytes.Buffer
		_, err := parseFlags([]string{"-h"}, &out)
		if !errors.Is(err, flag.ErrHelp) {
			t.Fatalf("got %v", err)
		}
		for _, name := range []string{"dry-run", "limit", "older-than", "delete-auth", "feedback-older-than", "feedback-limit"} {
			if !strings.Contains(out.String(), "-"+name) {
				t.Errorf("usage lacks -%s: %s", name, out.String())
			}
		}
	})

	t.Run("values below the floor are parsed as given (run refuses them)", func(t *testing.T) {
		t.Parallel()
		got, err := parseFlags([]string{"-older-than=1h", "-limit=0"}, &bytes.Buffer{})
		if err != nil || got.olderThan != time.Hour || got.limit != 0 {
			t.Fatalf("%+v %v", got, err)
		}
	})
}
