package dsl

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Server rules (#340, M4 approach A: declarations only). The vocabulary is
// closed (decisions #336-#339): triggers record created / updated and
// daily / weekly / monthly times; conditions are a subset of Expr; actions
// notify / createRecord / updateRecord / increment. No chains, no external
// sends, no arbitrary recipients. The runner (#341) enforces the same rules
// again at run time; this file is the generation-time gate.

// ServerRule is one automatic server action.
type ServerRule struct {
	ID string       `json:"id"`
	On RuleTrigger  `json:"on"`
	If *Expr        `json:"if,omitempty"`
	Do []RuleAction `json:"do"`
}

// RuleTrigger is a record trigger (Record + Entity [+ Fields]) or a time
// trigger (Every + Time [+ Weekday | Day]).
type RuleTrigger struct {
	Record  string   `json:"record,omitempty"` // created|updated
	Entity  string   `json:"entity,omitempty"`
	Fields  []string `json:"fields,omitempty"` // updated: fire only when one of these changed
	Every   string   `json:"every,omitempty"`  // daily|weekly|monthly
	Time    string   `json:"time,omitempty"`   // HH:MM in the creator's time zone
	Weekday string   `json:"weekday,omitempty"`
	Day     int      `json:"day,omitempty"` // monthly: 1..28
}

// RuleAction holds exactly one action.
type RuleAction struct {
	Notify       *RuleNotify       `json:"notify,omitempty"`
	CreateRecord *RuleCreateRecord `json:"createRecord,omitempty"`
	UpdateRecord *RuleUpdateRecord `json:"updateRecord,omitempty"`
	Increment    *RuleIncrement    `json:"increment,omitempty"`
}

// RuleNotify is a push notification with a fixed template.
type RuleNotify struct {
	To    RuleRecipient `json:"to"`
	Title string        `json:"title"`
	Body  string        `json:"body"`
}

// RuleRecipient is a closed recipient kind.
type RuleRecipient struct {
	Kind  string `json:"kind"`            // owner|author|authorOf|members
	Field string `json:"field,omitempty"` // authorOf: ref field of the trigger entity
}

// RuleCreateRecord creates one record of Entity.
type RuleCreateRecord struct {
	Entity string          `json:"entity"`
	Values map[string]Expr `json:"values"`
}

// RuleUpdateRecord updates the trigger record (Ref "") or the record its ref
// field Ref points to.
type RuleUpdateRecord struct {
	Ref    string          `json:"ref,omitempty"`
	Values map[string]Expr `json:"values"`
}

// RuleIncrement adds By (default 1) to a number field of the record the
// trigger's ref field Ref points to.
type RuleIncrement struct {
	Ref   string `json:"ref"`
	Field string `json:"field"`
	By    *Expr  `json:"by,omitempty"`
}

// Rule vocabulary values.
const (
	TriggerCreated = "created"
	TriggerUpdated = "updated"

	RecipientOwner    = "owner"
	RecipientAuthor   = "author"
	RecipientAuthorOf = "authorOf"
	RecipientMembers  = "members"

	// PrevPrefix reads a field's value before the update ("$prev.<field>").
	PrevPrefix = "$prev."
)

// Shape limits (#338). Validate enforces the Pro values; the plan-aware
// check (free) is ServerRuleQuotaProblems.
const (
	MaxServerRulesFree       = 3
	MaxServerRulesPro        = 20
	MaxScheduledRulesFree    = 1
	MaxScheduledRulesPro     = 5
	MaxRuleActionsFree       = 3
	MaxRuleActionsPro        = 5
	MaxRuleTriggerFields     = 10
	MaxNotifyTitleRunes      = 30
	MaxNotifyBodyRunes       = 80
	MaxNotifyInsertRunes     = 30 // the runner cuts each inserted value to this
	notifyAuthorPlaceholder  = RecordAuthor
	notifyPlaceholderPattern = `\{([^{}]*)\}`
)

var (
	ruleTimeRe          = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)
	ruleWeekdays        = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}
	ruleEvery           = []string{"daily", "weekly", "monthly"}
	notifyPlaceholderRe = regexp.MustCompile(notifyPlaceholderPattern)
	notifyURLRe         = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://|www\.|[a-z0-9-]+(\.[a-z0-9-]+)*\.[a-z]{2,})`)
	notifyEmailRe       = regexp.MustCompile(`@`)
	notifyDigitRun      = regexp.MustCompile(`[0-9][0-9\-\s().+]*[0-9]`)
	notifyNameWords     = []string{"公式", "運営", "tailor it", "tailorit", "apple"}
	// notifyFieldTypes may be inserted into a notification ({field}).
	notifyFieldTypes = []FieldType{FieldString, FieldText, FieldInteger, FieldNumber, FieldCurrency, FieldBoolean, FieldDate, FieldDatetime, FieldEnum}
)

