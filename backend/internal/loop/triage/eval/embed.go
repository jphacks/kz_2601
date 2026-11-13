package eval

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jphacks/kz_2601/backend/internal/loop/triage"
)

// DefaultEmbedThreshold is the cosine at or above which two clusters'
// representatives are merged for the comparison.
const DefaultEmbedThreshold = 0.7

// BundleStats is one way of bundling the set: how many bundles the cases
// ended up in and how the pairs compare with the labels.
type BundleStats struct {
	Bundles  int
	Together Score
	Apart    Score
}

// Mistakes are the pairs that differ from the labels (split or mixed up).
func (s BundleStats) Mistakes() int {
	return s.Together.Total - s.Together.Matched + s.Apart.Total - s.Apart.Matched
}

// EmbedCompare is the bundling by the model's cluster names (Before) next to
// the bundling after merging clusters whose representative summaries are near
// (After, #2148). Measurement only: the loop never merges.
type EmbedCompare struct {
	Threshold     float64
	Before, After BundleStats
}

// String is the printed comparison.
func (c EmbedCompare) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "  embedding merge (cosine >= %.2f)\n", c.Threshold)
	fmt.Fprintf(&b, "    %-7s %8s %9s %12s %12s\n", "", "bundles", "mistakes", "together", "apart")
	for _, r := range []struct {
		name string
		s    BundleStats
	}{{"before", c.Before}, {"after", c.After}} {
		fmt.Fprintf(&b, "    %-7s %8d %9d %12s %12s\n", r.name, r.s.Bundles, r.s.Mistakes(), r.s.Together, r.s.Apart)
	}
	return b.String()
}

// ErrNoRun: CompareEmbed on a report that Run did not fill.
var ErrNoRun = errors.New("triage eval: compare needs a report from Run")

// CompareEmbed merges the run's clusters whose representative (the first
// case of the cluster, in set order) has a summary within threshold of
// another's, transitively, and stores both bundlings in r.Embed. All
// summaries go to e in one call.
func (r *Report) CompareEmbed(ctx context.Context, e triage.Embedder, threshold float64) error {
	if len(r.labels) == 0 || len(r.clusters) != len(r.labels) {
		return ErrNoRun
	}
	if threshold <= 0 {
		threshold = DefaultEmbedThreshold
	}
	keys, rep := []string{}, map[string]string{}
	for i, k := range r.clusters {
		if _, ok := rep[k]; k != "" && !ok {
			keys = append(keys, k)
			rep[k] = r.summaries[i]
		}
	}
	sort.Strings(keys)
	parent := map[string]string{}
	var find func(string) string
	find = func(k string) string {
		if p, ok := parent[k]; ok && p != k {
			parent[k] = find(p)
			return parent[k]
		}
		return k
	}
	if len(keys) > 1 {
		texts := make([]string, len(keys))
		for i, k := range keys {
			texts[i] = rep[k]
		}
		vs, err := triage.Embed(ctx, e, texts)
		if err != nil {
			return err
		}
		for i := range keys {
			for j := i + 1; j < len(keys); j++ {
				s, err := triage.Cosine(vs[i], vs[j])
				if err != nil {
					return err
				}
				if s >= threshold {
					a, b := find(keys[i]), find(keys[j])
					if a != b {
						parent[max(a, b)] = min(a, b) // the first name stays
					}
				}
			}
		}
	}
	merged := make([]string, len(r.clusters))
	for i, k := range r.clusters {
		if k != "" {
			merged[i] = find(k)
		}
	}
	c := EmbedCompare{Threshold: threshold, Before: stats(r.labels, r.clusters), After: stats(r.labels, merged)}
	r.Embed = &c
	return nil
}

func stats(cases []Case, clusters []string) BundleStats {
	seen := map[string]bool{}
	for _, k := range clusters {
		if k != "" {
			seen[k] = true
		}
	}
	t, a := pairScores(cases, clusters, nil)
	return BundleStats{Bundles: len(seen), Together: t, Apart: a}
}
