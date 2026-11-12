package appconfig

import "github.com/jphacks/kz_2601/backend/pkg/dsl"

// Fetch-time substitution (#351 §5 "Z", #1268): when a stored DSL reaches a
// client that did not declare a component type, appconfig swaps that
// component for the table's stand-in in the response only (the stored DSL is
// untouched). Whatever is left is drawn by the client as its own fallback
// ("Y"). The header that carries the client's declared types is #353.

// substituteFunc builds the stand-in for c. nil = keep c as it is.
type substituteFunc func(c dsl.Component) *dsl.Component

// keepForClientFallback leaves the component to the client's own fallback.
func keepForClientFallback(dsl.Component) *dsl.Component { return nil }

// fetchSubstitutes is the one Go table of stand-ins (#351 §5). One line per
// type. Types not listed are kept, like keepForClientFallback.
var fetchSubstitutes = map[dsl.ComponentType]substituteFunc{
	// Android declares musicPlayer as fallback only (#1510) and draws the
	// stand-in itself from this component's title: "この端末では再生できません"
	// plus a search in a music app (#879). Replacing it here would lose that.
	dsl.CompMusicPlayer: keepForClientFallback,
}

// substituteUndeclared returns app with every component whose type is not in
// declared replaced by table's stand-in (id and area kept). declared == nil
// means the client did not declare anything: app is returned as is. app
// itself is never modified.
func substituteUndeclared(app *dsl.App, declared map[string]bool, table map[dsl.ComponentType]substituteFunc) *dsl.App {
	if app == nil || declared == nil {
		return app
	}
	out := *app
	out.Screens = make([]dsl.Screen, len(app.Screens))
	for i, s := range app.Screens {
		s.Components = substituteComponents(s.Components, declared, table)
		out.Screens[i] = s
	}
	return &out
}

func substituteComponents(cs []dsl.Component, declared map[string]bool, table map[dsl.ComponentType]substituteFunc) []dsl.Component {
	if cs == nil {
		return nil
	}
	out := make([]dsl.Component, len(cs))
	for i, c := range cs {
		if !declared[string(c.Type)] {
			if f := table[c.Type]; f != nil {
				if r := f(c); r != nil {
					sub := *r
					sub.ID, sub.Area = c.ID, c.Area
					out[i] = sub
					continue
				}
			}
		}
		c.Children = substituteComponents(c.Children, declared, table)
		out[i] = c
	}
	return out
}
