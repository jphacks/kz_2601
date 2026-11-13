package triage

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"strings"
)

// Embeddings for the cluster suggestions (#2148). Only the masked summary of
// an item is embedded, never its body. The source of the vectors is the
// Embedder interface so it can be swapped; a real one and LOOP_EMBED are a
// follow-up. Nothing here merges clusters: a high similarity is only a
// candidate for the draft.

// Embedder turns summaries into vectors: one per summary, in order, all of
// the same length.
type Embedder interface {
	Embed(ctx context.Context, summaries []string) ([][]float32, error)
}

// Embed calls e and checks its answer (count and lengths).
func Embed(ctx context.Context, e Embedder, summaries []string) ([][]float32, error) {
	if len(summaries) == 0 {
		return [][]float32{}, nil
	}
	vs, err := e.Embed(ctx, summaries)
	if err != nil {
		return nil, err
	}
	if len(vs) != len(summaries) {
		return nil, fmt.Errorf("embed: %d vectors for %d summaries", len(vs), len(summaries))
	}
	for i, v := range vs {
		if len(v) == 0 || len(v) != len(vs[0]) {
			return nil, fmt.Errorf("embed: vector %d has length %d, want %d", i, len(v), len(vs[0]))
		}
	}
	return vs, nil
}

// FakeEmbedder is a fixed embedder for tests and dry runs: the character
// bigrams of the summary (spaces dropped, lower case) hashed into Dim
// buckets (16 when 0). The same text
// always gives the same vector, and texts sharing words are near.
type FakeEmbedder struct{ Dim int }

// Embed implements Embedder.
func (f FakeEmbedder) Embed(_ context.Context, summaries []string) ([][]float32, error) {
	dim := f.Dim
	if dim <= 0 {
		dim = 16
	}
	out := make([][]float32, len(summaries))
	for i, s := range summaries {
		v := make([]float32, dim)
		rs := []rune(strings.ToLower(strings.Join(strings.Fields(s), "")))
		for j := range rs {
			h := fnv.New32a()
			end := min(j+2, len(rs))
			_, _ = h.Write([]byte(string(rs[j:end])))
			v[h.Sum32()%uint32(dim)]++
		}
		out[i] = v
	}
	return out, nil
}

// ErrDimMismatch: two vectors of different lengths.
var ErrDimMismatch = errors.New("embed: vectors of different lengths")

// Cosine is the cosine similarity of a and b, in [-1, 1]. A zero vector is
// near nothing (0).
func Cosine(a, b []float32) (float64, error) {
	if len(a) != len(b) {
		return 0, ErrDimMismatch
	}
	var dot, na, nb float64
	for i := range a {
		x, y := float64(a[i]), float64(b[i])
		dot += x * y
		na += x * x
		nb += y * y
	}
	if na == 0 || nb == 0 {
		return 0, nil
	}
	return math.Max(-1, math.Min(1, dot/math.Sqrt(na*nb))), nil
}

// NearestCluster is the cluster whose representative is nearest to v and
// that similarity. ok is false when there is no cluster. Ties go to the
// name first in order, so the answer does not depend on map order.
func NearestCluster(v []float32, reps map[string][]float32) (name string, sim float64, ok bool, err error) {
	for n, r := range reps {
		s, err := Cosine(v, r)
		if err != nil {
			return "", 0, false, fmt.Errorf("cluster %q: %w", n, err)
		}
		if !ok || s > sim || (s == sim && n < name) {
			name, sim, ok = n, s, true
		}
	}
	return name, sim, ok, nil
}
