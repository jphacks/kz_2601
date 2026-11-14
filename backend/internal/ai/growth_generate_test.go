package ai

import (
	"context"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Offline (no live AI): the generation prompt for the demo phrasings carries
// the growth rule and catalog, and a growth DSL coming back from Claude passes
// validation / lint unchanged with the steps bound live.

const flowerSteps = `{"schemaVersion":1,"app":{"name":"歩いて咲く花","description":"歩くほど花が咲く","icon":"camera.macro","voiceEntry":"floating"},
 "initialScreen":"home","entities":[],
 "screens":[{"id":"home","title":"今日の花","components":[
  {"type":"growth","id":"garden","area":"A1:F10","kind":"flower","label":"歩くほど咲く花","metric":"steps","live":true,"target":8000,"step":1000}]}]}`

func growthOf(t *testing.T, raw []byte) dsl.Component {
	t.Helper()
	app, err := dsl.Parse(raw)
	if err != nil {
		t.Fatalf("result DSL invalid: %v", err)
	}
	for _, s := range app.Screens {
		for _, c := range s.Components {
			if c.Type == dsl.CompGrowth {
				return c
			}
		}
	}
	t.Fatalf("no growth in %s", raw)
	return dsl.Component{}
}

func TestGrowthGenerate_DemoPhrasings(t *testing.T) {
	tree := readDoc(t, "../../../docs/dsl/examples/steps_tree.json")
	for _, tc := range []struct {
		transcript, reply, kind string
	}{
		{"歩数を数えて木に実がなるアプリを作って", string(tree), "tree"},
		{"歩くほど花が咲くアプリ", flowerSteps, "flower"},
	} {
		f := &fakeCaller{replies: []string{tc.reply}}
		svc, _ := newSvc(f)
		res, err := svc.Generate(context.Background(), Request{AppID: "a1", Transcript: tc.transcript})
		if err != nil {
			t.Fatalf("%s: %v", tc.transcript, err)
		}
		if res.Meta.Attempts != 1 {
			t.Errorf("%s: regenerated (%d attempts): the growth DSL should pass first time", tc.transcript, res.Meta.Attempts)
		}
		sys := f.calls[0].System
		for _, want := range []string{"## 数値で育つ絵", "「歩数で木に実がなる」", "「歩くほど花が咲く」", `metric "steps"・**"live": true**`} {
			if !strings.Contains(sys, want) {
				t.Errorf("%s: system prompt lacks %q", tc.transcript, want)
			}
		}
		g := growthOf(t, res.DSL)
		if g.Metric != "steps" || !g.Live || g.Kind != tc.kind || g.Entity != "" {
			t.Errorf("%s: growth not bound to live steps: %+v", tc.transcript, g)
		}
	}
}

// The schema-built catalog and index (sent with every generation) describe
// growth and its number sources, so Claude can pick it for other requests too.
func TestGrowthGenerate_CatalogDescribesPart(t *testing.T) {
	for _, want := range []string{"growth", "木に実がなる", "花が咲く", "貯金が増えると木が育つ"} {
		if !strings.Contains(catalog, want) {
			t.Errorf("catalog lacks %q", want)
		}
	}
}

// An iOS build that predates growth never receives it (restrict.go): the
// rule block is empty and a growth reply is regenerated.
func TestGrowthGenerate_OlderClientRetries(t *testing.T) {
	all := make([]string, 0, len(knownComponentTypes))
	for k := range knownComponentTypes {
		if k != "growth" {
			all = append(all, k)
		}
	}
	tree := readDoc(t, "../../../docs/dsl/examples/steps_tree.json")
	health := readDoc(t, "../../../docs/dsl/examples/health.json")
	f := &fakeCaller{replies: []string{string(tree), string(health)}}
	svc, _ := newSvc(f)
	res, err := svc.Generate(context.Background(), Request{AppID: "a1", Transcript: "歩数を数えて木に実がなるアプリを作って", AllowedComponents: all})
	if err != nil {
		t.Fatal(err)
	}
	if res.Meta.Attempts != 2 || !strings.Contains(f.calls[1].Messages[0].Content, `"growth" is not available`) {
		t.Fatalf("expected a retry without growth: %+v", res.Meta)
	}
	if strings.Contains(string(res.DSL), `"growth"`) {
		t.Fatalf("growth reached an older client: %s", res.DSL)
	}
}