// ruleScope is who a rule's data belongs to: private (one user's own),
// shared (creator + members) or public (anyone).
func (a *App) ruleScope(r *ServerRule) EntityScope {
	if r.On.Record != "" {
		if e := a.EntityByID(r.On.Entity); e != nil {
			return e.EffectiveScope()
		}
		return ScopePrivate
	}
	scope := ScopePrivate
	for i := range a.Entities {
		switch a.Entities[i].EffectiveScope() {
		case ScopePublic:
			return ScopePublic
		case ScopeShared:
			scope = ScopeShared
		}
	}
	return scope
}

// IsScheduled reports whether the rule has a time trigger.
func (r *ServerRule) IsScheduled() bool { return r.On.Every != "" }

// checkServerRules validates app.serverRules (#340).
func checkServerRules(a *App, entities map[string]*Entity, add func(string, ...any)) {
	if len(a.ServerRules) == 0 {
		return
	}
	if a.HasMatching() {
		add("serverRules: matching apps (entity match) cannot have server rules yet; remove serverRules")
	}
	if n := len(a.ServerRules); n > MaxServerRulesPro {
		add("serverRules: too many rules (%d > %d)", n, MaxServerRulesPro)
	}
	scheduled := 0
	ids := map[string]bool{}
	for i := range a.ServerRules {
		r := &a.ServerRules[i]
		where := fmt.Sprintf("serverRules[%d]", i)
		if !idRe.MatchString(r.ID) {
			add("%s: id %q is not a valid id", where, r.ID)
		}
		if ids[r.ID] {
			add("%s: duplicate rule id %q", where, r.ID)
		}
		ids[r.ID] = true
		if r.IsScheduled() {
			scheduled++
		}
		row := checkRuleTrigger(where, r, entities, add)
		scope := a.ruleScope(r)
		if r.If != nil {
			ch := ruleExprChecker(r, row, entities, add)
			if got := ch.check(where+".if", r.If, row, 1); !accepts(tBool, got) {
				add("%s.if: condition must be true/false (got %s); compare with eq / gt / and ...", where, got)
			}
			checkNoComputed(where+".if", ch, add)
		}
		if len(r.Do) == 0 {
			add("%s: do needs at least one action", where)
		}
		if len(r.Do) > MaxRuleActionsPro {
			add("%s: too many actions (%d > %d)", where, len(r.Do), MaxRuleActionsPro)
		}
		for j := range r.Do {
			checkRuleAction(fmt.Sprintf("%s.do[%d]", where, j), r, &r.Do[j], row, scope, entities, add)
		}
	}
	if scheduled > MaxScheduledRulesPro {
		add("serverRules: too many time-triggered rules (%d > %d)", scheduled, MaxScheduledRulesPro)
	}
	checkRuleChains(a, add)
}

func ruleExprChecker(r *ServerRule, row *Entity, entities map[string]*Entity, add func(string, ...any)) *exprChecker {
	ch := &exprChecker{entities: entities, add: add, deps: map[fieldKey]bool{}, server: true}
	if r.On.Record == TriggerUpdated {
		ch.prev = row
	}
	return ch
}

