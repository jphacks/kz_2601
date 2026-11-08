package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
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

// FeatureRecordRetention (#482) is a supportedFeatures value: the iOS build
// purges records past Entity.retentionDays. A client that sends
// allowedComponents (v1.0 and later) without it ignores retentionDays, so the
// AI must not promise auto-delete there.
const FeatureRecordRetention = "recordRetention"

// FeatureNotificationWhen (#265) is a supportedFeatures value: the iOS build
// evaluates app.notifications[].when. Older builds read a when notification
// as a plain repeat once (a 09:00 reminder), so it is never generated for them.
const FeatureNotificationWhen = "notification.when"

// FeatureSingleEntity (#267) is a supportedFeatures value: the iOS build edits
// the one record of an Entity.single and evaluates {"setting": ...}. Older
// builds add a record each time the settings form is saved and cannot read a
// computed that uses setting.
const FeatureSingleEntity = "singleEntity"

// Restriction is the per-request set of unusable parts. The zero value means
// no restriction.
type Restriction struct {
	disallowed map[string]bool // component types the client cannot render
	noHealth   bool
	noCamera   bool
	noShared   bool
	// noRetention (#482): the client cannot purge (Entity.retentionDays)
	noRetention bool
	// noWhen (#265): the client cannot evaluate Notification.when
	noWhen bool
	// noMatching (#507): the client has no matching UI (Entity.match)
	noMatching bool
	// noSingle (#267): the client has no Entity.single / {"setting": ...}
	noSingle bool
	// noRoles (#272): the client has no member picker / visibleTo (data_roles_rules.go)
	noRoles bool
	// noAddressed (#271): the client has no members picker (messages_addressed_rules.go)
	noAddressed bool
	// noNear: the client has no Notification.near / jitterMinutes (notify_near_rules.go)
	noNear bool
	key         string // "" = no restriction; memo key otherwise
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
			r.noMatching = true // match needs a shared entity
			r.noRoles = true    // roles are about other people's records
			r.noAddressed = true
		}
	}
	r.withholdMatchingUI()
	r.setKey()
	return r
}

// NewClientRestriction is NewRestriction plus the client's supportedFeatures
// (#482). Only a client that sends allowedComponents is judged by them: one
// that sends neither (dev builds, ai-eval, smoke) stays unrestricted.
func NewClientRestriction(allowed, disabled, supported []string) Restriction {
	r := NewRestriction(allowed, disabled)
	if len(allowed) > 0 && !slices.Contains(supported, FeatureRecordRetention) {
		r.noRetention = true
	}
	if len(allowed) > 0 && !slices.Contains(supported, FeatureNotificationWhen) {
		r.noWhen = true
	}
	if len(allowed) > 0 && !slices.Contains(supported, FeatureMatching) {
		r.noMatching = true
	}
	if len(allowed) > 0 && !slices.Contains(supported, FeatureSingleEntity) {
		r.noSingle = true
	}
	if len(allowed) > 0 && !slices.Contains(supported, FeatureDataRoles) {
		r.noRoles = true
	}
	if len(allowed) > 0 && !slices.Contains(supported, FeatureAddressed) {
		r.noAddressed = true
	}
	if len(allowed) > 0 && !slices.Contains(supported, FeatureNotificationNear) {
		r.noNear = true
	}
	r.withholdMatchingUI()
	r.setKey()
	return r
}

// withholdMatchingUI: without matching there is nothing for matchList to
// show (#507 phase 2), even when the client lists the type.
func (r *Restriction) withholdMatchingUI() {
	if !r.noMatching {
		return
	}
	if r.disallowed == nil {
		r.disallowed = map[string]bool{}
	}
	r.disallowed[string(dsl.CompMatchList)] = true
}

func (r *Restriction) setKey() {
	r.key = ""
	if len(r.disallowed) > 0 || r.noHealth || r.noCamera || r.noShared || r.noRetention || r.noWhen || r.noMatching || r.noSingle || r.noRoles || r.noAddressed || r.noNear {
		r.key = strings.Join(r.Disallowed(), ",") + fmt.Sprintf("|h=%t|c=%t|s=%t|r=%t|w=%t", r.noHealth, r.noCamera, r.noShared, r.noRetention, r.noWhen) +
			fmt.Sprintf("|m=%t|1=%t|a=%t|to=%t|n=%t", r.noMatching, r.noSingle, r.noRoles, r.noAddressed, r.noNear)
	}
}

// NoSingle reports whether Entity.single / {"setting": ...} is unavailable (#267).
func (r Restriction) NoSingle() bool { return r.noSingle }

// NoMatching reports whether Entity.match is unavailable (#507).
func (r Restriction) NoMatching() bool { return r.noMatching }

// NoRetention reports whether Entity.retentionDays is unavailable (#482).
func (r Restriction) NoRetention() bool { return r.noRetention }

