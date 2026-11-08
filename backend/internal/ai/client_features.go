package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// supportedFeatures values of DSL parts added after notification.near. A
// client that sends allowedComponents without one of them does not
// implement it, so the AI does not use it there (prompt rules, schema keys,
// new uses are retried, the rest is removed by enforceRestriction).
const (
	FeatureAILong       = "aiButton.long"       // aiButton length long (ai_long_rules.go)
	FeatureAIInputPhoto = "aiButton.inputPhoto" // aiButton inputPhoto (ai_photo_input_rules.go)
	FeatureAISaveTo     = "aiButton.saveTo"     // aiButton saveTo (ai_save_to_rules.go)
	FeatureAIInputFeeds = "aiButton.inputFeeds" // aiButton inputFeeds (ai_input_feed_rules.go)
	FeatureLookupBook   = "action.lookupBook"   // action lookupBook (book_rules.go)
	// FeatureTextHighlight covers text.highlight and text {{fieldId}} on
	// form / detail screens (text_highlight_rules.go): they shipped together.
	FeatureTextHighlight = "text.highlight"
	FeatureFeedV2        = "feed.v2" // feed providers warnings / pressure (feed_warnings_pressure_rules.go)
	// FeatureAIChatInput covers aiChat inputEntity and its limit
	// (ai_chat_rules_input.go): a build without it ignores both silently.
	FeatureAIChatInput = "aiChat.inputEntity"
)

// clientFeature is one optional DSL part judged by supportedFeatures.
type clientFeature struct {
	key     string
	rules   []string    // blocks appended to a prompt with "\n\n"; dropped
	rewrite [][2]string // other rule text that names the part
	note    string      // restriction note line
	schema  func(defs map[string]any)
	// places lists where a DSL document uses the part (one string per use).
	places func(root map[string]any) []string
	// strip removes the part from a document; it returns the places changed.
	strip func(root map[string]any) int
	// violation is the retry message for a new use at place.
	violation string
}

