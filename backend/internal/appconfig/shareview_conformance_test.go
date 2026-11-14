package appconfig

// #362 / #352: part of the DSL conformance suite on the Web share page. The
// data is shared, never copied: the §3.8 table in docs/dsl/README.md says
// which components the page draws, docs/dsl/conformance/cases holds the DSL
// and iOS's expected tree (headings and labels to look for).

import (
	"encoding/json"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var (
	docsDSLDir     = filepath.Join("..", "..", "..", "docs", "dsl")
	backtickTypeRe = regexp.MustCompile("`([a-zA-Z]+)`")
)

// webDocTable reads §3.8: types per kind of row ("描く…" = drawn,
// "代わりの表示" = placeholder / photo frame). The catch-all row
// ("上以外すべて") only lists examples and is skipped.
func webDocTable(t *testing.T) (drawn, alt map[string]bool) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(docsDSLDir, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	_, sec, ok := strings.Cut(string(raw), "### 3.8 ")
	if !ok {
		t.Fatal("docs/dsl/README.md has no §3.8")
	}
	sec, _, _ = strings.Cut(sec, "\n### ")
	drawn, alt = map[string]bool{}, map[string]bool{}
	for _, line := range strings.Split(sec, "\n") {
		cells := strings.Split(line, "|")
		if len(cells) < 4 || strings.Contains(cells[2], "上以外すべて") {
			continue
		}
		kind := strings.TrimSpace(cells[1])
		dst := drawn
		switch {
		case strings.HasPrefix(kind, "描く"):
		case kind == "代わりの表示":
			dst = alt
		default:
			continue
		}
		for _, m := range backtickTypeRe.FindAllStringSubmatch(cells[2], -1) {
			dst[m[1]] = true
		}
	}
	if len(drawn) < 20 || len(alt) == 0 {
		t.Fatalf("§3.8 table not parsed: drawn %d, alt %d", len(drawn), len(alt))
	}
	return drawn, alt
}

// schemaComponentTypes lists every component type in dsl.schema.json.
func schemaComponentTypes(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(docsDSLDir, "dsl.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Defs map[string]struct {
			Properties struct {
				Type struct {
					Const string   `json:"const"`
					Enum  []string `json:"enum"`
				} `json:"type"`
			} `json:"properties"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, d := range schema.Defs {
		if c := d.Properties.Type.Const; c != "" {
			out = append(out, c)
		}
		out = append(out, d.Properties.Type.Enum...) // Container: vstack, card, …
	}
	sort.Strings(out)
	return out
}

// Every component type renders as §3.8 says: drawn rows are not the
// placeholder, alternative rows are the photo frame or the placeholder, and
// types the table does not list fall back to the placeholder.
func TestShareViewMatchesDocTable(t *testing.T) {
	drawn, alt := webDocTable(t)
	types := schemaComponentTypes(t)
	inSchema := map[string]bool{}
	for _, typ := range types {
		inSchema[typ] = true
	}
	for typ := range drawn {
		if !inSchema[typ] {
			t.Errorf("§3.8 lists %q, which is not a schema type", typ)
		}
	}
	for _, typ := range types {
		raw := `{"schemaVersion":1,"app":{"name":"a"},"initialScreen":"s","screens":[{"id":"s","title":"s","components":[{"type":"` + typ + `","area":"A1:F1"}]}]}`
		p := buildWebPage("", []byte(raw), nil)
		if len(p.Screens) != 1 || len(p.Screens[0].Grid) != 1 {
			t.Errorf("%s: not rendered", typ)
			continue
		}
		kind := p.Screens[0].Grid[0].Kind
		switch {
		case drawn[typ]:
			if kind == "placeholder" || kind == "flashcards" || kind == "image" {
				t.Errorf("%s: §3.8 says drawn, got the %s frame", typ, kind)
			}
		case alt[typ]:
			if kind != "placeholder" && kind != "flashcards" && kind != "image" {
				t.Errorf("%s: §3.8 says alternative display, got %q", typ, kind)
			}
		default:
			if kind != "placeholder" {
				t.Errorf("%s: not in §3.8, so it must be the placeholder, got %q (add a row to §3.8)", typ, kind)
			}
		}
	}
}

// conformanceCase is the part of case.json / expected.json the Web uses.
type conformanceCase struct {
	DSL    json.RawMessage `json:"dsl"`
	Device struct {
		Locale string `json:"locale"`
	} `json:"device"`
}

type conformanceNode struct {
	Type     string            `json:"type"`
	State    string            `json:"state"`
	Props    map[string]any    `json:"props"`
	Children []conformanceNode `json:"children"`
}

type conformanceTree struct {
	Screens []struct {
		Title string            `json:"title"`
		Nodes []conformanceNode `json:"nodes"`
	} `json:"screens"`
}

// caseDSL resolves case.json's dsl (inline, or a path under docs/dsl).
func caseDSL(t *testing.T, c conformanceCase) []byte {
	t.Helper()
	var path string
	if json.Unmarshal(c.DSL, &path) == nil {
		raw, err := os.ReadFile(filepath.Join(docsDSLDir, filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	return c.DSL
}

// dslTypes collects every component type in the DSL (children included).
func dslTypes(raw []byte) map[string]bool {
	var doc struct {
		Screens []struct {
			Components []json.RawMessage `json:"components"`
		} `json:"screens"`
	}
	out := map[string]bool{}
	if json.Unmarshal(raw, &doc) != nil {
		return out
	}
	var walk func([]json.RawMessage)
	walk = func(cs []json.RawMessage) {
		for _, c := range cs {
			var comp struct {
				Type     string            `json:"type"`
				Children []json.RawMessage `json:"children"`
			}
			if json.Unmarshal(c, &comp) == nil {
				out[comp.Type] = true
				walk(comp.Children)
			}
		}
	}
	for _, s := range doc.Screens {
		walk(s.Components)
	}
	return out
}

const iosDefaultEmptyText = "まだありません"

// escapeText escapes s the way html/template does in text (it also escapes +).
func escapeText(t *testing.T, s string) string {
	t.Helper()
	var b strings.Builder
	if err := template.Must(template.New("").Parse("{{.}}")).Execute(&b, s); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// conformanceWords are the headings and labels iOS shows for a node (from its
// resolved props) that the Web page must show too. Values are not compared:
// the share page never shows records (#253).
func conformanceWords(n conformanceNode, out []string) []string {
	if n.State == "rendered" {
		for _, k := range []string{"title", "label", "text", "message", "emptyText"} {
			v, ok := n.Props[k].(string)
			// "まだありません" is iOS's wording when the DSL has no emptyText;
			// the Web says records are in the app instead (§3.8).
			if ok && strings.TrimSpace(v) != "" && (k != "emptyText" || v != iosDefaultEmptyText) {
				out = append(out, v)
			}
		}
		if cols, ok := n.Props["columns"].([]any); ok && n.Type == "table" {
			for _, c := range cols {
				if v, ok := c.(string); ok && v != "" {
					out = append(out, v)
				}
			}
		}
	}
	for _, c := range n.Children {
		out = conformanceWords(c, out)
	}
	return out
}

// The conformance cases that only use components the Web draws (§3.8 "描く")
// render without error, with no placeholder, and show every screen title and
// every heading / label / column iOS shows.
func TestShareViewConformanceCases(t *testing.T) {
	drawn, _ := webDocTable(t)
	dirs, err := filepath.Glob(filepath.Join(docsDSLDir, "conformance", "cases", "*"))
	if err != nil {
		t.Fatal(err)
	}
	ran := 0
	for _, dir := range dirs {
		name := filepath.Base(dir)
		raw, err := os.ReadFile(filepath.Join(dir, "case.json"))
		if err != nil {
			t.Fatal(err)
		}
		var c conformanceCase
		if err := json.Unmarshal(raw, &c); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		dslRaw := caseDSL(t, c)
		web := true
		for typ := range dslTypes(dslRaw) {
			web = web && drawn[typ]
		}
		if !web {
			continue
		}
		ran++
		t.Run(name, func(t *testing.T) {
			expRaw, err := os.ReadFile(filepath.Join(dir, "expected.json"))
			if err != nil {
				t.Fatal(err)
			}
			var exp conformanceTree
			if err := json.Unmarshal(expRaw, &exp); err != nil {
				t.Fatal(err)
			}
			p := buildWebPage("", dslRaw, nil)
			if !p.Found || len(p.Screens) == 0 || p.Truncated {
				t.Fatalf("page: found %v, %d screens, truncated %v", p.Found, len(p.Screens), p.Truncated)
			}
			var placeholders func([]webNode) int
			placeholders = func(ns []webNode) int {
				n := 0
				for _, x := range ns {
					if x.Kind == "placeholder" {
						n++
					}
					n += placeholders(x.Children)
				}
				return n
			}
			for _, s := range p.Screens {
				if n := placeholders(s.Grid) + placeholders(s.Stack); n > 0 {
					t.Errorf("screen %q: %d placeholder(s) for components §3.8 says are drawn", s.Title, n)
				}
			}
			p.Lang, p.Nonce = "ja", "n"
			if strings.HasPrefix(c.Device.Locale, "en") {
				p.Lang = "en"
			}
			body, err := renderSharePage(p)
			if err != nil {
				t.Fatal(err)
			}
			page := string(body)
			for _, s := range exp.Screens {
				words := conformanceWords(conformanceNode{Children: s.Nodes}, []string{s.Title})
				for _, w := range words {
					if w == "" {
						continue
					}
					// the page clips long strings; the start must be there
					if want := escapeText(t, strings.TrimSuffix(clip(w, 20), "…")); !strings.Contains(page, want) {
						t.Errorf("screen %q: %q is not on the page", s.Title, w)
					}
				}
			}
		})
	}
	if ran < 3 {
		t.Fatalf("only %d conformance case(s) use Web-drawn components only; the filter is broken", ran)
	}
	t.Logf("%d / %d conformance cases checked on the Web", ran, len(dirs))
}
