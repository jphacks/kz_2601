package dsl

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Field.encrypted (#233, Tailor It Pro): iOS seals the value with the user's
// key (AES-GCM, kept in the iCloud Keychain so the same Apple ID's devices
// read it) and stores the envelope "enc:v1:<base64>" in the record, so
// appconfig / Firestore / the runner / the AI only ever see ciphertext.
//
// Because no server can read the value, an encrypted field is:
//   - string / text only (appconfig keeps the envelope as a string),
//   - only in apps whose entities are all private (shared data is read by
//     other people, who do not have the key),
//   - never read by an expression, a filter / sort / aggregation, a server
//     rule, entity.unique, the online AI (aiButton input / {{field}},
//     aiChat textField) or nearbyShare.
//
// iOS also leaves encrypted fields out of the AI material it builds
// (aiButton without input, aiChat inputEntity).

// EncryptedPrefix starts every stored value of an encrypted field.
const EncryptedPrefix = "enc:v1:"

// HasEncryptedFields reports whether any entity of a has an encrypted field.
func (a *App) HasEncryptedFields() bool {
	if a == nil {
		return false
	}
	for i := range a.Entities {
		if a.Entities[i].hasEncrypted() {
			return true
		}
	}
	return false
}

func (e *Entity) hasEncrypted() bool {
	for i := range e.Fields {
		if e.Fields[i].Encrypted {
			return true
		}
	}
	return false
}

func encryptedField(e *Entity, id string) bool {
	if e == nil {
		return false
	}
	f := e.FieldByID(id)
	return f != nil && f.Encrypted
}

const encryptedHint = "the server and the AI only see the encrypted value; drop encrypted from the field or use another field"

func checkEncryptedFields(a *App, entities map[string]*Entity, add func(string, ...any)) {
	if !a.HasEncryptedFields() {
		return
	}
	for i := range a.Entities {
		e := &a.Entities[i]
		if e.IsShared() {
			add("entity %q is %s: encrypted fields are only for apps whose data is private (other people do not have the key); make the app private or drop encrypted", e.ID, e.EffectiveScope())
		}
		for j := range e.Fields {
			f := &e.Fields[j]
			if !f.Encrypted {
				continue
			}
			where := fmt.Sprintf("entity %q field %q", e.ID, f.ID)
			if f.Type != FieldString && f.Type != FieldText {
				add("%s: encrypted is only for string / text fields (got %s)", where, f.Type)
			}
			if f.Computed != nil || f.Hidden {
				add("%s: a computed or hidden field cannot be encrypted", where)
			}
			if f.DefaultValue != nil {
				add("%s: an encrypted field cannot have a defaultValue (the server would store it as plain text)", where)
			}
			for _, u := range e.Unique {
				if u == f.ID {
					add("%s: an encrypted field cannot be in unique (the server cannot compare it)", where)
				}
			}
		}
		for j := range e.Fields {
			if c := e.Fields[j].Computed; c != nil {
				checkEncryptedExpr(fmt.Sprintf("entity %q field %q computed", e.ID, e.Fields[j].ID), c, e, entities, add)
			}
		}
	}
	for i := range a.App.Notifications {
		n := &a.App.Notifications[i]
		checkEncryptedExpr(fmt.Sprintf("app.notifications[%d].when", i), n.When, entities[n.Entity], entities, add)
	}
	var walk func(where string, form *Entity, c *Component)
	walk = func(where string, form *Entity, c *Component) {
		checkEncryptedComponent(where, form, c, entities, add)
		for i := range c.Children {
			walk(fmt.Sprintf("%s.children[%d]", where, i), form, &c.Children[i])
		}
	}
	for _, s := range a.Screens {
		for i := range s.Components {
			walk(fmt.Sprintf("screen %q components[%d]", s.ID, i), entities[s.Entity], &s.Components[i])
		}
	}
	for i := range a.ServerRules {
		checkEncryptedRule(fmt.Sprintf("serverRules[%d]", i), &a.ServerRules[i], entities, add)
	}
}

func checkEncryptedComponent(where string, form *Entity, c *Component, entities map[string]*Entity, add func(string, ...any)) {
	bound := entities[c.Entity]
	if bound == nil {
		bound = form
	}
	use := func(key, field string) {
		if field != "" && encryptedField(bound, field) {
			add("%s: %s %q is encrypted; %s", where, key, field, encryptedHint)
		}
	}
	if c.Filter != nil {
		use("filter.field", c.Filter.Field)
	}
	if c.Sort != nil {
		use("sort.field", c.Sort.Field)
	}
	switch c.Type {
	case CompStat, CompChart, CompProgressBar, CompGauge:
		if c.Aggregate != "count" {
			use("field", c.Field)
		}
		use("by", c.By)
		use("groupBy", c.GroupBy)
	case CompTable:
		use("groupBy", c.GroupBy)
	case CompGridTable:
		use("columnField", c.ColumnField)
		use("rowField", c.RowField)
	case CompAiButton:
		if encryptedField(form, c.Input) {
			add("%s: aiButton input %q is encrypted; %s", where, c.Input, encryptedHint)
		}
		for _, m := range placeholderRe.FindAllStringSubmatch(c.Prompt, -1) {
			if encryptedField(form, m[1]) {
				add("%s: aiButton prompt {{%s}} is encrypted; %s", where, m[1], encryptedHint)
			}
		}
	case CompAiChat:
		use("textField", c.TextField)
	case CompNearbyShare:
		if bound != nil && bound.hasEncrypted() {
			add("%s: nearbyShare sends records of entity %q, which has encrypted fields; %s", where, bound.ID, encryptedHint)
		}
	}
	checkEncryptedExpr(where+".value", c.Value, form, entities, add)
	checkEncryptedExpr(where+".target", c.Target, form, entities, add)
}

// checkEncryptedExpr reports every encrypted field e reads (row = the
// expression's record, nil when it has none).
func checkEncryptedExpr(where string, e *Expr, row *Entity, entities map[string]*Entity, add func(string, ...any)) {
	walkExpr(e, func(x *Expr) {
		switch x.Kind {
		case ExprRef:
			if row == nil {
				return
			}
			if owner, f := resolveRefField(x.Ref, row, entities); f != nil && f.Encrypted {
				add("%s: reads encrypted field %s.%s; %s", where, owner, f.ID, encryptedHint)
			}
		case ExprAgg:
			if x.Agg == nil {
				return
			}
			t := entities[x.Agg.Entity]
			fields := []string{x.Agg.Field, x.Agg.By}
			for _, f := range x.Agg.Filter {
				fields = append(fields, f.Field)
			}
			for _, f := range fields {
				if f != "" && encryptedField(t, f) {
					add("%s: agg reads encrypted field %s.%s; %s", where, t.ID, f, encryptedHint)
				}
			}
		case ExprSetting:
			for _, id := range sortedKeys(entities) {
				if t := entities[id]; t.Single && encryptedField(t, x.Setting) {
					add("%s: setting reads encrypted field %s.%s; %s", where, t.ID, x.Setting, encryptedHint)
				}
			}
		}
	})
}

// checkEncryptedRule: the runner reads records on the server, so a rule must
// not name an encrypted field of an entity it touches (any key or value of
// the rule, or a {field} in a notification).
func checkEncryptedRule(where string, r *ServerRule, entities map[string]*Entity, add func(string, ...any)) {
	raw, err := json.Marshal(r)
	if err != nil {
		return
	}
	var doc any
	if json.Unmarshal(raw, &doc) != nil {
		return
	}
	touched := map[string]bool{r.On.Entity: true, r.On.Each: true}
	for _, id := range r.ReadEntities() {
		touched[id] = true
	}
	var words []string
	var walk func(v any)
	walk = func(v any) {
		switch t := v.(type) {
		case map[string]any:
			for k, x := range t {
				if s, ok := x.(string); ok && (k == "entity" || k == "each") {
					touched[s] = true
				}
				words = append(words, k)
				walk(x)
			}
		case []any:
			for _, x := range t {
				walk(x)
			}
		case string:
			words = append(words, t)
		}
	}
	walk(doc)
	seen := map[string]bool{}
	for _, id := range sortedKeys(touched) {
		e := entities[id]
		if e == nil {
			continue
		}
		for i := range e.Fields {
			f := &e.Fields[i]
			if !f.Encrypted || seen[e.ID+"."+f.ID] {
				continue
			}
			for _, w := range words {
				if w == f.ID || strings.Contains(w, "{"+f.ID+"}") {
					seen[e.ID+"."+f.ID] = true
					add("%s: the server rule reads encrypted field %s.%s; %s", where, e.ID, f.ID, encryptedHint)
					break
				}
			}
		}
	}
}

// encryptedNotifications warns when a record notification shows an encrypted
// field: the device decrypts it, so the text appears on the lock screen.
func (l *linter) encryptedNotifications() {
	for i := range l.app.App.Notifications {
		n := &l.app.App.Notifications[i]
		if n.TitleField != "" && encryptedField(l.entities[n.Entity], n.TitleField) {
			l.add("app.notifications[%d]: titleField %q is encrypted but the notification shows it on the lock screen; use a field that is not secret", i, n.TitleField)
		}
	}
}
