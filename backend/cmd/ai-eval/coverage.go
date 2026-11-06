package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Past JPHACKS works (docs/research/jphacks-coverage.md「測り方」):
// jphacks_coverage_cases.json uses only parts on develop, and
// jphacks_coverage_pending_cases.json needs parts of open PRs. A pending case
// keeps the keys its PR adds in pendingExpect; they are checked once the PR
// adds them to expectKeys. -score reads saved runs and prints the share of
// works whose every expect held (作れた割合) without calling the API.

// mergePendingExpect moves the pendingExpect keys that expectKeys knows into
// Expect and returns the keys still waiting for their PR.
func mergePendingExpect(c *Case) []string {
	var waiting []string
	for k, v := range c.PendingExpect {
		if _, known := expectKeys[k]; !known {
			waiting = append(waiting, k)
			continue
		}
		if c.Expect == nil {
			c.Expect = map[string]any{}
		}
		c.Expect[k] = v
	}
	sort.Strings(waiting)
	return waiting
}

// validatePendingExpect type-checks the pendingExpect keys that are already
// known; unknown ones belong to a PR that is not merged yet.
func validatePendingExpect(exp, pending map[string]any) []string {
	known := map[string]any{}
	var bad []string
	for k, v := range pending {
		if _, dup := exp[k]; dup {
			bad = append(bad, "pendingExpect "+k+" is also in expect")
		}
		if _, ok := expectKeys[k]; ok {
			known[k] = v
		}
	}
	for _, b := range validateExpect(known) {
		bad = append(bad, "pendingExpect: "+b)
	}
	sort.Strings(bad)
	return bad
}

// latestRecords reads raw run files (glob order, so date-named files run
// oldest first) and keeps the last record of each id: a re-run of a few cases
// with -only replaces their earlier result.
func latestRecords(glob string) ([]Record, error) {
	files, err := filepath.Glob(glob)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no files match %q", glob)
	}
	sort.Strings(files)
	byID := map[string]Record{}
	var order []string
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var run Run
		if err := json.Unmarshal(raw, &run); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		for _, r := range run.Cases {
			if _, seen := byID[r.ID]; !seen {
				order = append(order, r.ID)
			}
			byID[r.ID] = r
		}
	}
	out := make([]Record, 0, len(order))
	for _, id := range order {
		out = append(out, byID[id])
	}
	return out, nil
}

// scoreRuns prints the pass rate per group of saved runs and the cases that
// did not pass (generation error or expect misses).
func scoreRuns(glob string) error {
	recs, err := latestRecords(glob)
	if err != nil {
		return err
	}
	hasExpect := map[string]bool{}
	for _, r := range recs {
		hasExpect[r.ID] = r.Checked
	}
	printPassByGroup(passByGroup(recs, hasExpect))
	var failed []string
	for _, r := range recs {
		switch {
		case !r.Checked:
		case !r.OK:
			failed = append(failed, fmt.Sprintf("| %s | %s | error: %s |", r.ID, r.Group, r.Error))
		case len(r.ExpectMisses) > 0:
			failed = append(failed, fmt.Sprintf("| %s | %s | %s |", r.ID, r.Group, strings.Join(r.ExpectMisses, ", ")))
		}
	}
	if len(failed) > 0 {
		fmt.Println("\n| id | group | misses |")
		fmt.Println("|---|---|---|")
		fmt.Println(strings.Join(failed, "\n"))
	}
	return nil
}
