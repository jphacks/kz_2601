package ai

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// 有料機能の判定(#77)。chat の ready / proposal に「作る予定の内容が Tailor It Pro を要するか」を付ける。
// リストは paid_features.json の 1 か所。判定は generate に渡す文(transcript / instruction)の語で行う:
// 生成プロンプト(prompt.go の entity.scope)が同じ語で scope:public を選ぶので、生成前に同じ結果を予測できる。
// 課金の強制(エンタイトルメント)はしない。iOS が確認カードを出すための情報だけ(後続: レシート検証 + appconfig)。

//go:embed paid_features.json
var paidFeaturesJSON []byte

type localized struct {
	JA string `json:"ja"`
	EN string `json:"en"`
}

func (l localized) in(loc Locale) string {
	if loc == LocaleEN && l.EN != "" {
		return l.EN
	}
	return l.JA
}

type paidFeatureDef struct {
	ID                 string    `json:"id"`
	Enabled            bool      `json:"enabled"`
	Title              localized `json:"title"`
	Reason             localized `json:"reason"`
	Keywords           []string  `json:"keywords"`
	ExcludeKeywords    []string  `json:"excludeKeywords"`
	DSLEntityScopes    []string  `json:"dslEntityScopes"`
	WithoutInstruction localized `json:"withoutInstruction"`
}

// PaidFeature is one paid feature in a chat reply (openapi `PaidFeature`).
type PaidFeature struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Reason string `json:"reason"`
}

// PaidCatalog is the parsed paid_features.json.
type PaidCatalog struct {
	features []paidFeatureDef
}

var defaultPaidCatalog = mustPaidCatalog(paidFeaturesJSON)

func mustPaidCatalog(raw []byte) *PaidCatalog {
	c, err := ParsePaidCatalog(raw)
	if err != nil {
		panic(err)
	}
	return c
}

// ParsePaidCatalog reads a paid_features.json document; disabled entries are dropped.
func ParsePaidCatalog(raw []byte) (*PaidCatalog, error) {
	var doc struct {
		Features []paidFeatureDef `json:"features"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("paid_features.json: %w", err)
	}
	c := &PaidCatalog{}
	seen := map[string]bool{}
	for _, f := range doc.Features {
		if f.ID == "" || seen[f.ID] {
			return nil, fmt.Errorf("paid_features.json: empty or duplicate id %q", f.ID)
		}
		seen[f.ID] = true
		if f.Enabled {
			c.features = append(c.features, f)
		}
	}
	return c, nil
}

func (f paidFeatureDef) public(loc Locale) PaidFeature {
	return PaidFeature{ID: f.ID, Title: f.Title.in(loc), Reason: f.Reason.in(loc)}
}

// Detect returns the paid features the text asks for (generation is driven by this text).
func (c *PaidCatalog) Detect(text string, loc Locale) []PaidFeature {
	t := strings.ToLower(text)
	var out []PaidFeature
	for _, f := range c.features {
		s := t
		for _, ex := range f.ExcludeKeywords {
			s = strings.ReplaceAll(s, strings.ToLower(ex), " ")
		}
		for _, kw := range f.Keywords {
			if kw != "" && strings.Contains(s, strings.ToLower(kw)) {
				out = append(out, f.public(loc))
				break
			}
		}
	}
	return out
}

// InApp returns the paid features a DSL actually uses (`dslEntityScopes`).
func (c *PaidCatalog) InApp(a *dsl.App, loc Locale) []PaidFeature {
	if a == nil {
		return nil
	}
	var out []PaidFeature
	for _, f := range c.features {
		if appUsesScopes(a, f.DSLEntityScopes) {
			out = append(out, f.public(loc))
		}
	}
	return out
}

func appUsesScopes(a *dsl.App, scopes []string) bool {
	for _, e := range a.Entities {
		for _, s := range scopes {
			if string(e.Scope) == s {
				return true
			}
		}
	}
	return false
}

// Without returns text plus the instructions that drop the given features (「この機能なしで作る」).
func (c *PaidCatalog) Without(text string, drop []PaidFeature, loc Locale) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(text))
	for _, p := range drop {
		for _, f := range c.features {
			if f.ID == p.ID {
				b.WriteString("\n")
				b.WriteString(f.WithoutInstruction.in(loc))
			}
		}
	}
	return b.String()
}

// newPaid drops features the current app already has (editing an app that is
// already shared does not ask again).
func newPaid(found, existing []PaidFeature) []PaidFeature {
	if len(existing) == 0 {
		return found
	}
	has := map[string]bool{}
	for _, e := range existing {
		has[e.ID] = true
	}
	var out []PaidFeature
	for _, f := range found {
		if !has[f.ID] {
			out = append(out, f)
		}
	}
	return out
}

// annotate sets requiresPro / paidFeatures / free* on a ready or proposal reply.
// current = the app being edited (nil for create).
func (c *PaidCatalog) annotate(res *ChatResult, current *dsl.App) {
	if res == nil {
		return
	}
	existing := c.InApp(current, res.Locale)
	switch res.Kind {
	case "ready":
		if found := newPaid(c.Detect(res.Transcript, res.Locale), existing); len(found) > 0 {
			res.RequiresPro = true
			res.PaidFeatures = found
			res.FreeTranscript = c.Without(res.Transcript, found, res.Locale)
		}
	case "proposal":
		for i := range res.Options {
			o := &res.Options[i]
			if found := newPaid(c.Detect(o.Instruction, res.Locale), existing); len(found) > 0 {
				o.RequiresPro = true
				o.PaidFeatures = found
				o.FreeInstruction = c.Without(o.Instruction, found, res.Locale)
			}
		}
	}
}
