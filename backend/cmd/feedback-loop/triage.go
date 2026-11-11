package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/jphacks/kz_2601/backend/internal/ai"
	"github.com/jphacks/kz_2601/backend/internal/loop"
	"github.com/jphacks/kz_2601/backend/internal/loop/triage"
	"github.com/jphacks/kz_2601/backend/internal/loop/triage/eval"
)

// Env of the triage model. The key is the loop's own (#365 decision 7: not
// the app's ANTHROPIC_API_KEY), so the loop's cost is separate and can be
// revoked alone.
const (
	envTriageKey   = "LOOP_ANTHROPIC_API_KEY"
	envTriageModel = "LOOP_TRIAGE_MODEL"
)

func (a *app) triageModel() (triage.Model, error) {
	if a.model != nil {
		return a.model, nil
	}
	key := a.getenv(envTriageKey)
	if key == "" {
		return nil, errors.New(envTriageKey + " is not set")
	}
	model := a.getenv(envTriageModel)
	if model == "" {
		model = ai.DefaultRecipeModel
	}
	return triage.AIModel{C: ai.NewClient(key, model, "")}, nil
}

func (a *app) triage(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("triage", flag.ContinueOnError)
	maxItems := fs.Int("max", triage.DefaultMaxItems, "max items per run")
	batch := fs.Int("batch", triage.DefaultBatch, "distinct texts per call (<= 50)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	g := a.gate()
	if err := g.AllowLLM(ctx); err != nil {
		return g.Stopped(ctx, loop.StageTriage, err)
	}
	m, err := a.triageModel()
	if err != nil {
		return err
	}
	t := &triage.Triager{Store: a.loop, Model: m, Batch: *batch, MaxItems: *maxItems, Now: a.now,
		// re-checked before every call: a switch flipped mid-run, or the
		// monthly cap reached, stops the next call
		BeforeCall: func(ctx context.Context) error {
			if err := g.AllowLLM(ctx); err != nil {
				return g.Stopped(ctx, loop.StageTriage, err)
			}
			return nil
		},
		AfterCall: func(ctx context.Context, u triage.Usage, callErr error) error {
			if _, err := a.control.AddUsage(ctx, loop.Month(a.now()), u.Total()); err != nil {
				return err
			}
			e := loop.Event{Stage: loop.StageTriage, Action: "llm_call", Tokens: u.Total()}
			if callErr != nil {
				e.Action, e.Reason = "llm_call_failed", "error"
			}
			return g.Record(ctx, e)
		},
		OnItem: func(ctx context.Context, it loop.Item) error {
			e := loop.Event{Stage: loop.StageTriage, Action: it.Status, ItemID: it.ID, Reason: it.HoldReason}
			if it.Status == loop.ItemTriaged || it.Status == loop.ItemSpam {
				e.Reason = it.Category + "/" + it.Priority
			}
			return g.Record(ctx, e)
		},
	}
	res, err := t.Run(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "triage: items %d, calls %d, triaged %d (duplicates %d), spam %d, held %d (rejected %d), tokens %d\n",
		res.Items, res.Calls, res.Triaged, res.Duplicates, res.Spam, res.Held, res.Rejected, res.Usage.Total())
	if res.Stopped != "" {
		fmt.Fprintf(a.out, "triage: stopped early: %s\n", res.Stopped)
	}
	return nil
}

// triageEval runs triage on a hand-labelled set in memory (nothing is
// written to Firestore) and prints the agreement. It reports; it does not
// fail on a low score. -model ai calls the loop's model under the triage
// switch and the monthly cap; -model rules (default) is offline.
func (a *app) triageEval(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("triage-eval", flag.ContinueOnError)
	path := fs.String("set", "", "labelled set (JSON lines: id, text, category, priority, bundle)")
	model := fs.String("model", "rules", "rules (offline keyword baseline) or ai (the triage model)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *path == "" {
		return errors.New("triage-eval needs -set")
	}
	f, err := os.Open(*path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	cases, err := eval.Load(f)
	if err != nil {
		return fmt.Errorf("%s: %w", *path, err)
	}
	var m triage.Model
	switch *model {
	case "rules":
		m = eval.Rules{}
	case "ai":
		g := a.gate()
		if err := g.AllowLLM(ctx); err != nil {
			return g.Stopped(ctx, loop.StageTriage, err)
		}
		if m, err = a.triageModel(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown -model %q (rules or ai)", *model)
	}
	rep, err := eval.Run(ctx, m, cases)
	if t := rep.Usage.Total(); t > 0 {
		if _, uerr := a.control.AddUsage(ctx, loop.Month(a.now()), t); uerr != nil {
			return uerr
		}
		if rerr := a.gate().Record(ctx, loop.Event{Stage: loop.StageTriage, Action: "eval_call", Tokens: t}); rerr != nil {
			return rerr
		}
	}
	if err != nil {
		return err
	}
	a.out.WriteString(rep.String())
	return nil
}

// triageScore reads "itemId,category" lines (a human's weekly labels) and
// prints how many the model got the same (#376: 8 割以上).
func (a *app) triageScore(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("triage-score", flag.ContinueOnError)
	path := fs.String("labels", "", "file of itemId,category lines")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *path == "" {
		return errors.New("triage-score needs -labels")
	}
	f, err := os.Open(*path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	labels := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		id, cat, ok := strings.Cut(strings.TrimSpace(sc.Text()), ",")
		if ok && id != "" && !strings.HasPrefix(id, "#") {
			labels[strings.TrimSpace(id)] = strings.TrimSpace(cat)
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	var items []loop.Item
	for id := range labels {
		it, err := a.loop.GetItem(ctx, id)
		if errors.Is(err, loop.ErrNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		items = append(items, *it)
	}
	m, n := triage.Agreement(items, labels)
	if n == 0 {
		fmt.Fprintln(a.out, "triage-score: no labelled item has a model category")
		return nil
	}
	fmt.Fprintf(a.out, "triage-score: %d/%d agree (%.0f%%)\n", m, n, 100*float64(m)/float64(n))
	return nil
}