// checkPrev validates {"ref":"$prev.<field>"} (the value before the update).
func (c *exprChecker) checkPrev(where, ref string) exprType {
	if c.prev == nil {
		c.add("%s: ref %q: %s<field> (the value before the update) is only for record updated triggers", where, ref, PrevPrefix)
		return tAny
	}
	id := strings.TrimPrefix(ref, PrevPrefix)
	f := c.prev.FieldByID(id)
	if f == nil {
		c.add("%s: ref %q: %q is not a field of entity %q (fields: %s)", where, ref, id, c.prev.ID, fieldIDs(c.prev))
		return tAny
	}
	c.dep(c.prev, f.ID)
	return fieldExprType(f.Type)
}

// checkNoComputed rejects computed fields read by a server expression: they
// are calculated on the device and are not stored with the record.
func checkNoComputed(where string, ch *exprChecker, add func(string, ...any)) {
	keys := make([]string, 0, len(ch.deps))
	for k := range ch.deps {
		keys = append(keys, k.String())
	}
	slices.Sort(keys)
	for _, k := range keys {
		add("%s: computed field %s is calculated on the device and cannot be read by a server rule; use the fields it is computed from", where, k)
	}
}

// checkRuleTrigger validates the trigger and returns the trigger entity (the
// row of the rule's expressions; nil for a time trigger).
func checkRuleTrigger(where string, r *ServerRule, entities map[string]*Entity, add func(string, ...any)) *Entity {
	t := &r.On
	where += ".on"
	switch {
	case t.Record != "" && t.Every != "":
		add("%s: use either a record trigger (record + entity) or a time trigger (every + time), not both", where)
		return nil
	case t.Record == "" && t.Every == "":
		add("%s: needs record + entity (when a record is created / updated) or every + time (daily / weekly / monthly)", where)
		return nil
	case t.Every != "":
		if !slices.Contains(ruleEvery, t.Every) {
			add("%s: every %q must be daily, weekly or monthly", where, t.Every)
		}
		if !ruleTimeRe.MatchString(t.Time) {
			add("%s: time %q must be HH:MM", where, t.Time)
		}
		if t.Entity != "" || len(t.Fields) > 0 {
			add("%s: entity / fields are only for record triggers", where)
		}
		switch t.Every {
		case "weekly":
			if !slices.Contains(ruleWeekdays, t.Weekday) {
				add("%s: every weekly needs weekday (mon..sun)", where)
			}
			if t.Day != 0 {
				add("%s: day is only for every monthly", where)
			}
		case "monthly":
			if t.Day < 1 || t.Day > 28 {
				add("%s: every monthly needs day 1-28 (got %d)", where, t.Day)
			}
			if t.Weekday != "" {
				add("%s: weekday is only for every weekly", where)
			}
		default:
			if t.Weekday != "" || t.Day != 0 {
				add("%s: weekday / day are only for every weekly / monthly", where)
			}
		}
		return nil
	}
	if t.Record != TriggerCreated && t.Record != TriggerUpdated {
		add("%s: record %q must be created or updated", where, t.Record)
	}
	if t.Time != "" || t.Weekday != "" || t.Day != 0 {
		add("%s: time / weekday / day are only for time triggers (every)", where)
	}
	e := entities[t.Entity]
	if e == nil {
		add("%s: entity %q does not exist (entities: %s)", where, t.Entity, entityIDs(entities))
		return nil
	}
	if len(t.Fields) > 0 && t.Record != TriggerUpdated {
		add("%s: fields is only for record updated", where)
	}
	if len(t.Fields) > MaxRuleTriggerFields {
		add("%s: too many fields (%d > %d)", where, len(t.Fields), MaxRuleTriggerFields)
	}
	for _, id := range t.Fields {
		f := e.FieldByID(id)
		switch {
		case f == nil:
			add("%s: field %q is not a field of entity %q (fields: %s)", where, id, e.ID, fieldIDs(e))
		case f.Computed != nil:
			add("%s: field %q is computed on the device and never changes on the server; use the fields it is computed from", where, id)
		}
	}
	return e
}

