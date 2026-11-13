package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/loop/triage"
)

var areaCases = []struct{ text, want string }{
	{"課金したのに機能が使えない", "billing"},
	{"プランの料金はいくらですか", "billing"},
	{"写真を足すと落ちる", "render"},
	{"共有したら相手の画面が真っ白", "share"},
	{"公開の範囲を選びたい", "share"},
	{"頼んだアプリが作れなかった", "generate"},
	{"家計簿を作ってほしい", "generate"},
	{"ダークモードがほしい", "render"},
	{"ありがとう", "other"},
	{"なんとなく", "other"},
}

// The rules model returns an allowed area from simple keywords (#2363).
func TestRulesArea(t *testing.T) {
	var user strings.Builder
	for i, c := range areaCases {
		if got := classify(c.text).area; got != c.want {
			t.Errorf("%s: area %q want %q", c.text, got, c.want)
		}
		fmt.Fprintf(&user, "<feedback-abc123 id=\"i%d\" source=in_app>\n%s\n</feedback-abc123>\n", i+1, c.text)
	}
	out, _, err := Rules{}.Complete(context.Background(), triage.System, user.String(), triage.SchemaJSON())
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Items []struct {
			Area string `json:"area"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil || len(got.Items) != len(areaCases) {
		t.Fatalf("rows: %v %s", err, out)
	}
	for _, r := range got.Items {
		if !slices.Contains(triage.Areas, r.Area) {
			t.Fatalf("area %q not allowed", r.Area)
		}
	}
}
