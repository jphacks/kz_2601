package dsl

import (
	"slices"
	"strings"
	"unicode/utf8"
)

// aiChat: a multi-turn conversation with the cloud AI (counselor / tutor /
// practice partner). Bubbles like chatThread, the user on the right and the
// AI on the left; each turn is one record of entity, so the history stays.
// The AI reads the last MaxAIChatTurns turns (≤ MaxAIChatBytes) through
// POST /v1/text task chat (Pro, paid by the app's creator in a shared app).
// Every AI bubble has a speaker button (the on-device voice of speakButton,
// nothing is sent); speak: true also reads each new reply aloud. speak is a
// plain bool of aiChat only: the schema rejects it on any other part, and it
// needs no cross-check here.

// Bounds of aiChat; /v1/text task chat applies the same ones.
const (
	MaxAIChatPersonaRunes = MaxAIInstructionRunes
	MaxAIChatTurns        = 20
	MaxAIChatBytes        = 16 << 10
)

// checkAIChat: textField is a stored string/text field, roleField (required:
// the role of every turn is stored, never inferred from the order, so one
// failed save cannot flip later turns) a stored string field or an enum with
// the options user and assistant,
// persona is required (≤ 500 chars), length short|long, the entity is
// private (the history the AI reads is only the user's own conversation),
// and every other required field has a defaultValue (turns are saved
// without a form).
func checkAIChat(where string, c *Component, entities map[string]*Entity, add func(string, ...any)) {
	switch n := utf8.RuneCountInString(strings.TrimSpace(c.Persona)); {
	case n == 0:
		add("%s: aiChat needs persona (who the AI is and how it talks, e.g. a gentle counselor who listens first)", where)
	case n > MaxAIChatPersonaRunes:
		add("%s: aiChat persona must be at most %d characters", where, MaxAIChatPersonaRunes)
	}
	if c.Length != "" && !AILengths[c.Length] {
		add("%s: aiChat length %q must be short|long", where, c.Length)
	}
	e := entities[c.Entity]
	if e == nil {
		add("%s: aiChat entity %q does not exist", where, c.Entity)
		return
	}
	if e.EffectiveScope() != ScopePrivate {
		add(`%s: aiChat entity %q must be private (drop "scope"): the conversation is the user's own and the AI reads only those turns`, where, e.ID)
	}
	if f := e.FieldByID(c.TextField); f == nil {
		add("%s: aiChat textField %q not in entity %q (fields: %s)", where, c.TextField, e.ID, fieldIDs(e))
	} else if (f.Type != FieldString && f.Type != FieldText) || f.Computed != nil {
		add("%s: aiChat textField %q must be a stored string or text field (the turn body)", where, c.TextField)
	}
	if c.RoleField == "" {
		add("%s: aiChat needs roleField (a field that stores user / assistant for each turn, e.g. role: enum[user, assistant])", where)
	} else {
		switch f := e.FieldByID(c.RoleField); {
		case f == nil:
			add("%s: aiChat roleField %q not in entity %q (fields: %s)", where, c.RoleField, e.ID, fieldIDs(e))
		case f.Computed != nil || (f.Type != FieldString && f.Type != FieldEnum):
			add("%s: aiChat roleField %q must be a stored string or enum field (it gets user / assistant)", where, c.RoleField)
		case c.RoleField == c.TextField:
			add("%s: aiChat roleField and textField must differ", where)
		case f.Type == FieldEnum && (!slices.Contains(f.Options, "user") || !slices.Contains(f.Options, "assistant")):
			add("%s: aiChat roleField %q is an enum, so its options must include user and assistant", where, c.RoleField)
		}
	}
	checkAIChatSaveTo(where, c, entities, add) // ai_chat_save_to.go
	for _, f := range e.Fields {
		if f.Required && f.Computed == nil && f.DefaultValue == nil && f.ID != c.TextField && f.ID != c.RoleField {
			add("%s: aiChat saves turns without a form, so required field %q of entity %q needs a defaultValue (or make it optional; the time is $createdAt, not a field)", where, f.ID, e.ID)
		}
	}
}