var clientFeatures = []clientFeature{
	{
		key:   FeatureAILong,
		rules: []string{aiLongRules},
		rewrite: [][2]string{
			{"長い説明は length: long も付けてよい。", ""},
		},
		note:   "- aiButton の length long(長い文章)は使わない(この版では動かない)。答えは短い文(既定の short)にし、長い文章を頼まれたら「長い文章はこの版ではできないため、短くまとめる形にしました。」と申告する。\n",
		schema: func(defs map[string]any) { deleteProps(defs, "AiButton", "length") },
		places: func(root map[string]any) []string {
			return componentPlaces(root, func(c map[string]any) bool { return isType(c, dsl.CompAiButton) && c["length"] == "long" })
		},
		strip: func(root map[string]any) int {
			return editComponents(root, func(c map[string]any) bool { return isType(c, dsl.CompAiButton) && deleteKey(c, "length") })
		},
		violation: "aiButton length long is not available in this app build: omit length",
	},
	{
		key:   FeatureAIInputPhoto,
		rules: []string{aiPhotoInputRules},
		rewrite: [][2]string{
			{"写真を AI に見せて説明・相談するなら aiButton の inputPhoto(下の節)。", ""},
			{"までと、inputPhoto の写真 1 枚。", "まで。写真は AI に見せない。"},
		},
		note:   "- aiButton の inputPhoto(写真を AI に見せる)は使わない(今は使えない)。写真の文字・金額・日付・分類を読むなら photoField の analyze(端末内)を使い、説明・要約は文字の field を材料にする。頼まれたら「写真を AI に見せる機能は今は使えないため、○○で代替しました。」と申告する。\n",
		schema: func(defs map[string]any) { deleteProps(defs, "AiButton", "inputPhoto") },
		places: func(root map[string]any) []string {
			return componentPlaces(root, func(c map[string]any) bool { return isType(c, dsl.CompAiButton) && c["inputPhoto"] != nil })
		},
		strip: func(root map[string]any) int {
			return editComponents(root, func(c map[string]any) bool { return isType(c, dsl.CompAiButton) && deleteKey(c, "inputPhoto") })
		},
		violation: "aiButton inputPhoto is not available: omit inputPhoto (use photoField analyze to read a photo on the device)",
	},
	{
		key:    FeatureAISaveTo,
		rules:  []string{aiSaveToRules},
		note:   "- aiButton の saveTo(AI が作った項目を 1 件ずつ記録に足す)は使わない(この版では動かない)。項目はボタンの下に出すだけにし、「1 件ずつ記録に足すのはこの版ではできないため、一覧で出す形にしました。」と申告する。\n",
		schema: func(defs map[string]any) { deleteProps(defs, "AiButton", "saveTo") },
		places: func(root map[string]any) []string {
			return componentPlaces(root, func(c map[string]any) bool { return isType(c, dsl.CompAiButton) && c["saveTo"] != nil })
		},
		strip: func(root map[string]any) int {
			return editComponents(root, func(c map[string]any) bool { return isType(c, dsl.CompAiButton) && deleteKey(c, "saveTo") })
		},
		violation: "aiButton saveTo is not available in this app build: omit saveTo (the items show under the button)",
	},
	{
		key:    FeatureAIInputFeeds,
		rules:  []string{aiInputFeedRules},
		note:   "- aiButton の inputFeeds(画面の feed の行を AI に読ませる)は使わない(この版では動かない)。feed は表示だけにし、AI のボタンは prompt と記録を材料にする。頼まれたら「feed の内容を AI に読ませるのはこの版ではできないため、○○で代替しました。」と申告する。\n",
		schema: func(defs map[string]any) { deleteProps(defs, "AiButton", "inputFeeds") },
		places: func(root map[string]any) []string {
			return componentPlaces(root, func(c map[string]any) bool { return isType(c, dsl.CompAiButton) && c["inputFeeds"] != nil })
		},
		strip: func(root map[string]any) int {
			return editComponents(root, func(c map[string]any) bool { return isType(c, dsl.CompAiButton) && deleteKey(c, "inputFeeds") })
		},
		violation: "aiButton inputFeeds is not available in this app build: omit inputFeeds (the feed still shows on the screen)",
	},
	{
		key:   FeatureLookupBook,
		rules: []string{bookRules},
		note:  "- ボタンの lookupBook(ISBN から本の情報を入れる)は使わない(今は使えない)。書名・著者は手で入力する形にし、頼まれたら「ISBN から本の情報を入れる機能は今は使えないため、手入力の形にしました。」と申告する。\n",
		schema: func(defs map[string]any) {
			dropEnumValues(defs, "Action", "type", string(dsl.ActionLookupBook))
			deleteProps(defs, "Action", "bookLookup")
		},
		places: func(root map[string]any) []string {
			return append(componentPlaces(root, hasLookupBook), toolbarPlaces(root, hasLookupBook)...)
		},
		strip: func(root map[string]any) int {
			return dropComponents(root, hasLookupBook) + dropToolbar(root, hasLookupBook)
		},
		violation: "action lookupBook is not available: remove the button and let the user type the book details",
	},
	{
		key:   FeatureTextHighlight,
		rules: []string{textHighlightRules},
		note:  "- text の highlight と text の {{fieldId}}(開いているレコードの値)は使わない(この版では動かない)。text は固定の文字だけにし、レコードの値は textField や list で見せる。\n",
		schema: func(defs map[string]any) {
			deleteProps(defs, "Text", "highlight")
			if t, _ := defs["Text"].(map[string]any); t != nil {
				for _, k := range []string{"description", keyUsage} {
					if s, ok := t[k].(string); ok {
						t[k] = dropSentences(s, "{{", "highlight")
					}
				}
			}
		},
		places: func(root map[string]any) []string {
			return componentPlaces(root, func(c map[string]any) bool {
				if !isType(c, dsl.CompText) {
					return false
				}
				s, _ := c["text"].(string)
				return c["highlight"] != nil || textFieldRefRe.MatchString(s)
			})
		},
		strip:     stripTextHighlight,
		violation: "text highlight and {{fieldId}} in text are not available in this app build: use fixed text (show record values with textField / list)",
	},
	{
		key:   FeatureFeedV2,
		rules: []string{feedWarningsPressureRules},
		note:  "- feed の provider warnings(警報・注意報)と pressure(気圧の予報)は使わない(この版では動かない)。天気予報(weather)で近い形を作り「警報・気圧の予報はこの版ではできないため、天気予報で代替しました。」と申告する。\n",
		schema: func(defs map[string]any) {
			dropEnumValues(defs, "Feed", "provider", dsl.FeedWarnings, dsl.FeedPressure)
			if f, _ := defs["Feed"].(map[string]any); f != nil {
				props, _ := f["properties"].(map[string]any)
				if p, _ := props["provider"].(map[string]any); p != nil {
					if s, ok := p["description"].(string); ok {
						p["description"] = strings.Replace(dropSentences(s, "warnings", "pressure"), "この 3 つは region が要る", "weather は region が要る", 1)
					}
				}
			}
		},
		places:    func(root map[string]any) []string { return componentPlaces(root, isFeedV2) },
		strip:     func(root map[string]any) int { return dropComponents(root, isFeedV2) },
		violation: "feed provider warnings / pressure is not available in this app build: use provider weather or remove the feed",
	},
	{
		key:    FeatureAIChatInput,
		rules:  []string{aiChatInputRules},
		note:   "- aiChat の inputEntity(自分の記録を読んで話す)と limit は使わない(この版では動かない)。aiChat は会話だけにし、頼まれたら「記録を AI に読ませて話すのはこの版ではできないため、会話だけの形にしました。」と申告する。\n",
		schema: func(defs map[string]any) { deleteProps(defs, "AiChat", "inputEntity", "limit") },
		places: func(root map[string]any) []string {
			return componentPlaces(root, func(c map[string]any) bool {
				return isType(c, dsl.CompAiChat) && (c["inputEntity"] != nil || c["limit"] != nil)
			})
		},
		strip: func(root map[string]any) int {
			return editComponents(root, func(c map[string]any) bool {
				if !isType(c, dsl.CompAiChat) {
					return false
				}
				a, b := deleteKey(c, "inputEntity"), deleteKey(c, "limit")
				return a || b
			})
		},
		violation: "aiChat inputEntity is not available in this app build: omit inputEntity and limit (the chat still works without records)",
	},
}

