package ai

import (
	"context"
	"net/http"
	"strings"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/paid"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// 有料機能の判定(#77)と強制(#316)。リストは internal/paid/paid_features.json の 1 か所。
// chat の ready / proposal には generate に渡す文(transcript / instruction)の語で requiresPro を付ける:
// 生成プロンプト(prompt.go の entity.scope)が同じ語で scope:public を選ぶので、生成前に同じ結果を予測できる。
// 保存の前には生成した DSL で判定し、Pro でない作成者には 402 pro_required を返す(gatePaid)。

// PaidFeature is one paid feature in a chat reply (openapi `PaidFeature`).
type PaidFeature = paid.Feature

// PaidCatalog wraps paid.Catalog with the text detection of the chat turn.
type PaidCatalog struct {
	c *paid.Catalog
}

var defaultPaidCatalog = &PaidCatalog{c: paid.Default}

// ParsePaidCatalog reads a paid_features.json document; disabled entries are dropped.
func ParsePaidCatalog(raw []byte) (*PaidCatalog, error) {
	c, err := paid.Parse(raw)
	if err != nil {
		return nil, err
	}
	return &PaidCatalog{c: c}, nil
}

// Detect returns the save-gated paid features the text asks for (generation
// is driven by this text). The 「この機能なしで作る」 instructions are not
// read as asking for anything.
func (c *PaidCatalog) Detect(text string, loc Locale) []PaidFeature {
	for _, f := range c.c.Defs() {
		for _, w := range []string{f.WithoutInstruction.JA, f.WithoutInstruction.EN} {
			if w != "" {
				text = strings.ReplaceAll(text, w, " ")
			}
		}
	}
	t := strings.ToLower(text)
	var out []PaidFeature
	for _, f := range c.c.Defs() {
		if !f.SaveGated() {
			continue // use-gated features are not asked about before generating
		}
		s := t
		for _, ex := range f.ExcludeKeywords {
			s = strings.ReplaceAll(s, strings.ToLower(ex), " ")
		}
		hit := false
		for _, kw := range f.Keywords {
			if kw != "" && strings.Contains(s, strings.ToLower(kw)) {
				hit = true
				break
			}
		}
		// #259: the share words that make generation pick scope public count too
		if !hit && sharesData(f.DSLEntityScopes) {
			in := classifyShareIntent(text)
			hit = in == shareEveryone || in == shareLimited
		}
		if hit {
			out = append(out, f.Public(string(loc)))
		}
	}
	return out
}

func sharesData(scopes []string) bool {
	for _, s := range scopes {
		if s == string(dsl.ScopePublic) || s == string(dsl.ScopeShared) {
			return true
		}
	}
	return false
}

// InApp returns the save-gated paid features a DSL actually uses.
func (c *PaidCatalog) InApp(a *dsl.App, loc Locale) []PaidFeature {
	return c.c.InApp(a, string(loc))
}

// Without returns text plus the instructions that drop the given features (「この機能なしで作る」).
func (c *PaidCatalog) Without(text string, drop []PaidFeature, loc Locale) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(text))
	for _, p := range drop {
		if f, ok := c.c.ByID(p.ID); ok && f.WithoutInstruction.JA != "" {
			b.WriteString("\n")
			b.WriteString(f.WithoutInstruction.In(string(loc)))
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

// dropped reports whether text carries the 「この機能なしで作る」 instruction of
// feature id (freeTranscript / freeInstruction).
func (c *PaidCatalog) dropped(text, id string) bool {
	f, ok := c.c.ByID(id)
	if !ok {
		return false
	}
	for _, w := range []string{f.WithoutInstruction.JA, f.WithoutInstruction.EN} {
		if w != "" && strings.Contains(text, w) {
			return true
		}
	}
	return false
}

// ProRequired is the 402 of a save (or a paid call) the caller's plan does
// not cover. details.features lists what needs Pro.
func ProRequired(features []PaidFeature) *Error {
	return &Error{
		Status: http.StatusPaymentRequired, Code: httpx.CodeProRequired,
		Message: "this needs Tailor It Pro: " + strings.Join(paid.IDs(features), ", "),
		Details: map[string]any{"features": features},
	}
}

// gatePaid refuses to save next when it adds save-gated paid features to
// prev and the caller is not Pro (#316). Without Service.Pro (local runs,
// tests) or a caller uid the check is left to appconfig, which enforces it
// again on PUT / PATCH /dsl.
func (s *Service) gatePaid(ctx context.Context, next []byte, prev *dsl.App, loc Locale) error {
	uid := auth.UID(ctx)
	if s.Pro == nil || uid == "" || !s.ProMode.Checks() {
		return nil
	}
	a, err := dsl.Parse(next)
	if err != nil {
		return nil // validated before; appconfig answers the save
	}
	added := defaultPaidCatalog.c.Added(a, prev, string(loc))
	if len(added) == 0 || s.Pro.IsPro(ctx, uid) || !s.ProMode.Stop(ctx, s.logger(), "ai.generate", uid, paid.IDs(added)) {
		return nil
	}
	return ProRequired(added)
}