// checkRuleAction validates one action of rule r. row is the trigger entity
// (nil = time trigger); scope the rule's data scope.
func checkRuleAction(where string, r *ServerRule, act *RuleAction, row *Entity, scope EntityScope, entities map[string]*Entity, add func(string, ...any)) {
	n := 0
	for _, set := range []bool{act.Notify != nil, act.CreateRecord != nil, act.UpdateRecord != nil, act.Increment != nil} {
		if set {
			n++
		}
	}
	if n != 1 {
		add("%s: write exactly one of notify, createRecord, updateRecord, increment (got %d)", where, n)
		return
	}
	switch {
	case act.Notify != nil:
		checkRuleNotify(where+".notify", act.Notify, row, scope, entities, add)
	case act.CreateRecord != nil:
		w := where + ".createRecord"
		target := entities[act.CreateRecord.Entity]
		if target == nil {
			add("%s: entity %q does not exist (entities: %s)", w, act.CreateRecord.Entity, entityIDs(entities))
			return
		}
		checkRuleTargetScope(w, target, scope, add)
		checkRuleValues(w, r, target, act.CreateRecord.Values, row, entities, add)
		for _, f := range target.Fields {
			if f.Required && f.Computed == nil {
				if _, ok := act.CreateRecord.Values[f.ID]; !ok {
					add("%s: required field %q of entity %q needs a value", w, f.ID, target.ID)
				}
			}
		}
	case act.UpdateRecord != nil:
		w := where + ".updateRecord"
		if row == nil {
			add("%s: updateRecord needs a record trigger (a time trigger has no record to update)", w)
			return
		}
		target := row
		if act.UpdateRecord.Ref != "" {
			if target = refTarget(w, row, act.UpdateRecord.Ref, entities, add); target == nil {
				return
			}
		}
		checkRuleTargetScope(w, target, scope, add)
		if len(act.UpdateRecord.Values) == 0 {
			add("%s: values is empty", w)
		}
		checkRuleValues(w, r, target, act.UpdateRecord.Values, row, entities, add)
	case act.Increment != nil:
		w := where + ".increment"
		if row == nil {
			add("%s: increment needs a record trigger (it adds to the record the trigger's ref field points to)", w)
			return
		}
		target := refTarget(w, row, act.Increment.Ref, entities, add)
		if target == nil {
			return
		}
		checkRuleTargetScope(w, target, scope, add)
		f := target.FieldByID(act.Increment.Field)
		switch {
		case f == nil:
			add("%s: field %q is not a field of entity %q (fields: %s)", w, act.Increment.Field, target.ID, fieldIDs(target))
		case f.Computed != nil:
			add("%s: field %q is computed; increment a stored number field", w, f.ID)
		case fieldExprType(f.Type) != tNum:
			add("%s: field %q is %s; increment needs an integer / number / currency field", w, f.ID, f.Type)
		}
		if act.Increment.By != nil {
			ch := ruleExprChecker(r, row, entities, add)
			if got := ch.check(w+".by", act.Increment.By, row, 1); !accepts(tNum, got) {
				add("%s.by: must be a number (got %s)", w, got)
			}
			checkNoComputed(w+".by", ch, add)
		}
	}
}

// refTarget resolves a type ref field of the trigger entity to its entity.
func refTarget(where string, row *Entity, ref string, entities map[string]*Entity, add func(string, ...any)) *Entity {
	f := row.FieldByID(ref)
	switch {
	case f == nil:
		add("%s: ref %q is not a field of entity %q (fields: %s)", where, ref, row.ID, fieldIDs(row))
		return nil
	case f.Type != FieldRef:
		add("%s: ref %q is %s; it must be a type ref field of entity %q", where, ref, f.Type, row.ID)
		return nil
	}
	return entities[f.RefEntity] // nil: reported by checkComputedFields
}

// checkRuleTargetScope keeps writes on the rule's side: a private rule
// writes only the user's own records, a shared / public rule only shared /
// public ones (#339).
func checkRuleTargetScope(where string, target *Entity, scope EntityScope, add func(string, ...any)) {
	if target.IsShared() != (scope != ScopePrivate) {
		add("%s: entity %q is %s but the rule works on %s data; a rule writes only records on the same side (private with private, shared / public with shared / public)", where, target.ID, target.EffectiveScope(), scope)
	}
}