// offFeatures lists the clientFeatures r turns off, in table order.
func (r Restriction) offFeatures() []*clientFeature {
	var out []*clientFeature
	for i := range clientFeatures {
		if r.off[clientFeatures[i].key] {
			out = append(out, &clientFeatures[i])
		}
	}
	return out
}

// NoFeature reports whether a clientFeature (FeatureAILong, ...) is off.
func (r Restriction) NoFeature(key string) bool { return r.off[key] }

// withFeatureOff turns one clientFeature off (the server's own switches,
// e.g. AI_TEXT_IMAGE for inputPhoto).
func (r Restriction) withFeatureOff(key string) Restriction {
	off := map[string]bool{key: true}
	for k := range r.off {
		off[k] = true
	}
	r.off = off
	r.setKey()
	return r
}

// clientFeaturesOff: the clientFeatures a client that sends allowedComponents
// does not list in supported.
func clientFeaturesOff(allowed, supported []string) map[string]bool {
	if len(allowed) == 0 {
		return nil
	}
	var off map[string]bool
	for _, f := range clientFeatures {
		if !slices.Contains(supported, f.key) {
			if off == nil {
				off = map[string]bool{}
			}
			off[f.key] = true
		}
	}
	return off
}

func (r Restriction) offKey() string {
	keys := make([]string, 0, len(r.off))
	for k := range r.off {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ",")
}

// clientRestriction is the request's restriction plus the server switches:
// with AI_TEXT_IMAGE off, inputPhoto would answer 403; with the book lookup
// off on appconfig (openBD terms, BOOK_LOOKUP_ENABLED), lookupBook would
// answer 503. Neither is generated then.
func (s *Service) clientRestriction(allowed, disabled, supported []string) Restriction {
	r := NewClientRestriction(allowed, disabled, supported)
	if !s.TextImage {
		r = r.withFeatureOff(FeatureAIInputPhoto)
	}
	if s.NoBookLookup {
		r = r.withFeatureOff(FeatureLookupBook)
	}
	return r
}

func dropFeatureRules(s string, f *clientFeature) string {
	for _, b := range f.rules {
		s = strings.Replace(s, "\n\n"+b, "", 1)
	}
	for _, rw := range f.rewrite {
		s = strings.Replace(s, rw[0], rw[1], 1)
	}
	return s
}

func featureViolations(raw, base []byte, r Restriction) []string {
	var out []string
	if len(r.off) == 0 {
		return nil
	}
	now, before := decodeDoc(raw), decodeDoc(base)
	for _, f := range r.offFeatures() {
		prev := map[string]int{}
		for _, p := range f.places(before) {
			prev[p]++
		}
		for _, p := range f.places(now) {
			if prev[p] > 0 {
				prev[p]--
				continue
			}
			out = append(out, fmt.Sprintf("%s: %s", p, f.violation))
		}
	}
	return out
}

