package triage

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/loop"
)

// The area enum in both schemas is exactly Areas (#2363).
func TestAreaEnumMatchesAreas(t *testing.T) {
	for name, raw := range map[string][]byte{"schema": schemaJSON, "wire": wireSchemaJSON} {
		var s struct {
			Properties struct {
				Items struct {
					Items struct {
						Required   []string `json:"required"`
						Properties struct {
							Area struct {
								Enum []string `json:"enum"`
							} `json:"area"`
						} `json:"properties"`
					} `json:"items"`
				} `json:"items"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(raw, &s); err != nil {
			t.Fatal(err)
		}
		row := s.Properties.Items.Items
		if !slices.Equal(row.Properties.Area.Enum, Areas) || !slices.Contains(row.Required, "area") {
			t.Fatalf("%s: area enum %v required %v", name, row.Properties.Area.Enum, row.Required)
		}
	}
}

func TestParseArea(t *testing.T) {
	sent := map[string]bool{"i1": true}
	for _, a := range Areas {
		r := row("i1", "bug", "x", "落ちる。")
		r["area"] = a
		good, _, err := Parse(rows(r), sent)
		if err != nil || len(good) != 1 || good[0].Area != a {
			t.Fatalf("%s: %v %v", a, good, err)
		}
	}
	// a value outside the list, an empty one or none at all: the response is not used
	for name, mut := range map[string]func(map[string]any){
		"unknown": func(r map[string]any) { r["area"] = "payments" },
		"label":   func(r map[string]any) { r["area"] = "agent:ok" },
		"empty":   func(r map[string]any) { r["area"] = "" },
		"missing": func(r map[string]any) { delete(r, "area") },
	} {
		r := row("i1", "bug", "x", "落ちる。")
		mut(r)
		if _, _, err := Parse(rows(r), sent); !errors.Is(err, ErrOutput) {
			t.Fatalf("%s accepted: %v", name, err)
		}
	}
}

func TestTriageStoresArea(t *testing.T) {
	s := loop.NewMemory()
	items := seed(t, s, "課金したのに使えない", "共有したら真っ白")
	m := &fakeModel{respond: func(ids []string, _ string) string {
		a, b := row(ids[0], "bug", "billing-x", "支払った後に使えない。"), row(ids[1], "bug", "share-x", "共有先で表示されない。")
		a["area"], b["area"] = "billing", "share"
		return rows(a, b)
	}}
	if _, err := (&Triager{Store: s, Model: m, nonce: "abc123"}).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := get(t, s, items[0].ID); got.Area != "billing" || got.Status != loop.ItemTriaged {
		t.Fatalf("billing: %+v", got)
	}
	if got := get(t, s, items[1].ID); got.Area != "share" {
		t.Fatalf("share: %+v", got)
	}
}
