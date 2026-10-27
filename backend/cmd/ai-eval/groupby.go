package main

import (
	"fmt"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Grouped table (#261) expect key: groupedTable (bool: some table has groupBy
// and at least one aggregate column, i.e. "店ごとの平均点" is one row per shop).
// Cases: internal/ai/eval/groupby_cases.json (-cases).

// groupedTables lists "entity:groupBy" of grouped tables in the app.
func groupedTables(a *dsl.App) []string {
	var out []string
	for _, s := range a.Screens {
		for _, c := range s.Components {
			for _, x := range append([]dsl.Component{c}, c.Children...) {
				if x.Type != dsl.CompTable || x.GroupBy == "" {
					continue
				}
				for _, col := range x.Columns {
					if col.Aggregate != "" {
						out = append(out, x.Entity+":"+x.GroupBy)
						break
					}
				}
			}
		}
	}
	return out
}

func checkGroupBy(exp map[string]any, a *dsl.App) []string {
	want, ok := exp["groupedTable"].(bool)
	if !ok {
		return nil
	}
	got := groupedTables(a)
	if (len(got) > 0) != want {
		return []string{fmt.Sprintf("groupedTable=%v", got)}
	}
	return nil
}