// --- DSL document helpers (generic JSON, as enforceRestriction works) ---

var textFieldRefRe = regexp.MustCompile(`\{\{\s*[A-Za-z0-9_-]+\s*\}\}`)

func decodeDoc(raw []byte) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var root map[string]any
	if err := dec.Decode(&root); err != nil {
		return nil
	}
	return root
}

func isType(c map[string]any, t dsl.ComponentType) bool {
	s, _ := c["type"].(string)
	return s == string(t)
}

func deleteKey(m map[string]any, k string) bool {
	if _, ok := m[k]; !ok {
		return false
	}
	delete(m, k)
	return true
}

func hasLookupBook(c map[string]any) bool {
	a, _ := c["action"].(map[string]any)
	t, _ := a["type"].(string)
	return t == string(dsl.ActionLookupBook)
}

func isFeedV2(c map[string]any) bool {
	p, _ := c["provider"].(string)
	return isType(c, dsl.CompFeed) && (p == dsl.FeedWarnings || p == dsl.FeedPressure)
}

// componentPlaces names the components match picks ("screen <id> <type>").
func componentPlaces(root map[string]any, match func(map[string]any) bool) []string {
	var out []string
	forScreens(root, func(id string, sc map[string]any) {
		var walk func([]any)
		walk = func(cs []any) {
			for _, v := range cs {
				c, _ := v.(map[string]any)
				if c == nil {
					continue
				}
				if match(c) {
					t, _ := c["type"].(string)
					out = append(out, fmt.Sprintf("screen %q %s", id, t))
				}
				if ch, ok := c["children"].([]any); ok {
					walk(ch)
				}
			}
		}
		cs, _ := sc["components"].([]any)
		walk(cs)
	})
	return out
}

func toolbarPlaces(root map[string]any, match func(map[string]any) bool) []string {
	var out []string
	forScreens(root, func(id string, sc map[string]any) {
		tb, _ := sc["toolbar"].([]any)
		for _, v := range tb {
			if b, _ := v.(map[string]any); b != nil && match(b) {
				out = append(out, fmt.Sprintf("screen %q toolbar", id))
			}
		}
	})
	return out
}

func forScreens(root map[string]any, fn func(id string, sc map[string]any)) {
	screens, _ := root["screens"].([]any)
	for _, s := range screens {
		if sc, _ := s.(map[string]any); sc != nil {
			id, _ := sc["id"].(string)
			fn(id, sc)
		}
	}
}

// editComponents runs edit on every component; it returns how many changed.
func editComponents(root map[string]any, edit func(map[string]any) bool) int {
	n := 0
	forScreens(root, func(_ string, sc map[string]any) {
		var walk func([]any)
		walk = func(cs []any) {
			for _, v := range cs {
				c, _ := v.(map[string]any)
				if c == nil {
					continue
				}
				if edit(c) {
					n++
				}
				if ch, ok := c["children"].([]any); ok {
					walk(ch)
				}
			}
		}
		cs, _ := sc["components"].([]any)
		walk(cs)
	})
	return n
}

// dropComponents removes the components drop picks (with their children).
func dropComponents(root map[string]any, drop func(map[string]any) bool) int {
	n := 0
	var fix func([]any) []any
	fix = func(cs []any) []any {
		kept := make([]any, 0, len(cs))
		for _, v := range cs {
			c, _ := v.(map[string]any)
			if c != nil && drop(c) {
				n++
				continue
			}
			if c != nil {
				if ch, ok := c["children"].([]any); ok {
					c["children"] = fix(ch)
				}
			}
			kept = append(kept, v)
		}
		return kept
	}
	forScreens(root, func(_ string, sc map[string]any) {
		if cs, ok := sc["components"].([]any); ok {
			sc["components"] = fix(cs)
		}
	})
	return n
}

func dropToolbar(root map[string]any, drop func(map[string]any) bool) int {
	n := 0
	forScreens(root, func(_ string, sc map[string]any) {
		tb, ok := sc["toolbar"].([]any)
		if !ok {
			return
		}
		kept := make([]any, 0, len(tb))
		for _, v := range tb {
			if b, _ := v.(map[string]any); b != nil && drop(b) {
				n++
				continue
			}
			kept = append(kept, v)
		}
		sc["toolbar"] = kept
	})
	return n
}

