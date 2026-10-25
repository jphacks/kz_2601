package dsl

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
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
	MaxFieldsPerEntity    = 40 // computed fields count too (#221: 20 made edits drop existing fields)
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
		causes := v.Causes
		if _, ok := v.ErrorKind.(*kind.AnyOf); ok {
			causes = deepestCauses(matchingTypeCauses(v.InstanceLocation, causes))
		}
		for _, c := range causes {
			walk(c)
		}
	}
	walk(ve)
	if len(out) > 20 {
		out = append(out[:20], fmt.Sprintf("...and %d more", len(out)-20))
	}
	return out
}

// matchingTypeCauses drops anyOf branches that failed on the node's own
// `type` (a component def whose const did not match), so a stat with a bad
// value reports the value, not 50 "type must be text/button/..." lines.
// If every branch failed on type (unknown type), all are kept.
func matchingTypeCauses(loc []string, causes []*jsonschema.ValidationError) []*jsonschema.ValidationError {
	typeLoc := strings.Join(append(append([]string{}, loc...), "type"), "/")
	var failsOnType func(v *jsonschema.ValidationError) bool
	failsOnType = func(v *jsonschema.ValidationError) bool {
		if len(v.Causes) == 0 {
			return strings.Join(v.InstanceLocation, "/") == typeLoc
		}
		if _, ok := v.ErrorKind.(*kind.AnyOf); ok { // nested union: fails on type only if every branch does
			for _, c := range v.Causes {
				if !failsOnType(c) {
					return false
				}
			}
			return true
		}
		for _, c := range v.Causes {
			if failsOnType(c) {
				return true
			}
		}
		return false
	}
	var out []*jsonschema.ValidationError
	for _, c := range causes {
		if !failsOnType(c) {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		return causes
	}
	return out
}

// deepestCauses keeps the anyOf branches whose errors reach deepest into the
// instance: for {"op":"median"} that is ".../op: value must be one of ..."
// instead of one line per Expr shape that did not match ("best match").
func deepestCauses(causes []*jsonschema.ValidationError) []*jsonschema.ValidationError {
	var depth func(v *jsonschema.ValidationError) int
	depth = func(v *jsonschema.ValidationError) int {
		d := len(v.InstanceLocation)
		for _, c := range v.Causes {
			d = max(d, depth(c))
		}
		return d
	}
	best := -1
	var out []*jsonschema.ValidationError
	for _, c := range causes {
		switch d := depth(c); {
		case d > best:
			best, out = d, []*jsonschema.ValidationError{c}
		case d == best:
			out = append(out, c)
		}
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
			add("entity %q has too many fields (%d > %d)", e.ID, len(e.Fields), MaxFieldsPerEntity)
		}
		switch e.Scope {
		case "", ScopePrivate, ScopeShared, ScopePublic:
		default:
			add("entity %q scope %q must be private, shared or public", e.ID, e.Scope)
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
			if e.IsShared() && slices.Contains(ReservedRecordFields, f.ID) {
				add("entity %q field %q is reserved on %s entities (appconfig sets it from the signed-in user)", e.ID, f.ID, e.EffectiveScope())
			}
			if f.Type == FieldEnum && len(f.Options) == 0 {
				add("entity %q field %q: enum needs options", e.ID, f.ID)
			}
			// ref: options = initial records of refEntity (their title values, #194)
			if f.Type != FieldEnum && f.Type != FieldRef && len(f.Options) > 0 {
				add("entity %q field %q: options only allowed for enum and ref", e.ID, f.ID)
			}
		}
		if e.TitleField != "" && !seen[e.TitleField] {
			add("entity %q titleField %q does not exist", e.ID, e.TitleField)
		}
	}
	checkComputedFields(a.Entities, entities, DailyToggleFields(a), add)

	notifIDs := map[string]bool{}
	for i := range a.App.Notifications {
		n := &a.App.Notifications[i]
		if notifIDs[n.ID] {
			add("duplicate notification id %q", n.ID)
		}
		notifIDs[n.ID] = true
		checkNotification(fmt.Sprintf("app.notifications[%d]", i), n, add)
		checkRecordNotification(fmt.Sprintf("app.notifications[%d]", i), n, entities, add)
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
		add("initialScreen %q does not exist; set it to one of the screen ids (%s)", a.InitialScreen, screenIDs(a))
	}
	if n := len(a.App.Tabs); n > 0 {
		if n < 2 || n > MaxTabs {
			add("app.tabs must have 2-%d tabs (got %d): add another tab whose screen is a different top-level screen, merge screens to keep at most %d, or remove app.tabs entirely for a single-stack app", MaxTabs, n, MaxTabs)
		}
		seen := map[string]bool{}
		for i, t := range a.App.Tabs {
			if !screens[t.Screen] {
				add("app.tabs[%d]: screen %q does not exist; point the tab at one of the screen ids (%s) or add that screen to screens", i, t.Screen, screenIDs(a))
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
				add("%s: navigate target screen %q does not exist; use one of the screen ids (%s) or add that screen to screens", where, act.Screen, screenIDs(a))
			}
		case ActionSaveRecord, ActionDeleteRecord:
			if entities[act.Entity] == nil {
				add("%s: %s entity %q does not exist", where, act.Type, act.Entity)
			}
			if act.Screen != "" && !screens[act.Screen] {
				add("%s: saveRecord then-screen %q does not exist; use one of the screen ids (%s) or omit screen to close the form after saving", where, act.Screen, screenIDs(a))
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
				if act.Notification.Entity != "" {
					add("%s: record-based notification (entity/field) belongs in app.notifications, not scheduleNotification", where)
				}
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
		searchKeys := collectSearchKeys(s.ID, s.Components, add)
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
				checkContainerProps(where, c, add)
				for j := range c.Children {
					walk(fmt.Sprintf("%s.children[%d]", where, j), &c.Children[j], depth+1)
				}
				return
			}
			if len(c.Children) > 0 {
				add("%s: %s cannot have children", where, c.Type)
			}
			if c.Filter != nil {
				checkComponentFilterValue(where, c.Filter, entities, add)
				checkComponentPeriodFilter(where, c, entities, add)
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
			case CompTextField, CompNumberField, CompDatePicker, CompPicker, CompToggle, CompPhotoField, CompImage, CompVideoField, CompVideoPlayer:
				if formEntity == nil {
					add("%s: %s requires screen.entity", where, c.Type)
				} else if f := formEntity.FieldByID(c.Field); f == nil {
					add("%s: field %q not in entity %q", where, c.Field, formEntity.ID)
				} else if f.Computed != nil && c.Type != CompImage && c.Type != CompVideoPlayer {
					add("%s: field %q is computed (read-only); inputs cannot bind to it. Bind a stored field or show it with list/table/stat", where, c.Field)
				} else if c.Type == CompPicker && f.Type != FieldEnum && f.Type != FieldRef {
					add("%s: picker field %q must be enum or ref", where, c.Field)
				} else if c.Type != CompPicker && f.Type == FieldRef {
					add("%s: field %q is type ref; use a picker to choose the %s record", where, c.Field, f.RefEntity)
				} else if (c.Type == CompPhotoField || c.Type == CompImage) && f.Type != FieldPhoto {
					add("%s: %s field %q must be type photo", where, c.Type, c.Field)
				} else if (c.Type == CompVideoField || c.Type == CompVideoPlayer) && f.Type != FieldVideo {
					add("%s: %s field %q must be type video", where, c.Type, c.Field)
				}
				if c.Type == CompVideoField && (c.MaxDuration < 0 || c.MaxDuration > 600) {
					add("%s: videoField maxDuration must be 1..600 seconds", where)
				}
			case CompSlider, CompStepper, CompRating, CompTextEditor, CompSecureField, CompTimePicker, CompColorPicker:
				checkInputBinding(where, c, formEntity, add)
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
				checkListToggle(where, c.Item, e, add)
				if c.Sort != nil && e.FieldByID(c.Sort.Field) == nil {
					add("%s: sort field %q not in entity %q", where, c.Sort.Field, e.ID)
				}
				if c.Filter != nil && e.FieldByID(c.Filter.Field) == nil {
					add("%s: filter field %q not in entity %q", where, c.Filter.Field, e.ID)
				}
				checkAction(where+".onItemTap", c.OnItemTap)
				if c.SearchField != "" && !searchKeys[c.SearchField] {
					add("%s: searchField %q has no matching searchBar (field) on screen %q", where, c.SearchField, s.ID)
				}
			case CompStat:
				checkStat(where, c, entities, add)
			case CompSettlement:
				checkSettlement(where, c, entities, add)
			case CompChart:
				e := entities[c.Entity]
				if e == nil {
					add("%s: %s entity %q does not exist", where, c.Type, c.Entity)
					break
				}
				if c.Aggregate != "count" && e.FieldByID(c.Field) == nil {
					add("%s: %s field %q not in entity %q (required unless aggregate=count)", where, c.Type, c.Field, e.ID)
				}
				if e.FieldByID(c.GroupBy) == nil {
					add("%s: chart groupBy %q not in entity %q", where, c.GroupBy, e.ID)
				}
				if c.Filter != nil && e.FieldByID(c.Filter.Field) == nil {
					add("%s: filter field %q not in entity %q", where, c.Filter.Field, e.ID)
				}
			case CompVoiceInput, CompSpacer, CompDivider:
			case CompSearchBar:
				if !idRe.MatchString(c.Field) {
					add("%s: searchBar field %q must be an id (screen-local search key)", where, c.Field)
				}
			case CompCalendar:
				e := entities[c.Entity]
				if e == nil {
					add("%s: calendar entity %q does not exist", where, c.Entity)
					break
				}
				if f := e.FieldByID(c.DateField); f == nil {
					add("%s: calendar dateField %q not in entity %q", where, c.DateField, e.ID)
				} else if f.Type != FieldDate && f.Type != FieldDatetime {
					add("%s: calendar dateField %q must be date or datetime", where, c.DateField)
				}
				checkAction(where+".onDateTap", c.OnDateTap)
			case CompShareButton:
				if c.Text == "" {
					add("%s: shareButton needs text", where)
				}
				if !ShareTargets[c.ShareTo] {
					add("%s: shareButton shareTo %q must be one of any|line|slack|discord", where, c.ShareTo)
				}
				if c.Field != "" {
					if formEntity == nil {
						add("%s: shareButton.field requires screen.entity", where)
					} else if formEntity.FieldByID(c.Field) == nil {
						add("%s: field %q not in entity %q", where, c.Field, formEntity.ID)
					}
				}
			case CompOpenURLButton:
				if c.Label == "" || c.URL == "" {
					add("%s: openUrlButton needs label and url", where)
				} else if !isOpenableURL(c.URL) {
					add("%s: openUrlButton url %q must be https or one of the schemes %s", where, c.URL, strings.Join(OpenURLSchemes, ", "))
				}
			case CompWebView:
				if !strings.HasPrefix(c.URL, "https://") {
					add("%s: webView url %q must be an https URL", where, c.URL)
				}
			case CompMapView:
				e := entities[c.Entity]
				if e == nil {
					add("%s: mapView entity %q does not exist", where, c.Entity)
					break
				}
				if f := e.FieldByID(c.LocationField); f == nil {
					add("%s: mapView locationField %q not in entity %q", where, c.LocationField, e.ID)
				} else if f.Type != FieldLocation {
					add("%s: mapView locationField %q must be type location", where, c.LocationField)
				}
				if c.TitleField != "" && e.FieldByID(c.TitleField) == nil {
					add("%s: mapView titleField %q not in entity %q", where, c.TitleField, e.ID)
				}
				checkMapRoute(where, c, e, add)
				checkAction(where+".onItemTap", c.OnItemTap)
			case CompLocationButton, CompQrScanner:
				if formEntity == nil {
					add("%s: %s requires screen.entity", where, c.Type)
				} else if f := formEntity.FieldByID(c.Field); f == nil {
					add("%s: field %q not in entity %q", where, c.Field, formEntity.ID)
				} else if f.Computed != nil {
					add("%s: field %q is computed (read-only); inputs cannot bind to it", where, c.Field)
				} else if c.Type == CompLocationButton && f.Type != FieldLocation {
					add("%s: locationButton field %q must be type location", where, c.Field)
				} else if c.Type == CompQrScanner && f.Type != FieldString && f.Type != FieldText {
					add("%s: qrScanner field %q must be string or text", where, c.Field)
				}
			case CompLocationPicker: // #212
				checkLocationPicker(where, c, formEntity, add)
			case CompTimerDisplay:
				switch c.Mode {
				case "stopwatch":
				case "countdown":
					if c.Seconds <= 0 {
						add("%s: countdown timer needs seconds > 0", where)
					}
				default:
					add("%s: timerDisplay mode must be stopwatch or countdown", where)
				}
				if c.Field != "" {
					if formEntity == nil {
						add("%s: timerDisplay.field requires screen.entity", where)
					} else if f := formEntity.FieldByID(c.Field); f == nil {
						add("%s: field %q not in entity %q", where, c.Field, formEntity.ID)
					} else if (f.Type != FieldInteger && f.Type != FieldNumber) || f.Computed != nil {
						add("%s: timerDisplay field %q must be integer or number (stored, not computed)", where, c.Field)
					}
				}
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
			case CompGridTable:
				checkGridTable(where, c, entities, add)
				checkAction(where+".onItemTap", c.OnItemTap)
			case CompEmptyState:
				if c.Title == "" {
					add("%s: emptyState needs title", where)
				}
			case CompAiButton:
				checkAIButton(where, c, formEntity, entities, add)
			case CompLocationTracker:
				checkLocationTracker(where, c, entities, add)
			case CompHealthStat:
				checkHealthStat(where, c, add)
			case CompNearbyShare:
				if c.Label == "" {
					add("%s: nearbyShare needs label", where)
				}
				if entities[c.Entity] == nil {
					add("%s: nearbyShare entity %q does not exist", where, c.Entity)
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

// checkRecordNotification validates the record-based fields (#228).
func checkRecordNotification(where string, n *Notification, entities map[string]*Entity, add func(string, ...any)) {
	if n.Entity == "" {
		if n.Field != "" || n.TitleField != "" || n.EnabledField != "" || n.OffsetMinutes != 0 {
			add("%s: field/titleField/enabledField/offsetMinutes need entity", where)
		}
		return
	}
	e := entities[n.Entity]
	if e == nil {
		add("%s: notification entity %q does not exist", where, n.Entity)
		return
	}
	f := e.FieldByID(n.Field)
	switch {
	case n.Field == "":
		add("%s: record notification needs field (a datetime / date / HH:MM string field of %q)", where, n.Entity)
	case f == nil:
		add("%s: notification field %q does not exist in entity %q", where, n.Field, n.Entity)
	case f.Type != FieldDatetime && f.Type != FieldDate && f.Type != FieldString:
		add("%s: notification field %q must be datetime, date or string (HH:MM)", where, n.Field)
	}
	if n.TitleField != "" && e.FieldByID(n.TitleField) == nil {
		add("%s: notification titleField %q does not exist in entity %q", where, n.TitleField, n.Entity)
	}
	if n.EnabledField != "" {
		if ef := e.FieldByID(n.EnabledField); ef == nil || ef.Type != FieldBoolean {
			add("%s: notification enabledField %q must be a boolean field of %q", where, n.EnabledField, n.Entity)
		}
	}
	if n.OffsetMinutes < -10080 || n.OffsetMinutes > 10080 {
		add("%s: offsetMinutes %d out of range (±10080)", where, n.OffsetMinutes)
	}
}

var timeRe = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

// checkContainerProps validates props that only make sense for one container
// kind (grid columns range, carousel itemWidth, disclosure expanded).
func checkContainerProps(where string, c *Component, add func(string, ...any)) {
	if c.ColumnCount != 0 {
		if c.Type != CompGrid {
			add("%s: columnCount is only allowed on grid", where)
		} else if c.ColumnCount < MinGridContainerColumns || c.ColumnCount > MaxGridContainerColumns {
			add("%s: grid columnCount must be %d-%d (got %d)", where, MinGridContainerColumns, MaxGridContainerColumns, c.ColumnCount)
		}
	}
	if c.ItemWidth != "" && c.Type != CompCarousel {
		add("%s: itemWidth is only allowed on carousel", where)
	}
	if c.Expanded != nil && c.Type != CompDisclosure {
		add("%s: expanded is only allowed on disclosure", where)
	}
	if (c.Type == CompCarousel || c.Type == CompGrid) && len(c.Children) == 0 {
		add("%s: %s needs at least one child", where, c.Type)
	}
}

// collectSearchKeys returns the `field` keys of every searchBar on a screen
// (top level or inside a container) and reports duplicates.
func collectSearchKeys(screenID string, comps []Component, add func(string, ...any)) map[string]bool {
	keys := map[string]bool{}
	visit := func(c *Component) {
		if c.Type != CompSearchBar || c.Field == "" {
			return
		}
		if keys[c.Field] {
			add("screen %q: duplicate searchBar field %q", screenID, c.Field)
		}
		keys[c.Field] = true
	}
	for i := range comps {
		visit(&comps[i])
		for j := range comps[i].Children {
			visit(&comps[i].Children[j])
		}
	}
	return keys
}

func isHTTPURL(s string) bool {
	return strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://")
}

// ShareTargets are the shareButton.shareTo presets ("" = any, the OS share sheet).
var ShareTargets = map[string]bool{"": true, "any": true, "line": true, "slack": true, "discord": true}

// OpenURLSchemes is the allowlist of non-https schemes an openUrlButton may
// launch (other apps / system handlers). Keep in sync with the iOS
// ExternalURLPolicy and Info.plist LSApplicationQueriesSchemes.
var OpenURLSchemes = []string{"line", "slack", "discord", "tel", "mailto", "maps", "comgooglemaps"}

// isOpenableURL accepts https URLs and allowlisted custom-scheme URLs.
func isOpenableURL(s string) bool {
	if strings.HasPrefix(s, "https://") {
		return true
	}
	scheme, rest, ok := strings.Cut(s, ":")
	if !ok || rest == "" {
		return false
	}
	for _, allowed := range OpenURLSchemes {
		if strings.EqualFold(scheme, allowed) {
			return true
		}
	}
	return false
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
	if c.Tone != "" && c.Tone != "positive" && c.Tone != "negative" {
		add("%s: progressBar tone %q must be positive (goal) or negative (budget)", where, c.Tone)
	}
	if c.Value != nil {
		if v, ok := c.Value.Number(); !ok {
			add("%s: progressBar value must be a number literal 0..1 (expressions are only for stat.value)", where)
		} else if v < 0 || v > 1 {
			add("%s: progressBar value %v must be within 0..1", where, v)
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
		if _, ok := c.Value.Number(); !ok {
			add("%s: gauge value must be a number literal (expressions are only for stat.value)", where)
		}
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

// inputFieldTypes lists the Field types each input-batch component may bind to.
var inputFieldTypes = map[ComponentType][]FieldType{
	CompSlider:      {FieldInteger, FieldNumber, FieldCurrency},
	CompStepper:     {FieldInteger, FieldNumber, FieldCurrency},
	CompRating:      {FieldInteger, FieldNumber},
	CompTextEditor:  {FieldString, FieldText},
	CompSecureField: {FieldString, FieldText},
	CompTimePicker:  {FieldString, FieldText, FieldDatetime},
	CompColorPicker: {FieldString, FieldText},
}

// checkInputBinding validates field reference + type and the numeric range
// props (min/max/step) of the input-batch components.
func checkInputBinding(where string, c *Component, formEntity *Entity, add func(string, ...any)) {
	if formEntity == nil {
		add("%s: %s requires screen.entity", where, c.Type)
		return
	}
	f := formEntity.FieldByID(c.Field)
	if f == nil {
		add("%s: field %q not in entity %q", where, c.Field, formEntity.ID)
		return
	}
	if f.Computed != nil {
		add("%s: field %q is computed (read-only); inputs cannot bind to it", where, c.Field)
	}
	allowed := inputFieldTypes[c.Type]
	ok := false
	for _, t := range allowed {
		if f.Type == t {
			ok = true
			break
		}
	}
	if !ok {
		add("%s: %s field %q must be one of %v (got %s)", where, c.Type, c.Field, allowed, f.Type)
	}
	switch c.Type {
	case CompSlider:
		if c.Min == nil || c.Max == nil {
			add("%s: slider needs min and max", where)
		} else if *c.Min >= *c.Max {
			add("%s: slider min (%v) must be < max (%v)", where, *c.Min, *c.Max)
		}
		if c.Step != nil && *c.Step <= 0 {
			add("%s: slider step must be > 0", where)
		}
	case CompStepper:
		if c.Min != nil && c.Max != nil && *c.Min >= *c.Max {
			add("%s: stepper min (%v) must be < max (%v)", where, *c.Min, *c.Max)
		}
		if c.Step != nil && *c.Step <= 0 {
			add("%s: stepper step must be > 0", where)
		}
	case CompRating:
		if c.Max != nil && (*c.Max < 1 || *c.Max > 10 || *c.Max != float64(int(*c.Max))) {
			add("%s: rating max must be an integer 1..10", where)
		}
	case CompTextEditor:
		if c.MinLines < 0 {
			add("%s: textEditor minLines must be >= 0", where)
		}
	}
}
