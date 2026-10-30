package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Build restrictions (#245): an iOS build without some parts (App Store
// review build) sends allowedComponents / disabledFeatures, and the AI must
// not use what that binary cannot render. No restriction = the prompts,
// schemas and outputs stay exactly as before (byte-identical prompt cache).

// Known disabledFeatures values; unknown ones are ignored.
const (
	FeatureFieldHealth   = "fieldHealth"   // Field.health (HealthKit write)
	FeatureCameraCapture = "cameraCapture" // photoField / videoField source camera|both
	// FeatureSharedData: entity scope shared / public (records other users
	// see), edit and marks. The App Store review build has no report / block
	// for user content (guideline 1.2), so it keeps every record private (#451 G2).
	FeatureSharedData = "sharedData"
)

// Restriction is the per-request set of unusable parts. The zero value means
// no restriction.
type Restriction struct {
	disallowed map[string]bool // component types the client cannot render
	noHealth   bool
	noCamera   bool
	noShared   bool
	key        string // "" = no restriction; memo key otherwise
}

// knownComponentTypes are the container and leaf types of the canonical schema.
var knownComponentTypes = mustKnownTypes(dsl.SchemaJSON())

func mustKnownTypes(raw []byte) map[string]bool {
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		panic("ai: known types: " + err.Error())
	}
	defs, _ := root["$defs"].(map[string]any)
	out := map[string]bool{}
	container, _ := defs["Container"].(map[string]any)
	for _, t := range enumValues(container, "type") {
		out[t] = true
	}
	for _, name := range refNames(defs["Leaf"]) {
		def, _ := defs[name].(map[string]any)
		if t := constType(def); t != "" {
			out[t] = true
		}
	}
	return out
}

// NewRestriction builds a Restriction from the request fields. An empty
// allowlist (or one naming no known type) allows every component.
func NewRestriction(allowed, disabled []string) Restriction {
	r := Restriction{}
	allow := map[string]bool{}
	for _, t := range allowed {
		if t = strings.TrimSpace(t); knownComponentTypes[t] {
			allow[t] = true
		}
	}
	if len(allow) > 0 {
		for t := range knownComponentTypes {
			if !allow[t] {
				if r.disallowed == nil {
					r.disallowed = map[string]bool{}
				}
				r.disallowed[t] = true
			}
		}
	}
	for _, f := range disabled {
		switch strings.TrimSpace(f) {
		case FeatureFieldHealth:
			r.noHealth = true
		case FeatureCameraCapture:
			r.noCamera = true
		case FeatureSharedData:
			r.noShared = true
		}
	}
	if len(r.disallowed) > 0 || r.noHealth || r.noCamera || r.noShared {
		r.key = strings.Join(r.Disallowed(), ",") + fmt.Sprintf("|h=%t|c=%t|s=%t", r.noHealth, r.noCamera, r.noShared)
	}
	return r
}

// Active reports whether anything is restricted.
func (r Restriction) Active() bool { return r.key != "" }

// NoSharedData reports whether entities must stay private (#451 G2).
func (r Restriction) NoSharedData() bool { return r.noShared }

