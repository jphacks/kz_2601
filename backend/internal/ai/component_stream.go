package ai

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Streamed component events (#636). While a create turn streams, each
// direct component of the first visible screen (the initial tab, or
// initialScreen without tabs) is sent as `event: component` as soon as its
// object closes. They are provisional: a regeneration sends `event: restart`
// (discard them) and `event: result` always carries the final DSL.

// ComponentEvent is the minimal view of one streamed component.
type ComponentEvent struct {
	Index int    `json:"index"` // position in screens[].components
	ID    string `json:"id,omitempty"`
	Type  string `json:"type"`
	Area  string `json:"area,omitempty"`
	Label string `json:"label,omitempty"` // label | title | text | prompt
}

// createEventWatcher is the OnText hook for one create attempt: the `app`
// event first, then `component` events. *streamed is set once a component
// event has been sent. nil sink = no streaming.
func createEventWatcher(sink Sink, loc Locale, attempt int, streamed *bool) func(string) {
	if sink == nil {
		return nil
	}
	app := appEventWatcher(sink, loc, attempt)
	cw := &componentWatcher{emit: func(screen string, c ComponentEvent) {
		*streamed = true
		sink(Event{Type: "component", Attempt: attempt, Locale: loc, Screen: screen, Component: &c})
	}}
	return func(delta string) {
		app(delta)
		cw.Feed(delta)
	}
}

// restartEvent tells the receiver to drop the components of earlier attempts.
func restartEvent(sink Sink, loc Locale, attempt int, streamed bool) {
	if sink != nil && streamed {
		sink(Event{Type: "restart", Attempt: attempt, Locale: loc})
	}
}

// jsonFrame is one open object or array of the streamed document.
type jsonFrame struct {
	obj       bool
	start     int    // offset of the opening bracket in buf
	key       string // obj: current key
	expectKey bool   // obj: the next string is a key
	count     int    // arr: elements started so far
}

type streamedScreen struct {
	id    string
	comps [][]byte
}

// componentWatcher incrementally scans streamed JSON (O(n) overall) and
// collects root.screens[].components[] objects as they close. Malformed
// input stops the scan instead of failing; a code fence or preamble before
// the first brace is skipped like extractJSON does.
type componentWatcher struct {
	emit func(screen string, c ComponentEvent)

	buf      []byte
	pos      int
	started  bool
	stopped  bool
	inStr    bool
	esc      bool
	strStart int
	stack    []jsonFrame

	app      *dsl.AppMeta
	appSeen  bool
	initial  string
	screens  []*streamedScreen
	target   string // screen id shown first; "" until known
	targetIx int    // index into screens; -1 until bound
	sent     int
}

// Feed scans a text delta and emits the newly closed components.
func (w *componentWatcher) Feed(delta string) {
	if w.stopped {
		return
	}
	if !w.started {
		i := strings.IndexByte(delta, '{')
		if i < 0 {
			return
		}
		w.started, w.targetIx = true, -1
		delta = delta[i:]
	}
	w.buf = append(w.buf, delta...)
	for ; w.pos < len(w.buf) && !w.stopped; w.pos++ {
		w.step(w.buf[w.pos])
	}
	w.flush()
}

func (w *componentWatcher) step(c byte) {
	if w.inStr {
		switch {
		case w.esc:
			w.esc = false
		case c == '\\':
			w.esc = true
		case c == '"':
			w.inStr = false
			w.closeString()
		}
		return
	}
	switch c {
	case '"':
		w.inStr, w.strStart = true, w.pos
		if top := w.top(); top != nil && !top.obj {
			top.count++
		}
	case '{', '[':
		if top := w.top(); top != nil && !top.obj {
			top.count++
		}
		w.stack = append(w.stack, jsonFrame{obj: c == '{', start: w.pos, expectKey: c == '{'})
	case '}', ']':
		top := w.top()
		if top == nil || top.obj != (c == '}') {
			w.stopped = true // unbalanced: give up quietly
			return
		}
		w.closeFrame(w.buf[top.start : w.pos+1])
		w.stack = w.stack[:len(w.stack)-1]
		if len(w.stack) == 0 {
			w.stopped = true
		}
	case ':':
		if top := w.top(); top != nil && top.obj {
			top.expectKey = false
		}
	case ',':
		if top := w.top(); top != nil && top.obj {
			top.expectKey, top.key = true, ""
		}
	}
}

func (w *componentWatcher) top() *jsonFrame {
	if len(w.stack) == 0 {
		return nil
	}
	return &w.stack[len(w.stack)-1]
}

// inScreens reports whether the stack is root > "screens" array > ... .
func (w *componentWatcher) inScreens() bool {
	return len(w.stack) >= 2 && w.stack[0].key == "screens" && !w.stack[1].obj
}

func (w *componentWatcher) screen() *streamedScreen {
	i := w.stack[1].count - 1
	for len(w.screens) <= i {
		w.screens = append(w.screens, &streamedScreen{})
	}
	return w.screens[i]
}

func (w *componentWatcher) closeString() {
	top := w.top()
	if top == nil {
		return
	}
	raw := w.buf[w.strStart : w.pos+1]
	if top.obj && top.expectKey {
		var k string
		if json.Unmarshal(raw, &k) == nil {
			top.key = k
		}
		return
	}
	var v string
	if json.Unmarshal(raw, &v) != nil {
		return
	}
	switch {
	case len(w.stack) == 1 && top.key == "initialScreen":
		w.initial = v
	case len(w.stack) == 3 && w.inScreens() && top.obj && top.key == "id":
		w.screen().id = v
	}
}

func (w *componentWatcher) closeFrame(raw []byte) {
	n := len(w.stack) // frame being closed is w.stack[n-1]
	switch {
	case n == 2 && w.stack[0].key == "app" && w.stack[1].obj:
		w.appSeen = true
		var m dsl.AppMeta
		if json.Unmarshal(raw, &m) == nil {
			w.app = &m
		}
	case n == 5 && w.inScreens() && w.stack[2].obj && w.stack[2].key == "components" && !w.stack[3].obj && w.stack[4].obj:
		s := w.screen()
		s.comps = append(s.comps, slices.Clone(raw))
	}
}

// resolveTarget picks the first visible screen the way iOS does: with tabs,
// initialScreen if it is a tab, else the first tab; without tabs, initialScreen.
func (w *componentWatcher) resolveTarget() {
	if w.target != "" || !w.appSeen || w.initial == "" {
		return
	}
	w.target = w.initial
	if w.app != nil && len(w.app.Tabs) > 0 {
		isTab := slices.ContainsFunc(w.app.Tabs, func(t dsl.Tab) bool { return t.Screen == w.initial })
		if !isTab && w.app.Tabs[0].Screen != "" {
			w.target = w.app.Tabs[0].Screen
		}
	}
}

func (w *componentWatcher) flush() {
	w.resolveTarget()
	if w.target == "" {
		return
	}
	if w.targetIx < 0 {
		w.targetIx = slices.IndexFunc(w.screens, func(s *streamedScreen) bool { return s.id == w.target })
		if w.targetIx < 0 {
			return
		}
	}
	s := w.screens[w.targetIx]
	for ; w.sent < len(s.comps); w.sent++ {
		if c, ok := componentEventOf(s.comps[w.sent], w.sent); ok {
			w.emit(s.id, c)
		}
	}
}

func componentEventOf(raw []byte, index int) (ComponentEvent, bool) {
	var c struct {
		ID     string `json:"id"`
		Type   string `json:"type"`
		Area   string `json:"area"`
		Label  string `json:"label"`
		Title  string `json:"title"`
		Text   string `json:"text"`
		Prompt string `json:"prompt"`
	}
	if json.Unmarshal(raw, &c) != nil || strings.TrimSpace(c.Type) == "" {
		return ComponentEvent{}, false
	}
	label := c.Label
	for _, alt := range []string{c.Title, c.Text, c.Prompt} {
		if strings.TrimSpace(label) != "" {
			break
		}
		label = alt
	}
	return ComponentEvent{Index: index, ID: c.ID, Type: c.Type, Area: c.Area, Label: strings.TrimSpace(label)}, true
}