func checkRuleValues(where string, r *ServerRule, target *Entity, values map[string]Expr, row *Entity, entities map[string]*Entity, add func(string, ...any)) {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		w := fmt.Sprintf("%s.values.%s", where, k)
		f := target.FieldByID(k)
		switch {
		case f == nil:
			add("%s: %q is not a field of entity %q (fields: %s)", w, k, target.ID, fieldIDs(target))
			continue
		case f.Computed != nil:
			add("%s: field %q is computed (read-only)", w, k)
			continue
		case f.Type == FieldPhoto || f.Type == FieldVideo || f.Type == FieldLocation:
			add("%s: a server rule cannot write a %s field", w, f.Type)
			continue
		}
		v := values[k]
		ch := ruleExprChecker(r, row, entities, add)
		got := ch.check(w, &v, row, 1)
		checkNoComputed(w, ch, add)
		want := fieldExprType(f.Type)
		if want == tList || got == tList {
			if want != got && got != tAny {
				add("%s: field %q is %s; the value must be a type refs field", w, k, f.Type)
			}
			continue
		}
		if !accepts(want, got) {
			add("%s: field %q is %s but the value is a %s%s", w, k, f.Type, got, convertHint(want))
		}
	}
}

// recipientsByScope are the recipient kinds a rule on that data may use
// (#339: public only the creator and the author of the ref target).
var recipientsByScope = map[EntityScope][]string{
	ScopePrivate: {RecipientOwner},
	ScopeShared:  {RecipientOwner, RecipientAuthor, RecipientAuthorOf, RecipientMembers},
	ScopePublic:  {RecipientOwner, RecipientAuthorOf},
}

func checkRuleNotify(where string, n *RuleNotify, row *Entity, scope EntityScope, entities map[string]*Entity, add func(string, ...any)) {
	k := n.To.Kind
	switch k {
	case RecipientOwner, RecipientAuthor, RecipientAuthorOf, RecipientMembers:
		if allowed := recipientsByScope[scope]; !slices.Contains(allowed, k) {
			add("%s.to: kind %q is not allowed on %s data (allowed: %s)", where, k, scope, strings.Join(allowed, ", "))
		}
	default:
		add("%s.to: kind %q must be owner, author, authorOf or members", where, k)
	}
	if (k == RecipientAuthor || k == RecipientAuthorOf) && row == nil {
		add("%s.to: kind %s needs a record trigger (a time trigger has no record)", where, k)
	}
	if k == RecipientAuthorOf {
		if row != nil {
			if t := refTarget(where+".to", row, n.To.Field, entities, add); t != nil && !t.IsShared() {
				add("%s.to: the records of entity %q are private (they have no other author); notify owner instead", where, t.ID)
			}
		}
	} else if n.To.Field != "" {
		add("%s.to: field is only for kind authorOf", where)
	}
	checkNotifyText(where+".title", n.Title, MaxNotifyTitleRunes, row, add)
	checkNotifyText(where+".body", n.Body, MaxNotifyBodyRunes, row, add)
}

// checkNotifyText validates a notification template: length, placeholders
// ({field} of the trigger record / {$author}), and no URL / email / phone /
// official-sounding words / control or invisible characters.
func checkNotifyText(where, s string, maxRunes int, row *Entity, add func(string, ...any)) {
	if strings.TrimSpace(s) == "" {
		add("%s: is empty", where)
		return
	}
	if n := utf8.RuneCountInString(s); n > maxRunes {
		add("%s: too long (%d > %d characters, placeholders included)", where, n, maxRunes)
	}
	for _, m := range notifyPlaceholderRe.FindAllStringSubmatch(s, -1) {
		checkPlaceholder(where, m[1], row, add)
	}
	literal := norm.NFKC.String(notifyPlaceholderRe.ReplaceAllString(s, " "))
	if strings.ContainsAny(literal, "{}") {
		add("%s: unmatched { or }; insert a value as {<field>} or {$author}", where)
	}
	for _, r := range literal {
		if unicode.Is(unicode.Cc, r) || unicode.Is(unicode.Cf, r) {
			add("%s: contains a control or invisible character (U+%04X)", where, r)
			break
		}
	}
	lower := strings.ToLower(literal)
	switch {
	case notifyURLRe.MatchString(lower):
		add("%s: must not contain a URL or domain (a notification only opens the app)", where)
	case notifyEmailRe.MatchString(lower):
		add("%s: must not contain an email address", where)
	}
	for _, run := range notifyDigitRun.FindAllString(literal, -1) {
		digits := 0
		for _, r := range run {
			if r >= '0' && r <= '9' {
				digits++
			}
		}
		if digits >= 10 {
			add("%s: must not contain a phone number", where)
			break
		}
	}
	for _, w := range notifyNameWords {
		if strings.Contains(lower, w) {
			add("%s: must not use %q (a notification must not look official / from the service)", where, w)
		}
	}
}

