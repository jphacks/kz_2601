package triage

import (
	"context"
	"errors"
	"math"
	"testing"
)

func TestCosine(t *testing.T) {
	for _, tc := range []struct {
		name string
		a, b []float32
		want float64
	}{
		{"same", []float32{1, 2, 3}, []float32{1, 2, 3}, 1},
		{"scaled", []float32{1, 2, 3}, []float32{2, 4, 6}, 1},
		{"orthogonal", []float32{1, 0}, []float32{0, 1}, 0},
		{"opposite", []float32{1, -1}, []float32{-1, 1}, -1},
		{"45 degrees", []float32{1, 0}, []float32{1, 1}, 1 / math.Sqrt2},
		{"zero vector", []float32{0, 0}, []float32{1, 1}, 0},
		{"both zero", []float32{0, 0}, []float32{0, 0}, 0},
		{"empty", []float32{}, []float32{}, 0},
	} {
		got, err := Cosine(tc.a, tc.b)
		if err != nil || math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("%s: %v %v, want %v", tc.name, got, err, tc.want)
		}
	}
	if _, err := Cosine([]float32{1}, []float32{1, 2}); !errors.Is(err, ErrDimMismatch) {
		t.Fatalf("length mismatch: %v", err)
	}
}

func TestNearestCluster(t *testing.T) {
	reps := map[string][]float32{
		"save-button": {1, 0, 0},
		"crash":       {0, 1, 0},
		"login":       {0, 0, 1},
	}
	for _, tc := range []struct {
		name string
		v    []float32
		reps map[string][]float32
		want string
		sim  float64
		ok   bool
	}{
		{"nearest", []float32{0.9, 0.1, 0}, reps, "save-button", 0.9 / math.Sqrt(0.82), true},
		{"exact", []float32{0, 0, 2}, reps, "login", 1, true},
		{"tie goes to the first name", []float32{1, 1, 0}, reps, "crash", 1 / math.Sqrt2, true},
		{"all far is still the max", []float32{-1, -1, -1}, reps, "crash", -1 / math.Sqrt(3), true},
		{"no clusters", []float32{1, 0, 0}, map[string][]float32{}, "", 0, false},
		{"nil clusters", []float32{1, 0, 0}, nil, "", 0, false},
	} {
		for range 5 { // map order must not matter
			name, sim, ok, err := NearestCluster(tc.v, tc.reps)
			if err != nil || name != tc.want || ok != tc.ok || math.Abs(sim-tc.sim) > 1e-9 {
				t.Fatalf("%s: %q %v %v %v", tc.name, name, sim, ok, err)
			}
		}
	}
	if _, _, _, err := NearestCluster([]float32{1, 0}, reps); !errors.Is(err, ErrDimMismatch) {
		t.Fatalf("length mismatch: %v", err)
	}
}

func TestFakeEmbedder(t *testing.T) {
	ctx := context.Background()
	vs, err := Embed(ctx, FakeEmbedder{}, []string{
		"保存ボタンが押せない",
		"保存ボタンが 押せない",
		"保存ボタンが反応しない",
		"ログインできない",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 4 || len(vs[0]) != 16 {
		t.Fatalf("%d vectors of %d", len(vs), len(vs[0]))
	}
	same, _ := Cosine(vs[0], vs[1])
	near, _ := Cosine(vs[0], vs[2])
	far, _ := Cosine(vs[0], vs[3])
	if math.Abs(same-1) > 1e-9 {
		t.Errorf("spacing changed the vector: %v", same)
	}
	if near <= far {
		t.Errorf("near %v <= far %v", near, far)
	}
	// fixed: the same text gives the same vector on every call
	again, _ := Embed(ctx, FakeEmbedder{}, []string{"保存ボタンが押せない"})
	if s, _ := Cosine(vs[0], again[0]); s != 1 {
		t.Errorf("not fixed: %v", s)
	}
	if vs, _ := Embed(ctx, FakeEmbedder{Dim: 4}, []string{"a"}); len(vs[0]) != 4 {
		t.Errorf("dim %d", len(vs[0]))
	}
	if vs, err := Embed(ctx, FakeEmbedder{}, nil); err != nil || len(vs) != 0 {
		t.Errorf("no summaries: %v %v", vs, err)
	}
}

type badEmbedder struct {
	vs  [][]float32
	err error
}

func (b badEmbedder) Embed(context.Context, []string) ([][]float32, error) { return b.vs, b.err }

func TestEmbedChecksTheAnswer(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("boom")
	for name, e := range map[string]badEmbedder{
		"error":           {err: boom},
		"too few":         {vs: [][]float32{{1}}},
		"too many":        {vs: [][]float32{{1}, {1}, {1}}},
		"mixed lengths":   {vs: [][]float32{{1, 2}, {1}}},
		"an empty vector": {vs: [][]float32{{}, {}}},
	} {
		if _, err := Embed(ctx, e, []string{"a", "b"}); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	if _, err := Embed(ctx, badEmbedder{vs: [][]float32{{1, 2}, {3, 4}}}, []string{"a", "b"}); err != nil {
		t.Fatal(err)
	}
}
