package dsl

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

var errPrinter = message.NewPrinter(language.English)

//go:embed schema.json
var schemaJSON []byte

// SchemaJSON returns the embedded JSON Schema (same bytes as
// docs/dsl/dsl.schema.json; `make schema-sync` keeps them equal).
func SchemaJSON() []byte { return schemaJSON }

// Limits enforced by the semantic validator (not expressible for Claude
// structured outputs, so kept out of the JSON Schema).
const (
	MaxScreens            = 12
	MaxEntities           = 6
	MaxFieldsPerEntity    = 20
	MaxComponentsPerLevel = 30
	MaxDSLBytes           = 64 << 10
	MaxTabs               = 5
)

var idRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,39}$`)

var compiled = func() *jsonschema.Schema {
	c := jsonschema.NewCompiler()
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaJSON))
	if err != nil {
		panic("dsl: embedded schema is not JSON: " + err.Error())
	}
	if err := c.AddResource("dsl.schema.json", doc); err != nil {
		panic("dsl: add schema: " + err.Error())
	}
	s, err := c.Compile("dsl.schema.json")
	if err != nil {
		panic("dsl: compile schema: " + err.Error())
	}
	return s
}()

// ValidationError aggregates all problems found so the AI can be re-prompted
// with the full list at once.
type ValidationError struct {
	Problems []string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("dsl: %d problem(s): %s", len(e.Problems), strings.Join(e.Problems, "; "))
}

// Parse validates raw JSON against the schema and semantic rules, returning
// the typed App. This is the single entry point appconfig and ai must use.
func Parse(raw []byte) (*App, error) {
	if len(raw) > MaxDSLBytes {
		return nil, &ValidationError{Problems: []string{fmt.Sprintf("document exceeds %d bytes", MaxDSLBytes)}}
	}
	if err := ValidateSchema(raw); err != nil {
		return nil, err
	}
	var app App
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&app); err != nil {
		return nil, &ValidationError{Problems: []string{"decode: " + err.Error()}}
	}
	if err := app.Validate(); err != nil {
		return nil, err
	}
	return &app, nil
}

// ValidateSchema checks raw JSON against the embedded JSON Schema only.
func ValidateSchema(raw []byte) error {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return &ValidationError{Problems: []string{"invalid JSON: " + err.Error()}}
	}
	if err := compiled.Validate(doc); err != nil {
		var ve *jsonschema.ValidationError
		if errors.As(err, &ve) {
			return &ValidationError{Problems: flatten(ve)}
		}
		return &ValidationError{Problems: []string{err.Error()}}
	}
	return nil
}

func flatten(ve *jsonschema.ValidationError) []string {
	var out []string
	var walk func(v *jsonschema.ValidationError)
	walk = func(v *jsonschema.ValidationError) {
		if len(v.Causes) == 0 {
			loc := "/" + strings.Join(v.InstanceLocation, "/")
			out = append(out, loc+": "+v.ErrorKind.LocalizedString(errPrinter))
			return
		}
		for _, c := range v.Causes {
			walk(c)
		}
	}
	walk(ve)
	if len(out) > 20 {
		out = append(out[:20], fmt.Sprintf("...and %d more", len(out)-20))
	}
	return out
}

// Validate applies semantic rules that the JSON Schema cannot express:
// id format/uniqueness, reference integrity, size limits, per-type props.
func (a *App) Validate() error {
	var p []string
	add := func(format string, args ...any) { p = append(p, fmt.Sprintf(format, args...)) }

	if a.SchemaVersion != SchemaVersion {
		add("schemaVersion must be %d", SchemaVersion)
	}
	if strings.TrimSpace(a.App.Name) == "" {
		add("app.name is empty")
	}
	if len(a.Screens) == 0 {
		add("at least one screen is required")
	}
	if len(a.Screens) > MaxScreens {
		add("too many screens (%d > %d)", len(a.Screens), MaxScreens)
	}
	if len(a.Entities) > MaxEntities {
		add("too many entities (%d > %d)", len(a.Entities), MaxEntities)
	}

	entities := map[string]*Entity{}
	for i := range a.Entities {
		e := &a.Entities[i]
		if !idRe.MatchString(e.ID) {
			add("entities[%d].id %q is not a valid id", i, e.ID)
		}
		if _, dup := entities[e.ID]; dup {
			add("duplicate entity id %q", e.ID)
		}
		entities[e.ID] = e
		if len(e.Fields) == 0 {
			add("entity %q has no fields", e.ID)
		}
		if len(e.Fields) > MaxFieldsPerEntity {
			add("entity %q has too many fields", e.ID)
		}
		seen := map[string]bool{}
		for j, f := range e.Fields {
			if !idRe.MatchString(f.ID) {
				add("entity %q fields[%d].id %q is not a valid id", e.ID, j, f.ID)
			}
			if seen[f.ID] {
				add("entity %q duplicate field id %q", e.ID, f.ID)
			}
			seen[f.ID] = true
			if f.Type == FieldEnum && len(f.Options) == 0 {
				add("entity %q field %q: enum needs options", e.ID, f.ID)
			}
			if f.Type != FieldEnum && len(f.Options) > 0 {
				add("entity %q field %q: options only allowed for enum", e.ID, f.ID)
			}
		}
		if e.TitleField != "" && !seen[e.TitleField] {
			add("entity %q titleField %q does not exist", e.ID, e.TitleField)
		}
	}

	notifIDs := map[string]bool{}
	for i := range a.App.Notifications {
		n := &a.App.Notifications[i]
		if notifIDs[n.ID] {
			add("duplicate notification id %q", n.ID)
		}
		notifIDs[n.ID] = true
		checkNotification(fmt.Sprintf("app.notifications[%d]", i), n, add)
	}

	screens := map[string]bool{}
	for _, s := range a.Screens {
		if !idRe.MatchString(s.ID) {
			add("screen id %q is not a valid id", s.ID)
		}
		if screens[s.ID] {
			add("duplicate screen id %q", s.ID)
		}
		screens[s.ID] = true
	}
	if !screens[a.InitialScreen] {
		add("initialScreen %q does not exist", a.InitialScreen)
	}
	if n := len(a.App.Tabs); n > 0 {
		if n < 2 || n > MaxTabs {
			add("app.tabs must have 2-%d tabs (got %d); omit tabs for a single-stack app", MaxTabs, n)
		}
		seen := map[string]bool{}
		for i, t := range a.App.Tabs {
			if !screens[t.Screen] {
				add("app.tabs[%d]: screen %q does not exist", i, t.Screen)
			}
			if seen[t.Screen] {
				add("app.tabs[%d]: screen %q is used by another tab", i, t.Screen)
			}
			seen[t.Screen] = true
			if strings.TrimSpace(t.Title) == "" {
				add("app.tabs[%d]: title is empty", i)
			}
		}
	}

	checkAction := func(where string, act *Action) {
		if act == nil {
			return
		}
		switch act.Type {
		case ActionNavigate:
			if !screens[act.Screen] {
				add("%s: navigate target screen %q does not exist", where, act.Screen)
			}
		case ActionSaveRecord, ActionDeleteRecord:
			if entities[act.Entity] == nil {
				add("%s: %s entity %q does not exist", where, act.Type, act.Entity)
			}
			if act.Screen != "" && !screens[act.Screen] {
				add("%s: then-screen %q does not exist", where, act.Screen)
			}
		case ActionOpenURL:
			if act.URL == "" {
				add("%s: openUrl needs url", where)
			}
		case ActionScheduleNotif:
			if act.Notification == nil {
				add("%s: scheduleNotification needs notification", where)
			} else {
				checkNotification(where, act.Notification, add)
			}
		case ActionCancelNotif:
			if act.NotificationID == "" {
				add("%s: cancelNotification needs notificationId", where)
			}
		case ActionBack, ActionOpenVoice, ActionResetForm:
		default:
			add("%s: unknown action type %q", where, act.Type)
		}
	}

	for _, s := range a.Screens {
		var formEntity *Entity
		if s.Entity != "" {
			formEntity = entities[s.Entity]
			if formEntity == nil {
				add("screen %q: entity %q does not exist", s.ID, s.Entity)
			}
		}
		for i, tb := range s.Toolbar {
			checkAction(fmt.Sprintf("screen %q toolbar[%d]", s.ID, i), &tb.Action)
		}
		if len(s.Components) > MaxComponentsPerLevel {
			add("screen %q has too many components", s.ID)
		}
		checkGrid(s.ID, s.Components, add)
		compIDs := map[string]bool{}
		var walk func(where string, c *Component, depth int)
		walk = func(where string, c *Component, depth int) {
			if c.ID != "" {
				if !idRe.MatchString(c.ID) {
					add("%s: id %q is not a valid id", where, c.ID)
				}
				if compIDs[c.ID] {
					add("%s: duplicate component id %q in screen", where, c.ID)
				}
				compIDs[c.ID] = true
			}
			isContainer := ContainerTypes[c.Type]
			if isContainer {
				if depth >= 1 {
					add("%s: containers cannot be nested (max depth 1)", where)
				}
				if len(c.Children) > MaxComponentsPerLevel {
					add("%s: too many children", where)
				}
				for j := range c.Children {
					walk(fmt.Sprintf("%s.children[%d]", where, j), &c.Children[j], depth+1)
				}
				return
			}
			if len(c.Children) > 0 {
				add("%s: %s cannot have children", where, c.Type)
			}
			switch c.Type {
			case CompText:
				if c.Text == "" {
					add("%s: text is empty", where)
				}
			case CompButton:
				if c.Label == "" || c.Action == nil {
					add("%s: button needs label and action", where)
				}
				checkAction(where, c.Action)
			case CompTextField, CompNumberField, CompDatePicker, CompPicker, CompToggle, CompPhotoField, CompImage:
				if formEntity == nil {
					add("%s: %s requires screen.entity", where, c.Type)
				} else if f := formEntity.FieldByID(c.Field); f == nil {
					add("%s: field %q not in entity %q", where, c.Field, formEntity.ID)
				} else if c.Type == CompPicker && f.Type != FieldEnum {
					add("%s: picker field %q must be enum", where, c.Field)
				} else if (c.Type == CompPhotoField || c.Type == CompImage) && f.Type != FieldPhoto {
					add("%s: %s field %q must be type photo", where, c.Type, c.Field)
				}
			case CompList:
				e := entities[c.Entity]
				if e == nil {
					add("%s: list entity %q does not exist", where, c.Entity)
					break
				}
				if c.Item == nil {
					add("%s: list needs item", where)
					break
				}
				for _, fid := range []string{c.Item.Title, c.Item.Subtitle, c.Item.Trailing, c.Item.Image} {
					if fid != "" && e.FieldByID(fid) == nil {
						add("%s: list item field %q not in entity %q", where, fid, e.ID)
					}
				}
				if c.Sort != nil && e.FieldByID(c.Sort.Field) == nil {
					add("%s: sort field %q not in entity %q", where, c.Sort.Field, e.ID)
				}
				if c.Filter != nil && e.FieldByID(c.Filter.Field) == nil {
					add("%s: filter field %q not in entity %q", where, c.Filter.Field, e.ID)
				}
				checkAction(where+".onItemTap", c.OnItemTap)
			case CompStat, CompChart:
				e := entities[c.Entity]
				if e == nil {
					add("%s: %s entity %q does not exist", where, c.Type, c.Entity)
					break
				}
				if c.Aggregate != "count" && e.FieldByID(c.Field) == nil {
					add("%s: %s field %q not in entity %q (required unless aggregate=count)", where, c.Type, c.Field, e.ID)
				}
				if c.Type == CompChart && e.FieldByID(c.GroupBy) == nil {
					add("%s: chart groupBy %q not in entity %q", where, c.GroupBy, e.ID)
				}
				if c.Filter != nil && e.FieldByID(c.Filter.Field) == nil {
					add("%s: filter field %q not in entity %q", where, c.Filter.Field, e.ID)
				}
			case CompVoiceInput, CompSpacer, CompDivider:
			case CompLabel:
				if c.Text == "" || c.Icon == "" {
					add("%s: label needs text and icon", where)
				}
			case CompIcon:
				if c.Name == "" {
					add("%s: icon needs name", where)
				}
			case CompProgressBar:
				checkProgressBar(where, c, entities, add)
			case CompGauge:
				checkGauge(where, c, entities, add)
			case CompBadge:
				if c.Text == "" {
					add("%s: badge needs text", where)
				}
			case CompLink:
				if c.Text == "" {
					add("%s: link needs text", where)
				}
				if !isHTTPURL(c.URL) {
					add("%s: link url %q must be an http(s) URL", where, c.URL)
				}
			case CompMarkdown:
				if c.Text == "" {
					add("%s: markdown text is empty", where)
				}
			case CompAvatar:
				if c.Image != "" && !isHTTPURL(c.Image) && !strings.HasPrefix(c.Image, "data:image/") {
					add("%s: avatar image must be an https URL or data:image/... string", where)
				}
				if c.Field != "" {
					if formEntity == nil {
						add("%s: avatar field requires screen.entity", where)
					} else if f := formEntity.FieldByID(c.Field); f == nil {
						add("%s: field %q not in entity %q", where, c.Field, formEntity.ID)
					} else if f.Type != FieldPhoto {
						add("%s: avatar field %q must be type photo", where, c.Field)
					}
				}
			case CompTable:
				checkTable(where, c, entities, add)
			case CompTimeline:
				checkTimeline(where, c, entities, add)
			case CompEmptyState:
				if c.Title == "" {
					add("%s: emptyState needs title", where)
				}
			default:
				add("%s: unknown component type %q", where, c.Type)
			}
		}
		for i := range s.Components {
			walk(fmt.Sprintf("screen %q components[%d]", s.ID, i), &s.Components[i], 0)
		}
	}

	if len(p) > 0 {
		return &ValidationError{Problems: p}
	}
	return nil
}

func checkNotification(where string, n *Notification, add func(string, ...any)) {
	if !idRe.MatchString(n.ID) {
		add("%s: notification id %q is not a valid id", where, n.ID)
	}
	if n.Title == "" {
		add("%s: notification title is empty", where)
	}
	if n.Repeat == "weekly" && n.Weekday == "" {
		add("%s: weekly notification needs weekday", where)
	}
	if n.Time != "" && !timeRe.MatchString(n.Time) {
		add("%s: notification time %q must be HH:MM", where, n.Time)
	}
}

var timeRe = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

func isHTTPURL(s string) bool {
	return strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://")
}

// checkAggregateRef validates the entity/aggregate/field/filter trio shared by
// the display components that reuse the stat aggregation path. ok=false when
// the entity is missing (callers skip further checks).
func checkAggregateRef(where string, c *Component, entities map[string]*Entity, add func(string, ...any)) (*Entity, bool) {
	e := entities[c.Entity]
	if e == nil {
		add("%s: %s entity %q does not exist", where, c.Type, c.Entity)
		return nil, false
	}
	// aggregate omitted = sum, so a field is required unless aggregate=count.
	if c.Aggregate != "count" && e.FieldByID(c.Field) == nil {
		add("%s: %s field %q not in entity %q (required unless aggregate=count)", where, c.Type, c.Field, e.ID)
	}
	if c.Filter != nil && e.FieldByID(c.Filter.Field) == nil {
		add("%s: filter field %q not in entity %q", where, c.Filter.Field, e.ID)
	}
	return e, true
}

func checkProgressBar(where string, c *Component, entities map[string]*Entity, add func(string, ...any)) {
	if c.Value != nil {
		if *c.Value < 0 || *c.Value > 1 {
			add("%s: progressBar value %v must be within 0..1", where, *c.Value)
		}
		if c.Entity != "" {
			add("%s: progressBar takes either value or entity, not both", where)
		}
		return
	}
	if c.Entity == "" {
		add("%s: progressBar needs value or entity+target", where)
		return
	}
	if _, ok := checkAggregateRef(where, c, entities, add); !ok {
		return
	}
	if c.Target == nil || *c.Target <= 0 {
		add("%s: progressBar entity mode needs target > 0", where)
	}
}

func checkGauge(where string, c *Component, entities map[string]*Entity, add func(string, ...any)) {
	lo, hi := 0.0, 1.0
	if c.Min != nil {
		lo = *c.Min
	}
	if c.Max != nil {
		hi = *c.Max
	}
	if hi <= lo {
		add("%s: gauge max %v must be greater than min %v", where, hi, lo)
	}
	if c.Value != nil {
		if c.Entity != "" {
			add("%s: gauge takes either value or entity, not both", where)
		}
		return
	}
	if c.Entity == "" {
		add("%s: gauge needs value or entity", where)
		return
	}
	checkAggregateRef(where, c, entities, add)
}

func checkTable(where string, c *Component, entities map[string]*Entity, add func(string, ...any)) {
	e := entities[c.Entity]
	if e == nil {
		add("%s: table entity %q does not exist", where, c.Entity)
		return
	}
	if len(c.Columns) == 0 {
		add("%s: table needs at least one column", where)
	}
	for i, col := range c.Columns {
		if e.FieldByID(col.Field) == nil {
			add("%s: columns[%d] field %q not in entity %q", where, i, col.Field, e.ID)
		}
	}
	if c.Sort != nil && e.FieldByID(c.Sort.Field) == nil {
		add("%s: sort field %q not in entity %q", where, c.Sort.Field, e.ID)
	}
	if c.Filter != nil && e.FieldByID(c.Filter.Field) == nil {
		add("%s: filter field %q not in entity %q", where, c.Filter.Field, e.ID)
	}
	if c.Limit < 0 {
		add("%s: limit must be >= 0", where)
	}
}

func checkTimeline(where string, c *Component, entities map[string]*Entity, add func(string, ...any)) {
	e := entities[c.Entity]
	if e == nil {
		add("%s: timeline entity %q does not exist", where, c.Entity)
		return
	}
	if f := e.FieldByID(c.DateField); f == nil {
		add("%s: dateField %q not in entity %q", where, c.DateField, e.ID)
	} else if f.Type != FieldDate && f.Type != FieldDatetime {
		add("%s: dateField %q must be date or datetime", where, c.DateField)
	}
	if e.FieldByID(c.TitleField) == nil {
		add("%s: titleField %q not in entity %q", where, c.TitleField, e.ID)
	}
	if c.SubtitleField != "" && e.FieldByID(c.SubtitleField) == nil {
		add("%s: subtitleField %q not in entity %q", where, c.SubtitleField, e.ID)
	}
	if c.Filter != nil && e.FieldByID(c.Filter.Field) == nil {
		add("%s: filter field %q not in entity %q", where, c.Filter.Field, e.ID)
	}
	if c.Limit < 0 {
		add("%s: limit must be >= 0", where)
	}
}