func checkPlaceholder(where, id string, row *Entity, add func(string, ...any)) {
	if row == nil {
		add("%s: {%s}: a time trigger has no record to insert values from", where, id)
		return
	}
	if id == notifyAuthorPlaceholder {
		if !row.IsShared() {
			add("%s: {%s} is only for shared / public entities", where, id)
		}
		return
	}
	f := row.FieldByID(id)
	switch {
	case f == nil:
		add("%s: {%s} is not a field of entity %q (fields: %s, or $author)", where, id, row.ID, fieldIDs(row))
	case f.Computed != nil:
		add("%s: {%s} is computed on the device and cannot be inserted", where, id)
	case f.Hidden:
		add("%s: {%s} is hidden from answerers and cannot be inserted", where, id)
	case !slices.Contains(notifyFieldTypes, f.Type):
		add("%s: {%s} is %s and cannot be inserted into a notification", where, id, f.Type)
	}
}

// checkRuleChains rejects a rule whose trigger would be fired by another
// rule's write: actions do not trigger rules (no chains, #336), so such a
// rule would silently never run on those writes.
func checkRuleChains(a *App, add func(string, ...any)) {
	type write struct {
		rule   string
		create bool
		fields []string // updated fields
	}
	writes := map[string][]write{} // entity id -> writes
	for i := range a.ServerRules {
		r := &a.ServerRules[i]
		row := a.EntityByID(r.On.Entity)
		for _, act := range r.Do {
			switch {
			case act.CreateRecord != nil:
				writes[act.CreateRecord.Entity] = append(writes[act.CreateRecord.Entity], write{rule: r.ID, create: true})
			case act.UpdateRecord != nil && row != nil:
				target := row.ID
				if act.UpdateRecord.Ref != "" {
					f := row.FieldByID(act.UpdateRecord.Ref)
					if f == nil {
						continue
					}
					target = f.RefEntity
				}
				fs := make([]string, 0, len(act.UpdateRecord.Values))
				for k := range act.UpdateRecord.Values {
					fs = append(fs, k)
				}
				writes[target] = append(writes[target], write{rule: r.ID, fields: fs})
			case act.Increment != nil && row != nil:
				if f := row.FieldByID(act.Increment.Ref); f != nil {
					writes[f.RefEntity] = append(writes[f.RefEntity], write{rule: r.ID, fields: []string{act.Increment.Field}})
				}
			}
		}
	}
	for i := range a.ServerRules {
		r := &a.ServerRules[i]
		if r.On.Record == "" {
			continue
		}
		for _, w := range writes[r.On.Entity] {
			hit := false
			switch r.On.Record {
			case TriggerCreated:
				hit = w.create
			case TriggerUpdated:
				hit = !w.create && (len(r.On.Fields) == 0 || slices.ContainsFunc(w.fields, func(f string) bool { return slices.Contains(r.On.Fields, f) }))
			}
			if hit {
				add("serverRules: rule %q writes entity %q, which triggers rule %q; records written by a rule do not trigger rules (no chains). Put the actions in one rule or trigger on a different entity / fields", w.rule, r.On.Entity, r.ID)
			}
		}
	}
}

