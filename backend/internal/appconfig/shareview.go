package appconfig

// Read-only web view of a shared app (#362): GET /s/{token} turns the share's
// DSL snapshot into plain view nodes that sharepage.go renders with
// html/template. No JavaScript, no records (#253), nothing is interactive.
// Supported components: docs/dsl/README.md §3.8 (the Web column of #351).

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Rendering limits. They bound the work per request (the snapshot itself is
// already capped at dsl.MaxDSLBytes); anything past them is dropped and the
// page says the rest is in the app.
const (
	maxWebScreens  = 12
	maxWebNodes    = 400
	maxWebDepth    = 3 // screen > container > leaf (+1 for a local grid)
	maxWebText     = 400
	maxWebOptions  = 12
	maxWebColumns  = 8
	webRenderBudge = 200 * time.Millisecond
)

// RecordPeek is the seam for showing records on the share page later
// (#362 / #253). SharePage passes nil, so data-bound components only show
// their empty state. The renderer asks it about scope:"public" entities only
// (never private / shared data), whatever the implementation returns.
type RecordPeek interface {
	// Rows returns up to limit already-formatted rows of entity.
	Rows(entity string, limit int) []RecordRow
}

// RecordRow is one list row as display strings.
type RecordRow struct{ Title, Subtitle, Trailing string }

// webPage is the template's data.
type webPage struct {
	Found       bool
	Name        string
	Description string
	ImportURL   any // template.URL (the custom scheme is not on html/template's allowlist)
	StoreURL    string
	PageURL     string
	Nonce       string
	Accent      string // validated #RRGGBB, else ""
	Screens     []webScreen
	CSS         []webRule
	Truncated   bool
}

type webScreen struct {
	Anchor  string
	Title   string
	Form    bool
	Toolbar []string
	Grid    []webNode // children with `area`, on the 6-column grid
	Stack   []webNode // children without `area`, stacked below
}

// webRule is one generated placement class (ints only, see place).
type webRule struct {
	Class, Prop string
	From, To    int
}

// webNode is one rendered component. Kind selects the template branch.
type webNode struct {
	Kind        string
	Class       string // placement classes
	Style       string
	Title       string
	Text        string
	Label       string
	Unit        string
	Placeholder string
	ID          string // input id for <label for>
	Href        string // https only
	Options     []string
	Items       []string
	Rows        []RecordRow
	Cols        int
	Open        bool
	Grid        bool // children are placed on a local grid
	Children    []webNode
}

var hexColorRe = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

type webRenderer struct {
	app      *dsl.App
	peek     RecordPeek
	nodes    int
	seq      int
	deadline time.Time
	trunc    bool
	css      map[string]webRule
}

// buildWebPage decodes raw leniently (like iOS: unknown types and keys do not
// fail the page) and builds the view. A snapshot that does not decode still
// gets the name and the call to action.
func buildWebPage(name string, raw json.RawMessage, peek RecordPeek) webPage {
	p := webPage{Found: true, Name: clip(name, 80), StoreURL: AppStoreURL}
	if len(raw) == 0 || len(raw) > dsl.MaxDSLBytes {
		return p
	}
	var app dsl.App
	if err := json.Unmarshal(raw, &app); err != nil {
		return p
	}
	if p.Name == "" {
		p.Name = clip(app.App.Name, 80)
	}
	p.Description = clip(app.App.Description, 200)
	if app.App.Theme != nil && hexColorRe.MatchString(app.App.Theme.PrimaryColor) {
		p.Accent = app.App.Theme.PrimaryColor
	}
	r := &webRenderer{app: &app, peek: peek, deadline: time.Now().Add(webRenderBudge), css: map[string]webRule{}}
	for i, s := range orderedScreens(&app) {
		if i >= maxWebScreens {
			r.trunc = true
			break
		}
		p.Screens = append(p.Screens, r.screen(i, s))
	}
	p.Truncated = r.trunc
	for _, rule := range r.css {
		p.CSS = append(p.CSS, rule)
	}
	sort.Slice(p.CSS, func(i, j int) bool { return p.CSS[i].Class < p.CSS[j].Class })
	return p
}

// orderedScreens: tabs first (bar order), then initialScreen, then the rest.
func orderedScreens(a *dsl.App) []*dsl.Screen {
	seen := map[string]bool{}
	var out []*dsl.Screen
	add := func(id string) {
		for i := range a.Screens {
			if s := &a.Screens[i]; s.ID == id && !seen[id] {
				seen[id] = true
				out = append(out, s)
			}
		}
	}
	for _, t := range a.App.Tabs {
		add(t.Screen)
	}
	add(a.InitialScreen)
	for i := range a.Screens {
		add(a.Screens[i].ID)
	}
	return out
}

func (r *webRenderer) screen(i int, s *dsl.Screen) webScreen {
	out := webScreen{Anchor: fmt.Sprintf("s%d", i), Title: clip(s.Title, 80), Form: s.Entity != ""}
	for _, b := range s.Toolbar {
		if len(out.Toolbar) < 4 {
			out.Toolbar = append(out.Toolbar, clip(b.Label, 30))
		}
	}
	for j := range s.Components {
		c := &s.Components[j]
		a, err := dsl.ParseArea(c.Area)
		if c.Area == "" || err != nil {
			if n, ok := r.node(s, c, 1); ok {
				out.Stack = append(out.Stack, n)
			}
			continue
		}
		if n, ok := r.node(s, c, 1); ok {
			n.Class = r.place(a)
			out.Grid = append(out.Grid, n)
		}
	}
	return out
}

// place returns the classes for an area and records their CSS. Only ints go
// into the rules, so the generated stylesheet cannot carry DSL text.
func (r *webRenderer) place(a dsl.Area) string {
	col := fmt.Sprintf("gc%d-%d", a.Col1, a.Col2)
	row := fmt.Sprintf("gr%d-%d", a.Row1, a.Row2)
	r.css[col] = webRule{Class: col, Prop: "column", From: a.Col1 + 1, To: a.Col2 + 2}
	r.css[row] = webRule{Class: row, Prop: "row", From: a.Row1, To: a.Row2 + 1}
	return col + " " + row
}

// budget reports whether another node may be rendered.
func (r *webRenderer) budget() bool {
	if r.nodes >= maxWebNodes || time.Now().After(r.deadline) {
		r.trunc = true
		return false
	}
	r.nodes++
	return true
}

func (r *webRenderer) nextID() string {
	r.seq++
	return fmt.Sprintf("f%d", r.seq)
}

func (r *webRenderer) field(s *dsl.Screen, id string) *dsl.Field {
	if s.Entity == "" || id == "" {
		return nil
	}
	if e := r.app.EntityByID(s.Entity); e != nil {
		return e.FieldByID(id)
	}
	return nil
}

func (r *webRenderer) fieldName(entity, id string) string {
	if e := r.app.EntityByID(entity); e != nil {
		if f := e.FieldByID(id); f != nil {
			return f.Name
		}
	}
	return ""
}

// inputLabel is the component's label, else the bound field's name.
func (r *webRenderer) inputLabel(s *dsl.Screen, c *dsl.Component) string {
	if c.Label != "" {
		return clip(c.Label, 60)
	}
	if f := r.field(s, c.Field); f != nil {
		return clip(f.Name, 60)
	}
	return ""
}

func (r *webRenderer) node(s *dsl.Screen, c *dsl.Component, depth int) (webNode, bool) {
	if depth > maxWebDepth || !r.budget() {
		return webNode{}, false
	}
	n := webNode{Kind: "placeholder", Title: clip(c.Title, 80)}
	switch c.Type {
	case dsl.CompVStack, dsl.CompHStack, dsl.CompSection, dsl.CompCard, dsl.CompDisclosure, dsl.CompCarousel, dsl.CompGrid:
		n = r.container(s, c, depth)
	case dsl.CompText, dsl.CompLabel, dsl.CompMarkdown:
		n.Kind, n.Text, n.Style = "text", clip(c.Text, maxWebText), c.Style
		if c.Style == "title" || c.Style == "headline" {
			n.Kind = "heading"
		}
	case dsl.CompBadge:
		n.Kind, n.Text = "badge", clip(c.Text, 40)
	case dsl.CompDivider:
		n.Kind = "divider"
	case dsl.CompSpacer:
		n.Kind = "spacer"
	case dsl.CompIcon:
		n.Kind = "icon"
	case dsl.CompEmptyState:
		n.Kind, n.Text = "empty", clip(c.Message, 200)
	case dsl.CompStat, dsl.CompHealthStat, dsl.CompGauge, dsl.CompProgressBar:
		n.Kind, n.Label = "stat", clip(c.Label, 60)
		if c.Type == dsl.CompProgressBar || c.Type == dsl.CompGauge {
			n.Kind = "meter"
		}
	case dsl.CompList, dsl.CompTimeline:
		n = r.list(c)
	case dsl.CompTable:
		n.Kind = "table"
		for _, col := range c.Columns {
			if len(n.Items) >= maxWebColumns {
				break
			}
			t := col.Title
			if t == "" {
				t = r.fieldName(c.Entity, col.Field)
			}
			n.Items = append(n.Items, clip(t, 30))
		}
	case dsl.CompChart:
		n.Kind, n.Style = "chart", c.Kind
	case dsl.CompImage, dsl.CompAvatar, dsl.CompVideoPlayer:
		n.Kind, n.Style = "image", c.Height
	case dsl.CompTextField, dsl.CompSecureField, dsl.CompSearchBar, dsl.CompNumberField, dsl.CompTextEditor,
		dsl.CompDatePicker, dsl.CompTimePicker, dsl.CompColorPicker, dsl.CompStepper, dsl.CompSlider:
		n = r.input(s, c)
	case dsl.CompPicker:
		n.Kind, n.Label, n.ID = "select", r.inputLabel(s, c), r.nextID()
		if f := r.field(s, c.Field); f != nil {
			for _, o := range f.Options {
				if len(n.Options) < maxWebOptions {
					n.Options = append(n.Options, clip(o, 40))
				}
			}
		}
	case dsl.CompToggle:
		n.Kind, n.Label, n.ID = "toggle", r.inputLabel(s, c), r.nextID()
	case dsl.CompRating:
		n.Kind, n.Label, n.Cols = "rating", r.inputLabel(s, c), 5
		if c.Max != nil && *c.Max >= 1 && *c.Max <= 10 {
			n.Cols = int(*c.Max)
		}
		n.Text = strings.Repeat("☆", n.Cols)
	case dsl.CompPhotoField, dsl.CompVideoField:
		n.Kind, n.Label = "button", r.inputLabel(s, c)
		n.Style = "secondary"
	case dsl.CompButton, dsl.CompShareButton, dsl.CompOpenURLButton:
		n.Kind, n.Label, n.Style = "button", clip(c.Label, 40), c.Style
		if n.Label == "" {
			n.Label = "共有"
		}
	case dsl.CompLink:
		n.Kind, n.Text, n.Href = "link", clip(c.Text, 80), safeHref(c.URL)
	case dsl.CompFlashcards: // #264: the field names only (no cards), the deck is in the app
		front, back := r.fieldName(c.Entity, c.Front), r.fieldName(c.Entity, c.Back)
		if front != "" && back != "" {
			n.Title = clip("カード(表: "+front+" / 裏: "+back+")", 80)
		}
	}
	return n, true
}

func (r *webRenderer) container(s *dsl.Screen, c *dsl.Component, depth int) webNode {
	n := webNode{Kind: string(c.Type), Title: clip(c.Title, 80)}
	switch c.Type {
	case dsl.CompHStack, dsl.CompCarousel:
		n.Kind = "row"
	case dsl.CompVStack:
		n.Kind = "stack"
	case dsl.CompGrid:
		n.Kind, n.Cols = "grid", 2
		if c.ColumnCount >= dsl.MinGridContainerColumns && c.ColumnCount <= dsl.MaxGridContainerColumns {
			n.Cols = c.ColumnCount
		}
	case dsl.CompDisclosure:
		n.Open = c.Expanded != nil && *c.Expanded
	}
	// #81: card / section / vstack may lay children on a local grid when all
	// of them carry an area.
	local := dsl.LocalGridContainers[c.Type] && len(c.Children) > 0
	areas := make([]dsl.Area, len(c.Children))
	for i := range c.Children {
		a, err := dsl.ParseArea(c.Children[i].Area)
		if err != nil {
			local = false
			break
		}
		areas[i] = a
	}
	n.Grid = local
	for i := range c.Children {
		ch, ok := r.node(s, &c.Children[i], depth+1)
		if !ok {
			continue
		}
		if local {
			ch.Class = r.place(areas[i])
		}
		n.Children = append(n.Children, ch)
	}
	return n
}

func (r *webRenderer) list(c *dsl.Component) webNode {
	n := webNode{Kind: "list", Text: clip(c.EmptyText, 120)}
	if c.Item != nil {
		for _, f := range []string{c.Item.Title, c.Item.Subtitle, c.Item.Trailing} {
			if name := r.fieldName(c.Entity, f); name != "" {
				n.Items = append(n.Items, clip(name, 30))
			}
		}
	} else if name := r.fieldName(c.Entity, c.TitleField); name != "" {
		n.Items = append(n.Items, clip(name, 30))
	}
	if e := r.app.EntityByID(c.Entity); r.peek != nil && e != nil && e.EffectiveScope() == dsl.ScopePublic {
		for _, row := range r.peek.Rows(c.Entity, 20) {
			n.Rows = append(n.Rows, RecordRow{Title: clip(row.Title, 80), Subtitle: clip(row.Subtitle, 80), Trailing: clip(row.Trailing, 30)})
		}
	}
	return n
}

func (r *webRenderer) input(s *dsl.Screen, c *dsl.Component) webNode {
	n := webNode{Kind: "input", Style: "text", Label: r.inputLabel(s, c), ID: r.nextID(),
		Placeholder: clip(c.Placeholder, 60), Unit: clip(c.Unit, 10)}
	switch c.Type {
	case dsl.CompSecureField:
		n.Style = "password"
	case dsl.CompSearchBar:
		n.Style = "search"
	case dsl.CompNumberField, dsl.CompStepper:
		n.Style = "number"
	case dsl.CompDatePicker:
		n.Style = "date"
		if c.IncludeTime {
			n.Style = "datetime-local"
		}
	case dsl.CompTimePicker:
		n.Style = "time"
	case dsl.CompColorPicker:
		n.Style = "color"
	case dsl.CompSlider:
		n.Style = "range"
	case dsl.CompTextEditor:
		n.Kind = "textarea"
	case dsl.CompTextField:
		if c.Multiline {
			n.Kind = "textarea"
		}
	}
	if n.Unit == "" {
		if f := r.field(s, c.Field); f != nil {
			n.Unit = clip(f.Unit, 10)
		}
	}
	return n
}

// safeHref allows absolute https URLs only; anything else is shown as text.
func safeHref(u string) string {
	if len(u) > 2048 || !strings.HasPrefix(strings.ToLower(u), "https://") || strings.ContainsAny(u, " \t\r\n\"'<>\\") {
		return ""
	}
	return u
}

// clip trims s to n runes (with an ellipsis) and drops invalid UTF-8.
func clip(s string, n int) string {
	s = strings.ToValidUTF8(strings.TrimSpace(s), "")
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}
