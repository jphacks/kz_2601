package dsl

import (
	"strings"
	"unicode/utf8"
)

// aiChat saveTo: record by talking ("今日カレー食べた" → a meal log). The AI
// reply may carry a suggested record over saveTo.fields; iOS shows it under
// the reply as a card (追加 / 直す) and saves it only when the user taps 追加,
// through the usual record write path (rules, plan limits, text filter).
// /v1/text task chat checks every suggested value against the field types
// (text_chat_suggest.go).

// MaxAIChatSaveFields caps aiChat saveTo.fields; /v1/text applies the same.
const MaxAIChatSaveFields = 8

// Bounds of what /v1/text reads of saveTo (entity / field names, unit, enum
// options). Validation applies them so an aiChat never sends more; the
// server drops a saveTo over them (the chat goes on without a suggestion).
const (
	MaxAIChatSaveNameRunes   = 40
	MaxAIChatSaveOptions     = 50
	MaxAIChatSaveOptionRunes = 50
	MaxAIChatSaveUnitRunes   = 20
)

// AIChatSaveFieldTypes are the field types an aiChat suggestion can fill.
var AIChatSaveFieldTypes = map[FieldType]bool{
	FieldString: true, FieldText: true, FieldNumber: true, FieldDate: true, FieldEnum: true,
}

// checkAIChatSaveTo: saveTo{entity, fields} adds to a private or shared
// entity other than the conversation's (not single / match / addressedTo),
// 1..8 unique stored fields of AIChatSaveFieldTypes that members may set
// (not editableBy admins, not hidden), and every other required field fills
// itself (defaultValue, or a boolean / enum / date type).
func checkAIChatSaveTo(where string, c *Component, entities map[string]*Entity, add func(string, ...any)) {
	s := c.SaveTo
	if s == nil {
		return
	}
	if s.Field != "" {
		add("%s: aiChat saveTo takes fields (a list of field ids), not field", where)
	}
	e := entities[s.Entity]
	if e == nil {
		add("%s: aiChat saveTo entity %q does not exist", where, s.Entity)
		return
	}
	switch {
	case e.ID == c.Entity:
		add("%s: aiChat saveTo entity must differ from the conversation entity %q (records go to a log such as meal{...})", where, e.ID)
	case e.EffectiveScope() == ScopePublic:
		add("%s: aiChat saveTo cannot add to public entity %q; use a private or shared entity", where, e.ID)
	case e.Single:
		add("%s: aiChat saveTo cannot add to single entity %q (it holds one settings record)", where, e.ID)
	case e.Match != "":
		add("%s: aiChat saveTo cannot add to match entity %q", where, e.ID)
	case e.IsAddressed():
		add("%s: aiChat saveTo cannot add to addressed entity %q (its records need a recipient)", where, e.ID)
	}
	if utf8.RuneCountInString(strings.TrimSpace(e.Name)) > MaxAIChatSaveNameRunes {
		add("%s: aiChat saveTo entity %q name must be at most %d characters", where, e.ID, MaxAIChatSaveNameRunes)
	}
	if len(s.Fields) == 0 || len(s.Fields) > MaxAIChatSaveFields {
		add("%s: aiChat saveTo needs 1..%d fields (the field ids the AI fills, e.g. [\"dish\", \"date\"])", where, MaxAIChatSaveFields)
	}
	seen := map[string]bool{}
	for _, id := range s.Fields {
		if seen[id] {
			add("%s: aiChat saveTo field %q is listed twice", where, id)
			continue
		}
		seen[id] = true
		f := e.FieldByID(id)
		switch {
		case f == nil:
			add("%s: aiChat saveTo field %q not in entity %q (fields: %s)", where, id, e.ID, fieldIDs(e))
		case f.Computed != nil:
			add("%s: aiChat saveTo field %q is computed (read-only)", where, id)
		case !AIChatSaveFieldTypes[f.Type]:
			add("%s: aiChat saveTo field %q must be string, text, number, date or enum (got %s)", where, id, f.Type)
		case f.AdminOnly():
			add("%s: aiChat saveTo field %q is editableBy admins; members could not save it", where, id)
		case f.Hidden:
			add("%s: aiChat saveTo field %q is hidden (an answer key)", where, id)
		default:
			checkAIChatSaveFieldBounds(where, f, add)
		}
	}
	for i := range e.Fields {
		x := &e.Fields[i]
		if !x.Required || x.Computed != nil || x.DefaultValue != nil || importFillsByDefault[x.Type] || seen[x.ID] {
			continue
		}
		add("%s: aiChat saveTo fills only its fields; required field %q of entity %q would stay empty (add it to fields, drop required or give it a defaultValue)", where, x.ID, e.ID)
	}
}

// checkAIChatSaveFieldBounds: the field's name, unit and enum options fit
// what /v1/text reads (MaxAIChatSave*).
func checkAIChatSaveFieldBounds(where string, f *Field, add func(string, ...any)) {
	if n := utf8.RuneCountInString(strings.TrimSpace(f.Name)); n == 0 || n > MaxAIChatSaveNameRunes {
		add("%s: aiChat saveTo field %q name must be 1..%d characters", where, f.ID, MaxAIChatSaveNameRunes)
	}
	if utf8.RuneCountInString(strings.TrimSpace(f.Unit)) > MaxAIChatSaveUnitRunes {
		add("%s: aiChat saveTo field %q unit must be at most %d characters", where, f.ID, MaxAIChatSaveUnitRunes)
	}
	if f.Type != FieldEnum {
		return
	}
	if len(f.Options) > MaxAIChatSaveOptions {
		add("%s: aiChat saveTo field %q must have at most %d options", where, f.ID, MaxAIChatSaveOptions)
	}
	for _, o := range f.Options {
		if strings.TrimSpace(o) == "" || utf8.RuneCountInString(o) > MaxAIChatSaveOptionRunes {
			add("%s: aiChat saveTo field %q options must be 1..%d characters", where, f.ID, MaxAIChatSaveOptionRunes)
			break
		}
	}
}