// ServerRuleQuotaProblems checks the plan-dependent shape limits (#338): the
// free plan allows fewer rules, time triggers and actions than Validate
// (which enforces the Pro values). appconfig calls it when it knows the plan.
func (a *App) ServerRuleQuotaProblems(pro bool) []string {
	maxRules, maxScheduled, maxActions := MaxServerRulesFree, MaxScheduledRulesFree, MaxRuleActionsFree
	if pro {
		maxRules, maxScheduled, maxActions = MaxServerRulesPro, MaxScheduledRulesPro, MaxRuleActionsPro
	}
	var out []string
	if n := len(a.ServerRules); n > maxRules {
		out = append(out, fmt.Sprintf("serverRules: %d rules exceed the plan limit of %d", n, maxRules))
	}
	scheduled := 0
	for i := range a.ServerRules {
		r := &a.ServerRules[i]
		if r.IsScheduled() {
			scheduled++
		}
		if len(r.Do) > maxActions {
			out = append(out, fmt.Sprintf("serverRules[%d]: %d actions exceed the plan limit of %d", i, len(r.Do), maxActions))
		}
	}
	if scheduled > maxScheduled {
		out = append(out, fmt.Sprintf("serverRules: %d time-triggered rules exceed the plan limit of %d", scheduled, maxScheduled))
	}
	return out
}

var weekdayJA = map[string]string{"mon": "月", "tue": "火", "wed": "水", "thu": "木", "fri": "金", "sat": "土", "sun": "日"}

// DescribeServerRule is a plain Japanese sentence for the creator, e.g.
// 「出欠」が作られた時(条件に合えば)、「予定」を作った人に通知 (#340 / #347).
func (a *App) DescribeServerRule(r *ServerRule) string {
	ename := func(id string) string {
		if e := a.EntityByID(id); e != nil {
			return e.Name
		}
		return id
	}
	row := a.EntityByID(r.On.Entity)
	fname := func(e *Entity, id string) string {
		if e != nil {
			if f := e.FieldByID(id); f != nil {
				return f.Name
			}
		}
		return id
	}
	refName := func(ref string) string {
		if row != nil {
			if f := row.FieldByID(ref); f != nil {
				return ename(f.RefEntity)
			}
		}
		return ref
	}
	var when string
	t := r.On
	switch {
	case t.Every == "daily":
		when = "毎日 " + t.Time
	case t.Every == "weekly":
		when = fmt.Sprintf("毎週%s曜 %s", weekdayJA[t.Weekday], t.Time)
	case t.Every == "monthly":
		when = fmt.Sprintf("毎月 %d 日 %s", t.Day, t.Time)
	case t.Record == TriggerCreated:
		when = fmt.Sprintf("「%s」が作られた時", ename(t.Entity))
	case len(t.Fields) > 0:
		names := make([]string, len(t.Fields))
		for i, f := range t.Fields {
			names[i] = "「" + fname(row, f) + "」"
		}
		when = fmt.Sprintf("「%s」の%sが変わった時", ename(t.Entity), strings.Join(names, "・"))
	default:
		when = fmt.Sprintf("「%s」が更新された時", ename(t.Entity))
	}
	if r.If != nil {
		when += "(条件に合えば)"
	}
	acts := make([]string, 0, len(r.Do))
	for _, act := range r.Do {
		switch {
		case act.Notify != nil:
			var who string
			switch act.Notify.To.Kind {
			case RecipientOwner:
				who = "アプリの作成者"
			case RecipientAuthor:
				who = "書いた人"
			case RecipientAuthorOf:
				who = fmt.Sprintf("「%s」を作った人", refName(act.Notify.To.Field))
			case RecipientMembers:
				who = "参加者全員"
			}
			acts = append(acts, who+"に通知")
		case act.CreateRecord != nil:
			acts = append(acts, fmt.Sprintf("「%s」に記録を足す", ename(act.CreateRecord.Entity)))
		case act.UpdateRecord != nil && act.UpdateRecord.Ref == "":
			acts = append(acts, "その記録を書き換える")
		case act.UpdateRecord != nil:
			acts = append(acts, fmt.Sprintf("「%s」の記録を書き換える", refName(act.UpdateRecord.Ref)))
		case act.Increment != nil:
			var target *Entity
			if row != nil {
				if f := row.FieldByID(act.Increment.Ref); f != nil {
					target = a.EntityByID(f.RefEntity)
				}
			}
			acts = append(acts, fmt.Sprintf("「%s」の「%s」を数える", refName(act.Increment.Ref), fname(target, act.Increment.Field)))
		}
	}
	return when + "、" + strings.Join(acts, "・")
}
