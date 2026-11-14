package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Log-only modes: `ai-session-report <mode> -logs FILE [flags]` aggregates
// the exported log lines of every user and prints a table to stdout. Only
// counts and sums leave the process (no uid, no hash, no words). No
// Firestore, no report file, no uid list. Each mode registers itself from its
// own file (init) so modes do not touch each other.

type logMode struct {
	summary string
	run     func(args []string, stdout io.Writer) error
}

var logModes = map[string]logMode{}

func registerLogMode(name string, m logMode) {
	if _, dup := logModes[name]; dup {
		panic("ai-session-report: log mode registered twice: " + name)
	}
	logModes[name] = m
}

// runLogMode runs args[0] when it names a log mode.
func runLogMode(args []string, stdout io.Writer) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	m, ok := logModes[args[0]]
	if !ok {
		return false, nil
	}
	return true, m.run(args[1:], stdout)
}

// logModeNames lists the modes for usage messages.
func logModeNames() string {
	names := make([]string, 0, len(logModes))
	for n := range logModes {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// period is [From, To): whole JST days.
type period struct {
	Label    string
	From, To time.Time
}

func (p period) has(t time.Time) bool { return !t.Before(p.From) && t.Before(p.To) }

// parsePeriod reads "2026-11-01..2026-11-07" (JST dates, both days included).
func parsePeriod(s string) (period, error) {
	a, b, ok := strings.Cut(s, "..")
	if !ok {
		return period{}, fmt.Errorf("period %q: want YYYY-MM-DD..YYYY-MM-DD", s)
	}
	from, err := time.ParseInLocation(time.DateOnly, strings.TrimSpace(a), jst)
	if err != nil {
		return period{}, fmt.Errorf("period %q: %w", s, err)
	}
	last, err := time.ParseInLocation(time.DateOnly, strings.TrimSpace(b), jst)
	if err != nil {
		return period{}, fmt.Errorf("period %q: %w", s, err)
	}
	if last.Before(from) {
		return period{}, fmt.Errorf("period %q: ends before it starts", s)
	}
	return period{Label: s, From: from, To: last.AddDate(0, 0, 1)}, nil
}

// listFlag is a repeatable / comma-separated string flag.
type listFlag []string

func (l *listFlag) String() string { return strings.Join(*l, ",") }
func (l *listFlag) Set(s string) error {
	*l = append(*l, splitList(s)...)
	return nil
}

// newModeFlags is a FlagSet whose errors are returned, not printed + exited.
func newModeFlags(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet("ai-session-report "+name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	return fs
}

// readAllLogs parses every export, keeping every user's lines (counted only).
func readAllLogs(paths []string) ([]logEvent, error) {
	if len(paths) == 0 {
		return nil, errors.New("-logs is required (a `gcloud logging read --format=json` export)")
	}
	var out []logEvent
	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		evs, err := parseLogs(f, nil)
		_ = f.Close()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
		}
		out = append(out, evs...)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out, nil
}
