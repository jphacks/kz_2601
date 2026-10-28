package dsl

import (
	"strings"
	"testing"
)

// #425: passby exchanges only explicitly listed, safe fields of a private entity.
func TestNearbySharePassby(t *testing.T) {
	ok := memoApp()
	ok.Entities[0].Fields = append(ok.Entities[0].Fields,
		Field{ID: "secret", Name: "s", Type: FieldString, Hidden: true},
		Field{ID: "pic", Name: "p", Type: FieldPhoto},
		Field{ID: "len", Name: "l", Type: FieldInteger, Computed: &Expr{Kind: ExprLiteral, Lit: float64(1)}},
	)
	ok.Screens[0].Components = []Component{
		{Type: CompNearbyShare, Label: "すれ違い", Entity: "memo", Mode: NearbyModePassby, Fields: []string{"note", "place"}, Limit: 3, Distance: true},
		{Type: CompNearbyShare, Label: "送る", Entity: "memo", Mode: NearbyModeSend},
	}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid passby rejected: %v", err)
	}

	bad := ok
	bad.Screens = append([]Screen(nil), ok.Screens...)
	bad.Screens[0].Components = []Component{
		{Type: CompNearbyShare, Label: "a", Entity: "memo", Mode: NearbyModePassby},
		{Type: CompNearbyShare, Label: "b", Entity: "memo", Mode: NearbyModePassby, Fields: []string{"x"}},
		{Type: CompNearbyShare, Label: "c", Entity: "memo", Mode: NearbyModePassby, Fields: []string{"secret"}},
		{Type: CompNearbyShare, Label: "d", Entity: "memo", Mode: NearbyModePassby, Fields: []string{"pic"}},
		{Type: CompNearbyShare, Label: "e", Entity: "memo", Mode: NearbyModePassby, Fields: []string{"len"}},
		{Type: CompNearbyShare, Label: "f", Entity: "memo", Mode: NearbyModePassby, Fields: []string{"note"}, Limit: 21},
		{Type: CompNearbyShare, Label: "g", Entity: "memo", Fields: []string{"note"}, Distance: true},
		{Type: CompNearbyShare, Label: "h", Entity: "memo", Mode: "auto"},
		{Type: CompNearbyShare, Label: "i", Entity: "memo", Mode: NearbyModePassby, Fields: []string{"note", "note"}},
	}
	ve, isVE := bad.Validate().(*ValidationError)
	if !isVE {
		t.Fatal("expected ValidationError")
	}
	joined := strings.Join(ve.Problems, "\n")
	for _, want := range []string{
		"passby needs fields",
		`passby field "x" not in entity "memo"`,
		`"secret" is hidden`,
		`"pic" is photo`,
		`"len" is computed`,
		"limit 21 must be 1..20",
		`only for mode "passby"`,
		`mode "auto" must be send or passby`,
		`"note" is listed twice`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in:\n%s", want, joined)
		}
	}

	shared := ok
	shared.Entities = append([]Entity(nil), ok.Entities...)
	shared.Entities[0].Scope = ScopePublic
	if err := shared.Validate(); err == nil || !strings.Contains(err.Error(), "must be private") {
		t.Errorf("public passby entity: %v", err)
	}
}

// #425: the new props pass the schema and decode; out-of-range limit is a schema error.
func TestNearbySharePassbyParse(t *testing.T) {
	raw := `{"schemaVersion":1,"app":{"name":"すれ違い","voiceEntry":"none"},"initialScreen":"home",
	"entities":[{"id":"profile","name":"プロフィール","fields":[{"id":"name","name":"名前","type":"string","required":true},{"id":"hobby","name":"趣味","type":"string"}]}],
	"screens":[{"id":"home","title":"すれ違い","components":[
	  {"type":"nearbyShare","label":"すれ違い通信","entity":"profile","mode":"passby","fields":["name","hobby"],"limit":1,"distance":true}]}]}`
	app, err := Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	c := app.Screens[0].Components[0]
	if c.Mode != NearbyModePassby || len(c.Fields) != 2 || !c.Distance || c.Limit != 1 {
		t.Fatalf("decoded %+v", c)
	}
	for _, repl := range []string{`"limit":0,`, `"limit":30,`, `"fields":[],`} {
		in := strings.Replace(raw, `"limit":1,`, repl, 1)
		if repl == `"fields":[],` {
			in = strings.Replace(raw, `"fields":["name","hobby"],`, repl, 1)
		}
		if _, err := Parse([]byte(in)); err == nil {
			t.Errorf("accepted %s", repl)
		}
	}
}
