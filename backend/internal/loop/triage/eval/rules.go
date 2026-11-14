package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"regexp"
	"strings"

	"github.com/jphacks/kz_2601/backend/internal/loop/triage"
)

// Rules is a keyword baseline that answers like the triage model: offline,
// free and deterministic. It is the fixed fake of the eval test and a floor
// to compare a real model with (`triage-eval -model rules`). It is not used
// by the triage command.
type Rules struct{}

type rule struct {
	all, any                     []string
	category, area, impact, slug string
	summary                      string
}

// first match wins; slug "" = a cluster of the text's own
var rules = []rule{
	{any: []string{"稼げる", "販売中", "無視して", "指示", "マージ", "宣伝"}, category: "spam", area: "other", impact: "low", slug: "spam", summary: "ループの対象外の投稿。"},
	{any: []string{"ありがとう", "楽しい", "使いやすい", "最高", "好き"}, category: "praise", area: "other", impact: "low", slug: "praise", summary: "アプリへの好意的な感想。"},
	{all: []string{"課金|購入|支払|請求"}, any: []string{"ない", "二重"}, category: "bug", area: "billing", impact: "high", slug: "billing-not-applied", summary: "支払った後に有料の機能が使えない。"},
	{all: []string{"写真|画像"}, any: []string{"落ちる", "強制終了", "クラッシュ"}, category: "bug", area: "render", impact: "high", slug: "photo-add-crash", summary: "写真を追加するとアプリが終了する。"},
	{any: []string{"落ちる", "強制終了", "クラッシュ"}, category: "bug", area: "other", impact: "high", slug: "app-crash", summary: "アプリが終了する。"},
	{all: []string{"共有"}, any: []string{"真っ白", "表示されない", "開けない"}, category: "bug", area: "share", impact: "medium", slug: "share-blank", summary: "共有したアプリが相手の画面で表示されない。"},
	{any: []string{"作れなかった", "生成に失敗", "作れない"}, category: "feature", area: "generate", impact: "medium", slug: "generate-failed", summary: "頼んだアプリが作れなかった。"},
	{any: []string{"方法", "どうすれば", "？", "?", "わからない", "ますか"}, category: "question", impact: "low", summary: "使い方の質問。"},
	{any: []string{"遅い", "重い"}, category: "bug", area: "render", impact: "medium", slug: "render-slow", summary: "作ったアプリの表示が遅い。"},
	{any: []string{"何も起きない", "動かない", "反応しない", "エラー"}, category: "bug", area: "render", impact: "medium", summary: "操作しても動かない。"},
	{any: []string{"ダークモード"}, category: "feature", area: "render", impact: "low", slug: "dark-mode", summary: "暗い配色の要望。"},
}

var reBlock = regexp.MustCompile(`(?s)<feedback-[0-9a-f*]+ id="(i[0-9]+)"[^>]*>\n(.*?)\n</feedback-`)

// Complete implements triage.Model.
func (Rules) Complete(_ context.Context, _, user string, _ json.RawMessage) (string, triage.Usage, error) {
	rows := []map[string]any{}
	for _, m := range reBlock.FindAllStringSubmatch(user, -1) {
		r := classify(m[2])
		slug := r.slug
		if slug == "" {
			slug = textSlug(r.category, m[2])
		}
		rows = append(rows, map[string]any{"id": m[1], "category": r.category, "area": r.area, "impact": r.impact,
			"cluster": slug, "summary": r.summary, "pii_suspected": false})
	}
	b, err := json.Marshal(map[string]any{"items": rows})
	return string(b), triage.Usage{}, err
}

func classify(text string) rule {
	for _, r := range rules {
		if r.matches(text) {
			if r.area == "" {
				r.area = areaOf(text)
			}
			return r
		}
	}
	// anything else asks for something
	return rule{category: "feature", area: areaOf(text), impact: "low", summary: "機能の要望。"}
}

func (r rule) matches(text string) bool {
	for _, a := range r.all {
		if !containsAny(text, strings.Split(a, "|")) {
			return false
		}
	}
	return len(r.any) == 0 || containsAny(text, r.any)
}

func areaOf(text string) string {
	switch {
	case containsAny(text, []string{"共有", "公開"}):
		return "share"
	case containsAny(text, []string{"プラン", "課金", "料金"}):
		return "billing"
	case containsAny(text, []string{"作って", "生成"}):
		return "generate"
	}
	return "other"
}

func containsAny(s string, subs []string) bool {
	for _, x := range subs {
		if strings.Contains(s, x) {
			return true
		}
	}
	return false
}

// textSlug gives a text no rule bundles a cluster of its own.
func textSlug(category, text string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(triage.NormalizeText(text)))
	return fmt.Sprintf("%s-%08x", category, h.Sum32())
}
