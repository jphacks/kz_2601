package dsl

import (
	"strings"
	"testing"
)

// #231: 割り勘の「割り勘する人」= type refs(メンバーの id の配列)。
// few-shot の割り勘は split_with + len + filter op contains で通る。
func TestRefsWarikanExample(t *testing.T) {
	app := warikanApp(t)
	exp := app.EntityByID("expenses")
	f := exp.FieldByID("split_with")
	if f == nil || f.Type != FieldRefs || f.RefEntity != "members" {
		t.Fatalf("split_with = %+v", f)
	}
	if err := app.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestRefsRejects(t *testing.T) {
	cases := map[string]struct {
		mutate func(a *App)
		want   string
	}{
		"no refEntity": {func(a *App) { a.EntityByID("expenses").FieldByID("split_with").RefEntity = "" }, "type refs needs refEntity"},
		"eq on refs": {func(a *App) {
			a.EntityByID("members").FieldByID("share").Computed.Agg.Filter[0].Op = "eq"
		}, "is type refs (a list of ids); use op contains"},
		"contains on ref": {func(a *App) {
			a.EntityByID("members").FieldByID("paid").Computed.Agg.Filter[0].Op = "contains"
		}, "filter op contains is only for a type refs field"},
		"refs in div": {func(a *App) {
			a.EntityByID("expenses").FieldByID("per_person").Computed.Args[1] = Expr{Kind: ExprRef, Ref: "split_with"}
		}, "only op len"},
		"refs in textField": {func(a *App) {
			for i := range a.Screens {
				for j := range a.Screens[i].Components {
					for k := range a.Screens[i].Components[j].Children {
						if c := &a.Screens[i].Components[j].Children[k]; c.Field == "split_with" {
							c.Type = CompTextField
						}
					}
				}
			}
		}, "is type refs; use a picker"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			app := warikanApp(t)
			tc.mutate(&app)
			err := app.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
}