// NoWhen reports whether Notification.when is unavailable (#265).
func (r Restriction) NoWhen() bool { return r.noWhen }

// NoNear reports whether Notification.near / jitterMinutes are unavailable.
func (r Restriction) NoNear() bool { return r.noNear }

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
	if r.noRetention {
		if entity, _ := defs["Entity"].(map[string]any); entity != nil {
			props, _ := entity["properties"].(map[string]any)
			delete(props, "retentionDays")
		}
	}
	if r.noWhen {
		if n, _ := defs["Notification"].(map[string]any); n != nil {
			props, _ := n["properties"].(map[string]any)
			delete(props, "when")
		}
	}
	if r.noNear {
		restrictNearSchema(defs)
	}
	if r.noMatching {
		if entity, _ := defs["Entity"].(map[string]any); entity != nil {
			props, _ := entity["properties"].(map[string]any)
			delete(props, "match")
		}
		if item, _ := defs["ListItem"].(map[string]any); item != nil {
			props, _ := item["properties"].(map[string]any)
			delete(props, "interest")
		}
	}
	if r.noSingle {
		restrictSingle(defs)
	}
	if r.noRoles {
		restrictDataRoles(defs, r.noShared)
	}
	if r.noAddressed {
		restrictAddressed(defs)
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
	s = strings.Replace(s, "\n\n"+followRules, "", 1) // #270
	s = strings.Replace(s, "\n\n"+followChatRule, "", 1)
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
	if r.noRetention {
		s = strings.Replace(s, "\n\n"+retentionRules, "", 1)
		s = strings.Replace(s, retentionChatRule, noRetentionChatRule, 1)
	}
	if r.noWhen {
		s = strings.Replace(s, "\n\n"+notifyWhenRules, "", 1)
		s = strings.Replace(s, condNotifyCanLine, condNotifyCannotLine, 1)
		s = strings.Replace(s, notifyWhenChatRule, noNotifyWhenChatRule, 1)
	}
	if r.noNear {
		s = dropNearRules(s)
	}
	if r.noMatching {
		s = strings.Replace(s, "\n\n"+matchingRules, "", 1)
		s = strings.Replace(s, matchingChatRule, noMatchingChatRule, 1)
	}
	if r.noSingle {
		s = strings.Replace(s, "\n\n"+singleRules, "", 1)
		s = strings.Replace(s, settingsSingleLine, settingsLatestLine, 1)
		s = strings.Replace(s, notifySettingBudget, notifyLatestBudget, 1)
		s = strings.Replace(s, singleChatRule, noSingleChatRule, 1)
		s = strings.Replace(s, titleGoalSingleLine, titleGoalLatestLine, 1)
	}
	if r.noRoles {
		s = dropDataRolesRules(s, r.noShared)
	}
	if r.noAddressed {
		s = dropAddressedRules(s)
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
	if r.noRetention {
		b.WriteString("- 記録の自動削除(entity の retentionDays)は使わない(この版では期間を過ぎた記録を消せない)。頼まれたら「この版では記録の自動削除はできません。最新版に更新すると使えます」と伝え、ほかの変更だけ行う。\n")
	}
	if r.noWhen {
		b.WriteString("- 条件で知らせる通知(app.notifications の when)は使わない(この版では条件を確かめられない)。在庫が減ったら・予算を超えたら等と頼まれたら、不足の一覧などで代替し「条件で知らせる通知はこの版ではできないため、○○で代替しました。」と申告する。\n")
	}
	if r.noNear {
		b.WriteString(noNearNote)
	}
	if r.noMatching && !r.noShared {
		b.WriteString("- entity の match は使わない(この版ではお互いに承認してつながる仕組みが無い)。仲間探しを頼まれたら「この版ではお互いに承認してつながる仕組みは使えません。最新版に更新すると使えます」と伝え、みんなで見るプロフィールの一覧(連絡先・写真・位置は載せない)で近い形を作る。\n")
	}
	if r.noSingle {
		b.WriteString("- entity の single と式 {\"setting\":...} は使わない(この版では設定の 1 件を編集する画面が無い)。周期・目標・予算のような値は記録の entity に入れ、最新の値を {\"agg\":\"latest\",...} で読む。\n")
	}
	if r.noRoles {
		b.WriteString(dataRolesNote(r.noShared))
	}
	if r.noAddressed && !r.noShared {
		b.WriteString(addressedNote)
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
	if r.noRetention {
		prev := map[string]bool{}
		for _, id := range retentionEntities(base) {
			prev[id] = true
		}
		for _, id := range retentionEntities(raw) {
			if !prev[id] {
				out = append(out, fmt.Sprintf("entity %q: retentionDays (auto-delete) is not available in this app build: omit it", id))
			}
		}
	}
	if r.noWhen {
		prev := map[string]bool{}
		for _, id := range whenNotifications(base) {
			prev[id] = true
		}
		for _, id := range whenNotifications(raw) {
			if !prev[id] {
				out = append(out, fmt.Sprintf("app.notifications %q: when (condition notification) is not available in this app build: remove the notification and substitute (e.g. a shortage list)", id))
			}
		}
	}
	if r.noNear {
		out = append(out, nearViolations(raw, base)...)
	}
	if r.noSingle {
		prev := map[string]bool{}
		for _, id := range singleEntities(base) {
			prev[id] = true
		}
		for _, id := range singleEntities(raw) {
			if !prev[id] {
				out = append(out, fmt.Sprintf(`entity %q: single (one record of settings) is not available in this app build: omit it, keep the values as records and read the newest with {"agg":"latest","entity":%q,"field":...} instead of {"setting":...}`, id, id))
			}
		}
	}
	if r.noMatching {
		prev := map[string]bool{}
		for _, id := range matchEntities(base) {
			prev[id] = true
		}
		for _, id := range matchEntities(raw) {
			if !prev[id] {
				out = append(out, fmt.Sprintf("entity %q: match (mutual-approval matching) is not available in this app build: omit it", id))
			}
		}
	}
	return out
}

// whenNotifications lists the ids of app.notifications with when (#265).
func whenNotifications(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	var app dsl.App
	if err := json.Unmarshal(raw, &app); err != nil {
		return nil
	}
	var out []string
	for _, n := range app.App.Notifications {
		if n.When != nil {
			out = append(out, n.ID)
		}
	}
	return out
}

// matchEntities lists entities with match in a DSL document (#507).
func matchEntities(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	var app dsl.App
	if err := json.Unmarshal(raw, &app); err != nil {
		return nil
	}
	return app.MatchEntities()
}

// retentionEntities lists entities with retentionDays in a DSL document (#482).
func retentionEntities(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	var app dsl.App
	if err := json.Unmarshal(raw, &app); err != nil {
		return nil
	}
	return app.RetentionEntities()
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
	var health, camera, private, retention, when, matching int
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
			if r.noMatching {
				if item, _ := c["item"].(map[string]any); item != nil {
					if _, has := item["interest"]; has {
						delete(item, "interest") // goes with match below (#507)
						matching++
					}
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
	if r.noRetention {
		if ents, ok := root["entities"].([]any); ok {
			for _, e := range ents {
				if em, _ := e.(map[string]any); em != nil {
					if _, has := em["retentionDays"]; has {
						delete(em, "retentionDays")
						retention++
					}
				}
			}
		}
	}
	if r.noWhen {
		// the whole notification goes: without when an old build reads it as a 09:00 reminder
		if app, _ := root["app"].(map[string]any); app != nil {
			if ns, ok := app["notifications"].([]any); ok {
				kept := make([]any, 0, len(ns))
				for _, v := range ns {
					if n, _ := v.(map[string]any); n != nil && n["when"] != nil {
						when++
						continue
					}
					kept = append(kept, v)
				}
				app["notifications"] = kept
			}
		}
	}
	if r.noMatching {
		if ents, ok := root["entities"].([]any); ok {
			for _, e := range ents {
				if em, _ := e.(map[string]any); em != nil {
					if _, has := em["match"]; has {
						delete(em, "match")
						matching++
					}
				}
			}
		}
	}
	single := 0
	if r.noSingle {
		single = unsingle(root)
	}
	roles := 0
	if r.noRoles {
		roles = unrole(root, r.noShared)
	}
	addressed := 0
	if r.noAddressed || r.noRoles {
		addressed = unaddress(root, r.noAddressed)
	}
	near := 0
	if r.noNear {
		near = unnear(root)
	}
	if len(removed) == 0 && health == 0 && camera == 0 && private == 0 && retention == 0 && when == 0 && matching == 0 && single == 0 && roles == 0 && addressed == 0 && near == 0 {
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
	if retention > 0 {
		notes = append(notes, fmt.Sprintf("dropped retentionDays on %d entity(ies)", retention))
	}
	if when > 0 {
		notes = append(notes, fmt.Sprintf("dropped %d when notification(s)", when))
	}
	if matching > 0 {
		notes = append(notes, fmt.Sprintf("dropped match / item.interest on %d place(s)", matching))
	}
	if single > 0 {
		notes = append(notes, fmt.Sprintf("single -> latest records on %d entity(ies)", single))
	}
	if roles > 0 {
		notes = append(notes, fmt.Sprintf("dropped member / visibleTo / admin rules on %d place(s)", roles))
	}
	if addressed > 0 {
		notes = append(notes, fmt.Sprintf("dropped addressedTo / members on %d place(s)", addressed))
	}
	if near > 0 {
		notes = append(notes, fmt.Sprintf("dropped near notification / jitterMinutes on %d place(s)", near))
	}
	b, err := json.Marshal(root)
	if err != nil {
		return raw, nil, false
	}
	return b, notes, true
}
