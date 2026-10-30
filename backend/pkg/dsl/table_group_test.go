package dsl

import (
	"os"
	"path/filepath"
	"testing"
)

func groupedTable(groupBy string, sort *Sort, cols ...TableColumn) Component {
	return Component{Type: CompTable, Entity: "expense", GroupBy: groupBy, Columns: cols, Sort: sort}
}

func TestTableGroupValid(t *testing.T) {
	app := displayApp(
		groupedTable("memo", &Sort{Field: "amount", Direction: "desc"},
			TableColumn{Field: "memo", Title: "項目"},
			TableColumn{Field: "amount", Aggregate: "avg", Title: "平均"},
			TableColumn{Field: "memo", Aggregate: "count", Title: "件数"}),
		// sort by the groupBy field = the count column; sort omitted = first aggregate column
		groupedTable("date", &Sort{Field: "date", Direction: "asc"},
			TableColumn{Field: "date"}, TableColumn{Field: "amount", Aggregate: "sum"}),
		groupedTable("memo", nil, TableColumn{Field: "memo"}, TableColumn{Field: "amount", Aggregate: "max"}),
	)
	if err := app.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestTableGroupErrors(t *testing.T) {
	key := TableColumn{Field: "memo"}
	count := TableColumn{Field: "memo", Aggregate: "count"}
	cases := []struct {
		name string
		c    Component
		want string
	}{
		{"aggregate without groupBy", groupedTable("", nil, key, TableColumn{Field: "amount", Aggregate: "sum"}), `aggregate "sum" needs table groupBy`},
		{"unknown groupBy", groupedTable("shop", nil, key, count), `table groupBy "shop" not in entity`},
		{"groupBy photo", groupedTable("receipt", nil, TableColumn{Field: "receipt"}, TableColumn{Field: "receipt", Aggregate: "count"}), `groupBy "receipt" is photo`},
		{"groupBy currency", groupedTable("amount", nil, TableColumn{Field: "amount"}, TableColumn{Field: "amount", Aggregate: "count"}), `groupBy "amount" is currency`},
		{"plain non-key column", groupedTable("memo", nil, key, count, TableColumn{Field: "amount"}), `columns[2] field "amount" needs aggregate`},
		{"bad aggregate", groupedTable("memo", nil, key, TableColumn{Field: "amount", Aggregate: "median"}), `aggregate "median" must be one of`},
		{"avg of string", groupedTable("memo", nil, key, TableColumn{Field: "memo", Aggregate: "avg"}), `avg needs a numeric field but "memo" is string`},
		{"no key column", groupedTable("memo", nil, count), `needs a column {"field":"memo"}`},
		{"no aggregate column", groupedTable("memo", nil, key), "needs at least one aggregate column"},
		{"sort not a column", groupedTable("memo", &Sort{Field: "date", Direction: "desc"}, key, count), `sort field "date" of a grouped table`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { expectProblem(t, displayApp(tc.c), tc.want) })
	}
}

// The bundled ranking example keeps groupBy and the column aggregates.
func TestRankingExampleGroupedTable(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(docsDir, "examples", "ranking.json"))
	if err != nil {
		t.Fatal(err)
	}
	app, err := Parse(raw)
	if err != nil {
		t.Fatalf("ranking.json: %v", err)
	}
	tbl := app.Screens[0].Components[0].Children[0]
	if tbl.Type != CompTable || tbl.GroupBy != "shop" || len(tbl.Columns) != 3 {
		t.Fatalf("table = %+v", tbl)
	}
	if tbl.Columns[1].Aggregate != "avg" || tbl.Columns[2].Aggregate != "count" {
		t.Fatalf("columns = %+v", tbl.Columns)
	}
}
