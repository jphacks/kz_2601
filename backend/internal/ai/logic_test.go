package ai

import (
	"os"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestWarikanFewShotMatchesDocs(t *testing.T) {
	docs, err := os.ReadFile("../../../docs/dsl/examples/warikan.json")
	if err != nil {
		t.Skip("docs example not available: ", err)
	}
	if string(docs) != fewShotWarikan {
		t.Fatal("internal/ai/examples/warikan.json is out of sync with docs/dsl/examples/warikan.json")
	}
	for _, want := range []string{`"type":"settlement"`, `"computed":`, `"refEntity":"members"`, "計算式 Expr"} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("system prompt lacks %s", want)
		}
	}
	// stat.value must not get an implied aggregate back.
	filled := fillDefaults([]byte(abbreviateExample(fewShotWarikan)))
	if _, err := dsl.Parse(filled); err != nil {
		t.Fatal(err)
	}
}

func TestFillDefaults_AggFilterList(t *testing.T) {
	in := `{"computed":{"agg":"sum","entity":"e","field":"x","filter":[{"field":"d","op":"thisMonth"},{"field":"p","op":"eq","value":{"ref":"id"}}]}}`
	got := string(fillDefaults([]byte(in)))
	if !strings.Contains(got, `{"field":"d","op":"thisMonth","value":null}`) {
		t.Fatalf("filter list value not filled: %s", got)
	}
}