// Disallowed lists the unusable component types, sorted.
func (r Restriction) Disallowed() []string {
	out := make([]string, 0, len(r.disallowed))
	for t := range r.disallowed {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

type restrictionKey struct{}

// withRestriction attaches r to ctx; every Claude call made with that ctx
// (generate / edit / logic pass / chat) uses the restricted prompt.
func withRestriction(ctx context.Context, r Restriction) context.Context {
	if !r.Active() {
		return ctx
	}
	return context.WithValue(ctx, restrictionKey{}, r)
}

func restrictionFrom(ctx context.Context) Restriction {
	r, _ := ctx.Value(restrictionKey{}).(Restriction)
	return r
}

// restrictedAssets are the schema-derived prompt parts for one Restriction.
type restrictedAssets struct {
	catalog string
	index   string
	genSch  json.RawMessage
}

var (
	assetMemo  sync.Map // key -> *restrictedAssets
	promptMemo sync.Map // key + "\x00" + system -> string
)

func (r Restriction) assets() *restrictedAssets {
	if v, ok := assetMemo.Load(r.key); ok {
		return v.(*restrictedAssets)
	}
	raw := restrictSchemaJSON(dsl.SchemaJSON(), r)
	a := &restrictedAssets{catalog: catalog, index: chatCapabilityIndex, genSch: generationSchema}
	// An allowlist that leaves no leaf cannot build a catalog: keep the full
	// one (the note and enforceRestriction still apply).
	if c, err := BuildCatalog(raw); err == nil {
		a.catalog = c
	}
	if i, err := BuildCapabilityIndex(raw); err == nil {
		a.index = i
	}
	if g, err := Relax(raw); err == nil {
		a.genSch = g
	}
	v, _ := assetMemo.LoadOrStore(r.key, a)
	return v.(*restrictedAssets)
}

// restrictSchemaJSON drops the disallowed containers / leaves, Field.health
// and the camera sources from the canonical schema.
func restrictSchemaJSON(raw []byte, r Restriction) []byte {
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		panic("ai: restrict schema: " + err.Error())
	}
	defs, _ := root["$defs"].(map[string]any)
	if container, _ := defs["Container"].(map[string]any); container != nil {
		props, _ := container["properties"].(map[string]any)
		tp, _ := props["type"].(map[string]any)
		if en, ok := tp["enum"].([]any); ok {
			kept := []any{}
			for _, v := range en {
				if !r.disallowed[fmt.Sprint(v)] {
					kept = append(kept, v)
				}
			}
			tp["enum"] = kept
		}
		if m, ok := tp[keyEnumDescs].(map[string]any); ok {
			for t := range r.disallowed {
				delete(m, t)
			}
		}
	}
	if leaf, _ := defs["Leaf"].(map[string]any); leaf != nil {
		arr, _ := leaf["anyOf"].([]any)
		kept := []any{}
		for _, v := range arr {
			sub, _ := v.(map[string]any)
			ref, _ := sub["$ref"].(string)
			def, _ := defs[strings.TrimPrefix(ref, "#/$defs/")].(map[string]any)
			if r.disallowed[constType(def)] {
				continue
			}
			kept = append(kept, v)
		}
		leaf["anyOf"] = kept
	}
	if r.noHealth {
		if field, _ := defs["Field"].(map[string]any); field != nil {
			props, _ := field["properties"].(map[string]any)
			delete(props, "health")
		}
	}
	if r.noCamera {
		for _, name := range []string{"PhotoField", "VideoField"} {
			def, _ := defs[name].(map[string]any)
			props, _ := def["properties"].(map[string]any)
			if src, _ := props["source"].(map[string]any); src != nil {
				src["enum"] = []any{"library"}
				if m, ok := src[keyEnumDescs].(map[string]any); ok {
					delete(m, "camera")
					delete(m, "both")
				}
			}
		}
	}
	if r.noShared {
		restrictEntitySharing(defs)
	}
	if r.disallowed[string(dsl.CompWebView)] {
		// openUrlButton's description points to webView for embedding (#452 G3)
		for _, v := range defs {
			if def, _ := v.(map[string]any); def != nil {
				if d, ok := def["description"].(string); ok {
					def["description"] = strings.ReplaceAll(d, "サイトをアプリ内に埋め込むなら webView。", "")
				}
			}
		}
	}
	out, err := json.Marshal(root)
	if err != nil {
		panic("ai: restrict schema: " + err.Error())
	}
	return out
}

// restrictEntitySharing (#451 G2) leaves only scope private and drops marks
// (shared/public only). edit stays in the schema because the catalog needs
// its enum; its text says it is unused and enforceRestriction removes it.
func restrictEntitySharing(defs map[string]any) {
	entity, _ := defs["Entity"].(map[string]any)
	props, _ := entity["properties"].(map[string]any)
	if props == nil {
		return
	}
	if sp, _ := props["scope"].(map[string]any); sp != nil {
		sp["enum"] = []any{string(dsl.ScopePrivate)}
		sp["description"] = "この版では private だけ(レコードは本人だけが読み書きする)。他のユーザーと記録を共有できないので、scope は省略する。"
		sp[keyUsage] = "この版では scope を書かない(すべて private)。「みんなで」「共有」「投稿」「ランキング」「掲示板」と言われても、自分だけの記録として作る。"
		if m, ok := sp[keyEnumDescs].(map[string]any); ok {
			delete(m, string(dsl.ScopeShared))
			delete(m, string(dsl.ScopePublic))
		}
	}
	if ep, _ := props["edit"].(map[string]any); ep != nil {
		ep["description"] = "この版では使わない(private の entity では意味を持たない)。"
		delete(ep, keyUsage)
	}
	delete(props, "marks")
}

// restrictSchema swaps the generation schema for the restricted one (the
// output format then cannot name a disallowed type). Other schemas pass.
func restrictSchema(ctx context.Context, schema json.RawMessage) json.RawMessage {
	r := restrictionFrom(ctx)
	if !r.Active() || !bytes.Equal(schema, generationSchema) {
		return schema
	}
	return r.assets().genSch
}

// restrictPrompt returns system unchanged without a restriction; otherwise a
// memoized restricted copy (stable bytes per restriction, so it caches too).
func restrictPrompt(ctx context.Context, system string) string {
	r := restrictionFrom(ctx)
	if !r.Active() {
		return system
	}
	k := r.key + "\x00" + system
	if v, ok := promptMemo.Load(k); ok {
		return v.(string)
	}
	v, _ := promptMemo.LoadOrStore(k, buildRestrictedPrompt(system, r))
	return v.(string)
}

// noWebViewReplacer rewrites the rules that route to webView for builds
// without it (App Store review build, #452 G3): sites open in Safari via
// openUrlButton instead. Drift is caught by TestRestriction_PromptsDropPartsAndAddNote.
var noWebViewReplacer = strings.NewReplacer(
	"「○○のサイト/ニュースを見たい」は webView(実在する https URL。分からなければ公式トップ)。",
	"「○○のサイト/ニュースを見たい」は openUrlButton(実在する https URL を Safari で開く。分からなければ公式トップ)。",
	"webView 部品がある画面のその枠だけ WKWebView で https のページを表示する。",
	"アプリの中でウェブページは表示しない(サイトは openUrlButton で Safari に開く)。",
	"webView でそのサイトを表示",
	"openUrlButton でそのサイトを Safari で開く",
)

// sharingLineRewrites (#451 G2): rule lines (by prefix) about records other
// users see; an empty with drops the line. Drift is caught by
// TestRestriction_NoSharedDataKeepsRecordsPrivate.
var sharingLineRewrites = []struct{ prefix, with string }{
	{"- entity.scope:", "- entity.scope: この版では書かない(記録はすべて自分だけ = private)。「みんなで」「共有」「投稿」「ランキング」「掲示板」「友達と」と言われても、他のユーザーと同じデータを見る作りにはせず、自分だけの記録として作る。"},
	{"- 投稿者と「自分の」", ""},
	{"- entity.edit(#252)", ""},
	{"  - **みんなで使う記録(scope public / shared)", ""},
	{"  - **ほかの人の投稿を自分用に保存**", ""},
	{"- みんなで使う:", "- みんなで使う: このアプリの版では、他の人と同じ記録を見る・投稿する作り(みんなに公開・参加した人だけ・チャット)はできない。記録はすべて自分だけ(private)。頼まれたら「このアプリの版では、記録を他の人と共有できません」と答え、自分だけの記録で近い形を提案する。"},
}

// noShareChatRule replaces shareChatRule in builds without shared data.
const noShareChatRule = "- 共有の範囲: このアプリの版では、記録を他の人と共有できない(家族だけ・みんなに公開のどちらも)。頼まれたら「このアプリの版では、記録を他の人と共有できません」と答える。"

// dropSharingRules removes the shared-data rules (#451 G2): the marks and
// share-scope blocks, the chat share rule and the scope / poster / edit lines.
func dropSharingRules(s string) string {
	s = strings.Replace(s, "\n\n"+marksRules+"\n"+marksCatalog, "", 1)
	s = strings.Replace(s, shareRules, "", 1)
	s = strings.Replace(s, shareChatRule, noShareChatRule, 1)
	lines := strings.Split(s, "\n")
	kept := lines[:0]
	for _, ln := range lines {
		keep := true
		for _, rw := range sharingLineRewrites {
			if strings.HasPrefix(ln, rw.prefix) {
				if rw.with == "" {
					keep = false
				} else {
					ln = rw.with
				}
				break
			}
		}
		if keep {
			kept = append(kept, ln)
		}
	}
	return strings.Join(kept, "\n")
}

// exampleRe matches "、歩数 = healthStat" style examples in chatCapabilities.
var exampleRe = regexp.MustCompile(`、[^、()（）=]{1,24} = ([A-Za-z]+)`)

func buildRestrictedPrompt(system string, r Restriction) string {
	a := r.assets()
	s := strings.ReplaceAll(system, catalog, a.catalog)
	s = strings.ReplaceAll(s, chatCapabilityIndex, a.index)
	s = strings.ReplaceAll(s, string(generationSchema), string(a.genSch))
	if r.disallowed[string(dsl.CompWebView)] {
		s = noWebViewReplacer.Replace(s)
	}
	if r.disallowed[string(dsl.CompChatThread)] {
		s = strings.Replace(s, "\n\n"+chatThreadRules, "", 1) // #451 G2
	}
	if r.noShared {
		s = dropSharingRules(s)
	}

	// Rule lines that route an utterance to a disallowed part
	// (「歩数を見たい」は healthStat ...) are dropped.
	lines := strings.Split(s, "\n")
	kept := lines[:0]
	for _, ln := range lines {
		drop := false
		for t := range r.disallowed {
			if strings.Contains(ln, "」は "+t) {
				drop = true
				break
			}
		}
		if !drop {
			kept = append(kept, ln)
		}
	}
	s = strings.Join(kept, "\n")
	s = exampleRe.ReplaceAllStringFunc(s, func(m string) string {
		if r.disallowed[exampleRe.FindStringSubmatch(m)[1]] {
			return ""
		}
		return m
	})
	if r.noCamera {
		s = strings.ReplaceAll(s, "source camera", "source library")
		s = strings.ReplaceAll(s, "source both・内カメ/外カメは撮影画面で切り替えられる", "source library")
		s = strings.ReplaceAll(s, "source both", "source library")
	}
	return s + "\n\n" + restrictionNote(r)
}

// restrictionNote is appended to every restricted prompt; it overrides any
// rule or example above that still names a removed part.
func restrictionNote(r Restriction) string {
	var b strings.Builder
	b.WriteString("## この依頼の制限(このアプリの版では使えない機能。上の説明・例・ルールより優先する)\n")
	if d := r.Disallowed(); len(d) > 0 {
		fmt.Fprintf(&b, "- 使えない部品: %s。設計図に入れない・案や suggestions に出さない・「できます」と言わない(聞かれたら「このアプリの版では使えません」と答え、カタログにある部品で近い代わりを添える)。部品は上の部品カタログ/索引にあるものだけを使う。\n", strings.Join(d, ", "))
	}
	if r.noHealth {
		b.WriteString("- Field の health 属性は使わない(ヘルスケアへの書き込みはできない。値はアプリ内の記録として保存するだけ)。\n")
	}
	if r.disallowed[string(dsl.CompNearbyShare)] {
		b.WriteString(musicNoRequestsNote) // #412
	}
	if r.noShared {
		b.WriteString("- 記録はすべて自分だけ(private)。entity の scope / edit / marks、$author・$marks は使わない。「みんなで」「共有」「投稿」「ランキング」「掲示板」「チャット」のように他の人とデータを見る依頼は、このアプリの版ではできないと伝え、自分だけの記録で近い形を作る。\n")
	}
	if r.noCamera {
		b.WriteString("- photoField / videoField の source は library だけ(カメラで撮影はできない。写真・動画はライブラリから選ぶ)。camera / both は使わない。\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// --- post-generation enforcement ---

// countDisallowed counts disallowed components per type in a DSL document.
func countDisallowed(raw []byte, r Restriction) map[string]int {
	out := map[string]int{}
	if len(r.disallowed) == 0 || len(raw) == 0 {
		return out
	}
	var app dsl.App
	if err := json.Unmarshal(raw, &app); err != nil {
		return out
	}
	var walk func([]dsl.Component)
	walk = func(cs []dsl.Component) {
		for _, c := range cs {
			if r.disallowed[string(c.Type)] {
				out[string(c.Type)]++
			}
			walk(c.Children)
		}
	}
	for _, sc := range app.Screens {
		walk(sc.Components)
	}
	return out
}

// restrictionViolations lists disallowed components in raw that base (the
// current DSL of an edit; nil for create) does not already have: a turn may
// not add them. Existing ones are removed by enforceRestriction.
func restrictionViolations(raw, base []byte, r Restriction) []string {
	now := countDisallowed(raw, r)
	before := countDisallowed(base, r)
	var out []string
	for _, t := range sortedIntKeys(now) {
		if now[t] > before[t] {
			out = append(out, fmt.Sprintf("component type %q is not available in this app build: do not use it (use only the parts in the catalog)", t))
		}
	}
	if r.noShared {
		prev := map[string]bool{}
		for _, id := range sharedEntities(base) {
			prev[id] = true
		}
		for _, id := range sharedEntities(raw) {
			if !prev[id] {
				out = append(out, fmt.Sprintf("entity %q: shared / public records are not available in this app build: omit scope (keep it private) and do not use marks, edit, $author or $marks", id))
			}
		}
	}
	return out
}

// sharedEntities lists the ids of entities other users see (scope shared /
// public) in a DSL document (#451 G2).
func sharedEntities(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	var app dsl.App
	if err := json.Unmarshal(raw, &app); err != nil {
		return nil
	}
	var out []string
	for i := range app.Entities {
		if app.Entities[i].IsShared() {
			out = append(out, app.Entities[i].ID)
		}
	}
	return out
}

func sortedIntKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// enforceRestriction removes disallowed components (with their children),
// drops Field.health, turns camera sources into library and (sharedData,
// #451 G2) makes every entity private (scope / edit / marks / item.mark). It returns raw
// unchanged (changed=false) when nothing applied; notes say what was done.
func enforceRestriction(raw []byte, r Restriction) (out []byte, notes []string, changed bool) {
	if !r.Active() || len(raw) == 0 {
		return raw, nil, false
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var root map[string]any
	if err := dec.Decode(&root); err != nil {
		return raw, nil, false
	}
	removed := map[string]int{}
	var health, camera, private int
	var fixComps func([]any) []any
	fixComps = func(cs []any) []any {
		kept := make([]any, 0, len(cs))
		for _, v := range cs {
			c, ok := v.(map[string]any)
			if !ok {
				kept = append(kept, v)
				continue
			}
			t, _ := c["type"].(string)
			if r.disallowed[t] {
				removed[t]++
				continue
			}
			if r.noCamera && (t == string(dsl.CompPhotoField) || t == string(dsl.CompVideoField)) {
				if src, _ := c["source"].(string); src == "camera" || src == "both" {
					c["source"] = "library"
					camera++
				}
			}
			if r.noShared {
				if item, _ := c["item"].(map[string]any); item != nil {
					delete(item, "mark") // marks go with the scope below
				}
			}
			if ch, ok := c["children"].([]any); ok {
				c["children"] = fixComps(ch)
			}
			kept = append(kept, c)
		}
		return kept
	}
	if screens, ok := root["screens"].([]any); ok {
		for _, s := range screens {
			sc, _ := s.(map[string]any)
			if cs, ok := sc["components"].([]any); ok {
				sc["components"] = fixComps(cs)
			}
		}
	}
	if r.noHealth {
		if ents, ok := root["entities"].([]any); ok {
			for _, e := range ents {
				em, _ := e.(map[string]any)
				fs, _ := em["fields"].([]any)
				for _, f := range fs {
					if fm, ok := f.(map[string]any); ok {
						if _, has := fm["health"]; has {
							delete(fm, "health")
							health++
						}
					}
				}
			}
		}
	}
	if r.noShared {
		if ents, ok := root["entities"].([]any); ok {
			for _, e := range ents {
				em, _ := e.(map[string]any)
				if em == nil {
					continue
				}
				_, hasEdit := em["edit"]
				_, hasMarks := em["marks"]
				scope, _ := em["scope"].(string)
				if (scope != "" && scope != string(dsl.ScopePrivate)) || hasEdit || hasMarks {
					delete(em, "scope")
					delete(em, "edit")
					delete(em, "marks")
					private++
				}
			}
		}
	}
	if len(removed) == 0 && health == 0 && camera == 0 && private == 0 {
		return raw, nil, false
	}
	for _, t := range sortedIntKeys(removed) {
		notes = append(notes, fmt.Sprintf("removed %d %s", removed[t], t))
	}
	if health > 0 {
		notes = append(notes, fmt.Sprintf("dropped health on %d field(s)", health))
	}
	if camera > 0 {
		notes = append(notes, fmt.Sprintf("camera source -> library on %d component(s)", camera))
	}
	if private > 0 {
		notes = append(notes, fmt.Sprintf("made %d entities private", private))
	}
	b, err := json.Marshal(root)
	if err != nil {
		return raw, nil, false
	}
	return b, notes, true
}
