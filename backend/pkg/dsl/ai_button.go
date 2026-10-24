package dsl

import (
	"regexp"
	"slices"
)

// placeholderRe matches {{fieldId}} in aiButton.prompt.
var placeholderRe = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_-]+)\s*\}\}`)

// checkAIButton validates an on-device AI button (#78). It writes into the
// form draft, so it needs screen.entity and a stored text-like output field;
// classify additionally needs candidates (options or an enum output).
func checkAIButton(where string, c *Component, form *Entity, entities map[string]*Entity, add func(string, ...any)) {
	if !AITasks[c.Task] {
		add("%s: aiButton task %q must be one of summarize|classify|rewrite|extract|suggest", where, c.Task)
	}
	if form == nil {
		add("%s: aiButton requires screen.entity", where)
		return
	}
	out := form.FieldByID(c.Output)
	switch {
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