// screenFeedIDs returns the feed ids on each screen (keyed by screen id),
// children included.
func screenFeedIDs(root map[string]any) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	forScreens(root, func(sid string, sc map[string]any) {
		feeds := out[sid]
		if feeds == nil {
			feeds = map[string]bool{}
			out[sid] = feeds
		}
		eachScreenComponent(sc, func(c map[string]any) {
			if id, _ := c["id"].(string); id != "" && isType(c, dsl.CompFeed) {
				feeds[id] = true
			}
		})
	})
	return out
}

// removedFeedIDs returns, per screen, the feed ids in before that are gone
// from root (feeds the restriction removed).
func removedFeedIDs(before map[string]map[string]bool, root map[string]any) map[string]map[string]bool {
	after := screenFeedIDs(root)
	out := map[string]map[string]bool{}
	for sid, ids := range before {
		for id := range ids {
			if !after[sid][id] {
				if out[sid] == nil {
					out[sid] = map[string]bool{}
				}
				out[sid][id] = true
			}
		}
	}
	return out
}

func eachScreenComponent(sc map[string]any, fn func(map[string]any)) {
	cs, _ := sc["components"].([]any)
	for _, v := range cs {
		c, _ := v.(map[string]any)
		if c == nil {
			continue
		}
		fn(c)
		ch, _ := c["children"].([]any)
		for _, w := range ch {
			if x, _ := w.(map[string]any); x != nil {
				fn(x)
			}
		}
	}
}

// pruneInputFeeds drops the inputFeeds ids that name a feed the restriction
// removed (removed, per screen id), and inputFeeds left empty, so the
// enforced document stays valid. Other unknown ids are kept for validation
// to report. It returns the buttons changed.
func pruneInputFeeds(root map[string]any, removed map[string]map[string]bool) int {
	n := 0
	forScreens(root, func(sid string, sc map[string]any) {
		gone := removed[sid]
		if len(gone) == 0 {
			return
		}
		eachScreenComponent(sc, func(c map[string]any) {
			ids, ok := c["inputFeeds"].([]any)
			if !ok || !isType(c, dsl.CompAiButton) {
				return
			}
			kept := []any{}
			for _, id := range ids {
				if s, _ := id.(string); !gone[s] {
					kept = append(kept, id)
				}
			}
			if len(kept) == len(ids) {
				return
			}
			n++
			if len(kept) == 0 {
				delete(c, "inputFeeds")
			} else {
				c["inputFeeds"] = kept
			}
		})
	})
	return n
}

// stripTextHighlight drops text.highlight and the {{fieldId}} of texts; a
// text left empty goes.
func stripTextHighlight(root map[string]any) int {
	n := editComponents(root, func(c map[string]any) bool {
		if !isType(c, dsl.CompText) {
			return false
		}
		changed := deleteKey(c, "highlight")
		if s, _ := c["text"].(string); textFieldRefRe.MatchString(s) {
			c["text"] = strings.TrimSpace(textFieldRefRe.ReplaceAllString(s, ""))
			changed = true
		}
		return changed
	})
	dropComponents(root, func(c map[string]any) bool {
		s, ok := c["text"].(string)
		return isType(c, dsl.CompText) && ok && s == ""
	})
	return n
}

// --- schema helpers ---

func deleteProps(defs map[string]any, def string, keys ...string) {
	d, _ := defs[def].(map[string]any)
	props, _ := d["properties"].(map[string]any)
	for _, k := range keys {
		delete(props, k)
	}
}

func dropEnumValues(defs map[string]any, def, prop string, values ...string) {
	d, _ := defs[def].(map[string]any)
	props, _ := d["properties"].(map[string]any)
	p, _ := props[prop].(map[string]any)
	if p == nil {
		return
	}
	if en, ok := p["enum"].([]any); ok {
		kept := []any{}
		for _, v := range en {
			if !slices.Contains(values, fmt.Sprint(v)) {
				kept = append(kept, v)
			}
		}
		p["enum"] = kept
	}
	if m, ok := p[keyEnumDescs].(map[string]any); ok {
		for _, v := range values {
			delete(m, v)
		}
	}
}

// dropSentences removes the 。-ended sentences of s that contain any word.
func dropSentences(s string, words ...string) string {
	parts := strings.SplitAfter(s, "。")
	kept := parts[:0]
	for _, p := range parts {
		drop := false
		for _, w := range words {
			if strings.Contains(p, w) {
				drop = true
				break
			}
		}
		if !drop {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, "")
}
