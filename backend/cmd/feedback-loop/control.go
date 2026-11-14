package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/loop"
)

// switches: `switch show` / `switch set -by NAME <stage|all> <on|off>`.
// Setting "off" stops a stage at its next check without a deploy. Setting
// "on" only lifts a Firestore stop; the stage still needs LOOP_<STAGE>=on.
func (a *app) switches(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] == "show" {
		return a.switchShow(ctx)
	}
	if args[0] != "set" {
		return fmt.Errorf("unknown switch command %q", args[0])
	}
	fs := flag.NewFlagSet("switch set", flag.ContinueOnError)
	by := fs.String("by", "", "your name")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if strings.TrimSpace(*by) == "" || fs.NArg() != 2 {
		return errors.New("usage: switch set -by NAME <stage|all> <on|off>")
	}
	name, val := fs.Arg(0), fs.Arg(1)
	if name != loop.SwitchAll && !slices.Contains(loop.Stages, name) {
		return fmt.Errorf("unknown stage %q (%s, %s)", name, strings.Join(loop.Stages, ", "), loop.SwitchAll)
	}
	if val != "on" && val != "off" {
		return errors.New("value must be on or off")
	}
	if err := a.control.SetSwitch(ctx, name, val == "on", *by, a.now().UTC()); err != nil {
		return err
	}
	if err := a.gate().Record(ctx, loop.Event{Stage: name, Action: "switch_" + val, Actor: *by}); err != nil {
		return err
	}
	fmt.Fprintf(a.out, "switch %s = %s\n", name, val)
	return a.switchShow(ctx)
}

func (a *app) switchShow(ctx context.Context) error {
	g := a.gate()
	sw, err := a.control.Switches(ctx)
	if err != nil {
		fmt.Fprintf(a.out, "switches unreadable (every stage is stopped): %v\n", err)
	}
	fmt.Fprintf(a.out, "%-8s %-5s %-9s %s\n", "stage", "env", "firestore", "runs?")
	for _, s := range loop.Stages {
		fsv := "-"
		if v, ok := sw[s]; ok {
			fsv = map[bool]string{true: "on", false: "off"}[v]
		}
		runs := "yes"
		if err := g.Allow(ctx, s); err != nil {
			runs = "no: " + strings.TrimPrefix(err.Error(), loop.ErrStopped.Error()+": ")
		}
		env := a.getenv(loop.EnvName(s))
		if env == "" {
			env = "-"
		}
		fmt.Fprintf(a.out, "%-8s %-5s %-9s %s\n", s, env, fsv, runs)
	}
	used, err := a.control.Usage(ctx, loop.Month(a.now()))
	if err == nil {
		fmt.Fprintf(a.out, "triage tokens this month: %d / %d\n", used, g.TokenCap())
	}
	return nil
}

// audit prints the log, or with -summary the weekly review (#375: 何件直った・
// 何件戻した・費用).
func (a *app) audit(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("audit", flag.ContinueOnError)
	since := fs.Duration("since", 7*24*time.Hour, "events newer than this")
	draftID := fs.String("draft", "", "only this draft")
	itemID := fs.String("item", "", "only this inbox item")
	summary := fs.Bool("summary", false, "counts per stage and action instead of the events")
	limit := fs.Int("limit", 200, "max events")
	if err := fs.Parse(args); err != nil {
		return err
	}
	q := loop.AuditQuery{DraftID: *draftID, ItemID: *itemID, Limit: *limit}
	if *since > 0 {
		q.Since = a.now().Add(-*since)
	}
	if *summary {
		q.Limit = 500
	}
	evs, err := a.control.ListAudit(ctx, q)
	if err != nil {
		return err
	}
	if !*summary {
		for _, e := range evs {
			fmt.Fprintf(a.out, "%s %-8s %-16s %-10s %s\n", e.At.In(jst).Format("2006-01-02 15:04"), e.Stage, e.Action, e.Actor, eventRefs(e))
		}
		fmt.Fprintf(a.out, "%d event(s)\n", len(evs))
		return nil
	}
	counts, tokens := map[string]int{}, 0
	for _, e := range evs {
		counts[e.Stage+" "+e.Action]++
		tokens += e.Tokens
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Fprintf(a.out, "since %s\n", q.Since.In(jst).Format("2006-01-02 15:04"))
	for _, k := range keys {
		fmt.Fprintf(a.out, "  %-32s %d\n", k, counts[k])
	}
	fmt.Fprintf(a.out, "  %-32s %d\n", "triage tokens", tokens)
	return nil
}

var jst = time.FixedZone("JST", 9*60*60)

func eventRefs(e loop.Event) string {
	parts := []string{}
	add := func(k, v string) {
		if v != "" && v != "0" {
			parts = append(parts, k+"="+v)
		}
	}
	add("item", e.ItemID)
	add("feedback", e.FeedbackID)
	add("draft", e.DraftID)
	if e.Issue > 0 {
		add("issue", fmt.Sprintf("%s#%d", e.IssueRepo, e.Issue))
	}
	add("pr", fmt.Sprint(e.PR))
	add("reason", e.Reason)
	add("tokens", fmt.Sprint(e.Tokens))
	return strings.Join(parts, " ")
}
