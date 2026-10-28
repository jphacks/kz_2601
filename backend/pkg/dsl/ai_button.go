package dsl

import (
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// placeholderRe matches {{fieldId}} in aiButton.prompt.
var placeholderRe = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_-]+)\s*\}\}`)

// Bounds of aiButton task generate (#417); /v1/text applies the same ones.
const (
	MaxAIInstructionRunes = 500
	MaxAIItems            = 10 // list count / fields names
	MaxAIItemNameRunes    = 20
	MaxAIGenerateRecords  = 20 // entity input: newest N records
	DefaultAIListCount    = 3
)

// AIShapes is the closed set of aiButton.shape values (task generate).
var AIShapes = map[string]bool{"line": true, "list": true, "fields": true}

// checkAIButton validates an on-device AI button (#78). It writes into the
// form draft, so it needs screen.entity and a stored text-like output field;
// classify additionally needs candidates (options or an enum output).
// Task generate has its own rules (checkAIGenerate).
func checkAIButton(where string, c *Component, form *Entity, entities map[string]*Entity, add func(string, ...any)) {
	if !AITasks[c.Task] {
		add("%s: aiButton task %q must be one of summarize|classify|rewrite|extract|suggest|generate", where, c.Task)
	}
	if c.Task == "generate" {
		checkAIGenerate(where, c, form, entities, add)
		return
	}
	if c.Shape != "" || c.Count != 0 || len(c.Items) > 0 {
		add("%s: aiButton shape / count / items are only for task generate", where)
	}
	if form == nil {
		add("%s: aiButton requires screen.entity", where)
		return
	}
	out := form.FieldByID(c.Output)
	switch {
	case c.Output == "":
		add("%s: aiButton %s needs output", where, c.Task)
	case out == nil:
		add("%s: aiButton output %q not in entity %q", where, c.Output, form.ID)
	case out.Computed != nil:
		add("%s: aiButton output %q is computed (read-only)", where, c.Output)
	case c.Task == "classify":
		switch out.Type {
		case FieldEnum:
			if len(c.Options) == 0 && len(out.Options) < 2 {
				add("%s: aiButton classify output %q needs at least 2 enum options", where, c.Output)
			}
			for _, o := range c.Options {
				if !slices.Contains(out.Options, o) {
					add("%s: aiButton option %q is not an option of enum field %q", where, o, c.Output)
				}
			}
		case FieldString, FieldText:
			if len(c.Options) < 2 {
				add("%s: aiButton classify needs options (2+) or an enum output field", where)
			}
		default:
			add("%s: aiButton classify output %q must be enum, string or text (got %s)", where, c.Output, out.Type)
		}
	default:
		if out.Type != FieldString && out.Type != FieldText {
			add("%s: aiButton %s output %q must be string or text (got %s)", where, c.Task, c.Output, out.Type)
		}
		if len(c.Options) > 0 {
			add("%s: aiButton options are only for task classify", where)
		}
	}
	if c.Input != "" && c.Entity != "" {
		add("%s: aiButton input and entity are exclusive", where)
	}
	if c.Input != "" {
		if f := form.FieldByID(c.Input); f == nil {
			add("%s: aiButton input %q not in entity %q", where, c.Input, form.ID)
		} else if c.Input == c.Output {
			add("%s: aiButton input and output must differ", where)
		}
	}
	if c.Entity != "" && entities[c.Entity] == nil {
		add("%s: aiButton entity %q does not exist", where, c.Entity)
	}
	if c.Limit != 0 && c.Entity == "" {
		add("%s: aiButton limit needs entity", where)
	}
	for _, m := range placeholderRe.FindAllStringSubmatch(c.Prompt, -1) {
		if m[1] != "input" && form.FieldByID(m[1]) == nil {
			add("%s: aiButton prompt placeholder {{%s}} is not a field of %q (or {{input}})", where, m[1], form.ID)
		}
	}
}

// checkAIGenerate (#417): a fixed instruction (prompt) answered in a shape.
// The result is shown under the button; output (optional) also writes it into
// the form draft. Input is optional: entity records (≤ 20) or a form field.
func checkAIGenerate(where string, c *Component, form *Entity, entities map[string]*Entity, add func(string, ...any)) {
	switch n := utf8.RuneCountInString(strings.TrimSpace(c.Prompt)); {
	case n == 0:
		add("%s: aiButton generate needs prompt (the instruction)", where)
	case n > MaxAIInstructionRunes:
		add("%s: aiButton generate prompt must be at most %d characters (got %d)", where, MaxAIInstructionRunes, n)
	}
	shape := c.Shape
	if shape == "" {
		shape = "line"
	}
	if !AIShapes[shape] {
		add("%s: aiButton shape %q must be line|list|fields", where, c.Shape)
	}
	if c.Count != 0 && (shape != "list" || c.Count < 1 || c.Count > MaxAIItems) {
		add("%s: aiButton count must be 1..%d and only with shape list", where, MaxAIItems)
	}
	if shape == "fields" {
		if len(c.Items) == 0 || len(c.Items) > MaxAIItems {
			add("%s: aiButton shape fields needs 1..%d items", where, MaxAIItems)
		}
		seen := map[string]bool{}
		for _, it := range c.Items {
			name := strings.TrimSpace(it)
			if name == "" || utf8.RuneCountInString(name) > MaxAIItemNameRunes || seen[name] {
				add("%s: aiButton items must be unique non-empty names of at most %d characters (%q)", where, MaxAIItemNameRunes, it)
			}
			seen[name] = true
		}
	} else if len(c.Items) > 0 {
		add("%s: aiButton items are only for shape fields", where)
	}
	if len(c.Options) > 0 {
		add("%s: aiButton options are only for task classify", where)
	}
	if c.Input != "" && c.Entity != "" {
		add("%s: aiButton input and entity are exclusive", where)
	}
	if c.Entity != "" && entities[c.Entity] == nil {
		add("%s: aiButton entity %q does not exist", where, c.Entity)
	}
	if c.Limit != 0 && c.Entity == "" {
		add("%s: aiButton limit needs entity", where)
	}
	if c.Limit > MaxAIGenerateRecords {
		add("%s: aiButton generate limit must be at most %d", where, MaxAIGenerateRecords)
	}
	placeholders := placeholderRe.FindAllStringSubmatch(c.Prompt, -1)
	if form == nil {
		if c.Output != "" || c.Input != "" {
			add("%s: aiButton output / input need screen.entity", where)
		}
		for _, m := range placeholders {
			if m[1] != "input" {
				add("%s: aiButton prompt placeholder {{%s}} needs screen.entity", where, m[1])
			}
		}
		return
	}
	if c.Output != "" {
		switch out := form.FieldByID(c.Output); {
		case out == nil:
			add("%s: aiButton output %q not in entity %q", where, c.Output, form.ID)
		case out.Computed != nil:
			add("%s: aiButton output %q is computed (read-only)", where, c.Output)
		case out.Type != FieldString && out.Type != FieldText:
			add("%s: aiButton generate output %q must be string or text (got %s)", where, c.Output, out.Type)
		}
	}
	if c.Input != "" {
		if form.FieldByID(c.Input) == nil {
			add("%s: aiButton input %q not in entity %q", where, c.Input, form.ID)
		} else if c.Input == c.Output {
			add("%s: aiButton input and output must differ", where)
		}
	}
	for _, m := range placeholders {
		if m[1] != "input" && form.FieldByID(m[1]) == nil {
			add("%s: aiButton prompt placeholder {{%s}} is not a field of %q (or {{input}})", where, m[1], form.ID)
		}
	}
}
