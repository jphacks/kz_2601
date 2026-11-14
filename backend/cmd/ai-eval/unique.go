package main

import (
	"fmt"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// unique expect key, checked by checkUnique:
//
//	unique (bool)  some entity has a non-empty entity.unique (field ids or
//	               "$author", pkg/dsl/data_rules.go)
//
// Cases: internal/ai/eval/jphacks_products_cases.json jp_slot_booking and
// m0disc_cases.json d_reserve.
func init() {
	expectKeys["unique"] = expectBool
}

// uniqueEntities lists "entity[f1 f2]" for every entity with unique set.
func uniqueEntities(a *dsl.App) []string {
	var out []string
	for _, e := range a.Entities {
		if len(e.Unique) > 0 {
			out = append(out, fmt.Sprintf("%s%v", e.ID, e.Unique))
		}
	}
	return out
}

func checkUnique(exp map[string]any, a *dsl.App) []string {
	want, ok := exp["unique"].(bool)
	if !ok {
		return nil
	}
	got := uniqueEntities(a)
	if (len(got) > 0) != want {
		return []string{fmt.Sprintf("unique=%v", got)}
	}
	return nil
}
