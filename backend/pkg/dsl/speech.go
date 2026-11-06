package dsl

import "unicode/utf8"

// Speech leaves. Both run on the device only (no audio leaves the phone).
//   - transcribeField: push to talk; the on-device speech model (the same one
//     as voice input) turns the voice into text and appends it to a stored
//     string / text field of screen.entity. Keys: field / label / placeholder.
//   - speakButton: reads text aloud with the system voice of the UI language.
//     Exactly one source, like qrCode: text (fixed, `{{fieldId}}` = the screen
//     record), field (screen.entity field) or value (Expr). Keys: text / field
//     / value / label.
const (
	CompTranscribeField ComponentType = "transcribeField"
	CompSpeakButton     ComponentType = "speakButton"
)

// SpeakMaxText caps speakButton.text (characters).
const SpeakMaxText = 2000

// speakUnreadable lists field types whose value is not text to read aloud.
var speakUnreadable = map[FieldType]bool{FieldPhoto: true, FieldVideo: true, FieldAudio: true, FieldRefs: true}

func checkTranscribeField(where string, c *Component, formEntity *Entity, add func(string, ...any)) {
	if formEntity == nil {
		add("%s: transcribeField requires screen.entity", where)
		return
	}
	f := formEntity.FieldByID(c.Field)
	switch {
	case f == nil:
		add("%s: field %q not in entity %q (fields: %s)", where, c.Field, formEntity.ID, fieldIDs(formEntity))
	case f.Computed != nil:
		add("%s: field %q is computed (read-only); inputs cannot bind to it", where, c.Field)
	case f.Type != FieldString && f.Type != FieldText:
		add("%s: transcribeField field %q must be string or text (got %s)", where, c.Field, f.Type)
	}
}

func checkSpeakButton(where string, c *Component, formEntity *Entity, entities map[string]*Entity, add func(string, ...any)) {
	n := 0
	for _, set := range []bool{c.Text != "", c.Field != "", c.Value != nil} {
		if set {
			n++
		}
	}
	if n != 1 {
		add("%s: speakButton takes exactly one of text (fixed text), field (a field of screen.entity) or value (an expression)", where)
	}
	if c.Text != "" {
		if utf8.RuneCountInString(c.Text) > SpeakMaxText {
			add("%s: speakButton text is too long (max %d characters)", where, SpeakMaxText)
		}
		for _, m := range qrPlaceholderRe.FindAllStringSubmatch(c.Text, -1) {
			switch f := fieldOf(formEntity, m[1]); {
			case formEntity == nil:
				add("%s: speakButton text {{%s}} requires screen.entity", where, m[1])
			case f == nil:
				add("%s: speakButton text {{%s}} is not a field of entity %q (fields: %s)", where, m[1], formEntity.ID, fieldIDs(formEntity))
			case f.Hidden:
				add("%s: speakButton text {{%s}} is a hidden field; reading it aloud shows it to others — %s", where, m[1], hiddenHint)
			}
		}
	}
	if c.Field != "" {
		if formEntity == nil {
			add("%s: speakButton.field requires screen.entity (use text for a fixed sentence)", where)
		} else if f := formEntity.FieldByID(c.Field); f == nil {
			add("%s: field %q not in entity %q (fields: %s)", where, c.Field, formEntity.ID, fieldIDs(formEntity))
		} else if speakUnreadable[f.Type] {
			add("%s: speakButton field %q is type %s; use a text-like field (string / text / number / date / enum ...)", where, c.Field, f.Type)
		}
	}
	if c.Value != nil {
		ch := &exprChecker{entities: entities, add: add}
		ch.check(where+".value", c.Value, formEntity, 1)
		checkHiddenExpr(where+".value", c.Value, formEntity, entities, false, add)
	}
}

// fieldOf is FieldByID that tolerates a nil entity.
func fieldOf(e *Entity, id string) *Field {
	if e == nil {
		return nil
	}
	return e.FieldByID(id)
}
