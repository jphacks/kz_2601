// Package dsl holds the Server-Driven UI schema (v1, unified with iOS) shared by all services.
//
// The canonical schema is docs/dsl/dsl.schema.json (copied to schema.json
// in this package and embedded). Go types mirror it 1:1; Component is a single
// flattened struct discriminated by Type so that JSON round-trips losslessly
// and the human backend dev does not need custom (un)marshalers.
//
// Design rule: the tree is intentionally non-recursive
// (Screen > Container > Leaf). Claude structured outputs reject recursive
// schemas, and iOS gets a bounded renderer.
package dsl

// SchemaVersion is the only accepted value of App.SchemaVersion (v1).
const SchemaVersion = 1

// App is the root document (one per app version) stored in Firestore.
type App struct {
	SchemaVersion int      `json:"schemaVersion"`
	App           AppMeta  `json:"app"`
	InitialScreen string   `json:"initialScreen"`
	Entities      []Entity `json:"entities"`
	Screens       []Screen `json:"screens"`
}

// AppMeta describes the whole app.
type AppMeta struct {
	Name          string         `json:"name"`
	Description   string         `json:"description,omitempty"`
	Icon          string         `json:"icon,omitempty"`
	Theme         *Theme         `json:"theme,omitempty"`
	VoiceEntry    VoiceEntry     `json:"voiceEntry"`
	Notifications []Notification `json:"notifications,omitempty"` // iOS re-syncs local notifications on every version
	Tabs          []Tab          `json:"tabs,omitempty"`          // optional bottom tab bar (2-5); each tab = own nav stack
}

// Tab is one bottom-bar tab rooted at a screen.
type Tab struct {
	Screen string `json:"screen"`
	Title  string `json:"title"`
	Icon   string `json:"icon,omitempty"`
}

// Notification is a local notification (P0 native feature).
type Notification struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	Repeat  string `json:"repeat"`            // once|daily|weekly
	Time    string `json:"time,omitempty"`    // HH:MM local
	Weekday string `json:"weekday,omitempty"` // mon..sun (weekly)
	// Record-based (#228, app.notifications only): one notification per record
	// of Entity at the time in Field (datetime / date / "HH:MM" string),
	// shifted by OffsetMinutes (negative = before).
	Entity        string `json:"entity,omitempty"`
	Field         string `json:"field,omitempty"`
	OffsetMinutes int    `json:"offsetMinutes,omitempty"`
	TitleField    string `json:"titleField,omitempty"`   // record field shown as the body
	EnabledField  string `json:"enabledField,omitempty"` // boolean field; false = skip the record
}

// VoiceEntry is how the user re-enters voice mode.
type VoiceEntry string

const (
	VoiceEntryFullscreen VoiceEntry = "fullscreen"
	VoiceEntryFloating   VoiceEntry = "floating"
	VoiceEntryNone       VoiceEntry = "none"
)

// Theme is a minimal theme hint; iOS owns the rest.
type Theme struct {
	PrimaryColor string `json:"primaryColor,omitempty"`
	Mode         string `json:"mode,omitempty"` // system|light|dark
}

// Entity is a generic data model (e.g. 家計簿の支出). Records are stored by
// appconfig under users/{uid}/apps/{appId}/entities/{entityId}/records
// (private) or apps_public/{appId}/entities/{entityId}/records (shared/public).
type Entity struct {
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	TitleField string      `json:"titleField,omitempty"`
	Scope      EntityScope `json:"scope,omitempty"` // "" = private
	Edit       EntityEdit  `json:"edit,omitempty"`  // "" = author (shared/public only)
	Fields     []Field     `json:"fields"`
}

// EntityEdit says who may update a shared/public record. Deleting is always
// the author's alone.
type EntityEdit string

const (
	EditAuthor  EntityEdit = "author"  // only the record's author (default)
	EditMembers EntityEdit = "members" // anyone who opened the app (#252)
)

// MembersCanEdit reports whether non-authors may PATCH this entity's records.
// Private entities ignore edit.
func (e *Entity) MembersCanEdit() bool {
	return e.IsShared() && e.Edit == EditMembers
}

// EntityScope says who shares an entity's records.
type EntityScope string

const (
	ScopePrivate EntityScope = "private" // owner only (default)
	ScopeShared  EntityScope = "shared"  // creator + people who joined by share link (#388)
	ScopePublic  EntityScope = "public"  // anyone can read; signed-in users can post
)

// ReservedRecordFields are set by appconfig on shared/public records and
// therefore cannot be user-defined field ids on such entities.
var ReservedRecordFields = []string{"author", "authorName"}

// EffectiveScope resolves the default.
func (e *Entity) EffectiveScope() EntityScope {
	if e.Scope == "" {
		return ScopePrivate
	}
	return e.Scope
}

// IsShared reports whether records live in the app-wide (uid-independent)
// collection. shared and public are the same in v1.
func (e *Entity) IsShared() bool {
	s := e.EffectiveScope()
	return s == ScopeShared || s == ScopePublic
}

// FieldType is the closed set of record field types.
type FieldType string

const (
	FieldString   FieldType = "string"
	FieldText     FieldType = "text"
	FieldInteger  FieldType = "integer"
	FieldNumber   FieldType = "number"
	FieldBoolean  FieldType = "boolean"
	FieldDate     FieldType = "date"
	FieldDatetime FieldType = "datetime"
	FieldEnum     FieldType = "enum"
	FieldCurrency FieldType = "currency"
	FieldPhoto    FieldType = "photo"    // https URL uploaded by iOS to Firebase Storage
	FieldVideo    FieldType = "video"    // "video:<id>|data:image/jpeg;base64,<thumb>" (movie stays on the device)
	FieldLocation FieldType = "location" // "lat,lng"
	FieldRef      FieldType = "ref"      // id of a record of Field.RefEntity
	FieldRefs     FieldType = "refs"     // ids of records of Field.RefEntity (multi-select, #231); [] = everyone
)

// Field is one column of an Entity.
type Field struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Type         FieldType `json:"type"`
	Required     bool      `json:"required,omitempty"`
	Options      []string  `json:"options,omitempty"`
	DefaultValue any       `json:"defaultValue,omitempty"` // string|number|bool|nil; "now" for date types
	Unit         string    `json:"unit,omitempty"`
	RefEntity    string    `json:"refEntity,omitempty"` // type=ref/refs: target entity id
	Computed     *Expr     `json:"computed,omitempty"`  // read-only calculated field (row context = this record)
	Health       string    `json:"health,omitempty"`    // "weight": also write the saved value to HealthKit (#167)
	Hidden       bool      `json:"hidden,omitempty"`    // not shown to answerers (quiz answer key); computed may compare it (#262)
}

// Screen is one navigable page.
type Screen struct {
	ID         string          `json:"id"`
	Title      string          `json:"title"`
	Entity     string          `json:"entity,omitempty"` // form screen over this entity
	Toolbar    []ToolbarButton `json:"toolbar,omitempty"`
	Components []Component     `json:"components"`
}

// ToolbarButton is a navigation-bar button.
type ToolbarButton struct {
	Label  string `json:"label"`
	Icon   string `json:"icon,omitempty"`
	Action Action `json:"action"`
}

// ActionType is the closed set of actions.
type ActionType string

const (
	ActionNavigate      ActionType = "navigate"
	ActionBack          ActionType = "back"
	ActionSaveRecord    ActionType = "saveRecord"
	ActionDeleteRecord  ActionType = "deleteRecord"
	ActionOpenVoice     ActionType = "openVoice"
	ActionOpenURL       ActionType = "openUrl"
	ActionResetForm     ActionType = "resetForm"
	ActionScheduleNotif ActionType = "scheduleNotification"
	ActionCancelNotif   ActionType = "cancelNotification"
)

// Action is a non-nesting side effect triggered by buttons/list taps.
type Action struct {
	Type           ActionType    `json:"type"`
	Screen         string        `json:"screen,omitempty"`
	Presentation   string        `json:"presentation,omitempty"` // push|sheet|fullScreen
	Entity         string        `json:"entity,omitempty"`
	URL            string        `json:"url,omitempty"`
	ConfirmMessage string        `json:"confirmMessage,omitempty"`
	Notification   *Notification `json:"notification,omitempty"`   // scheduleNotification
	NotificationID string        `json:"notificationId,omitempty"` // cancelNotification
	PrefillRef     string        `json:"prefillRef,omitempty"`     // navigate: ref field of the target form set to the current record (#256)
}

// ComponentType discriminates Component.
type ComponentType string

const (
	// containers (children = leaves only)
	CompVStack  ComponentType = "vstack"
	CompHStack  ComponentType = "hstack"
	CompSection ComponentType = "section"
	// leaves
	CompText        ComponentType = "text"
	CompButton      ComponentType = "button"
	CompTextField   ComponentType = "textField"
	CompNumberField ComponentType = "numberField"
	CompDatePicker  ComponentType = "datePicker"
	CompPicker      ComponentType = "picker"
	CompToggle      ComponentType = "toggle"
	CompPhotoField  ComponentType = "photoField"  // camera / library -> photo field
	CompImage       ComponentType = "image"       // displays a photo field
	CompVideoField  ComponentType = "videoField"  // camera (front/rear) / library -> video field
	CompVideoPlayer ComponentType = "videoPlayer" // plays a video field
	CompList        ComponentType = "list"
	CompStat        ComponentType = "stat"
	CompChart       ComponentType = "chart"
	CompVoiceInput  ComponentType = "voiceInput"
	CompSpacer      ComponentType = "spacer"
	CompDivider     ComponentType = "divider"
	// structure containers (batch: structure/nav/native)
	CompCard       ComponentType = "card"       // rounded surface + shadow
	CompDisclosure ComponentType = "disclosure" // collapsible group
	CompCarousel   ComponentType = "carousel"   // horizontal strip
	CompGrid       ComponentType = "grid"       // 2-4 equal columns
	// nav / native leaves
	CompSearchBar      ComponentType = "searchBar"      // screen-local search text -> list.searchField
	CompCalendar       ComponentType = "calendar"       // month view over entity.dateField
	CompGridTable      ComponentType = "gridTable"      // timetable: columnField x rowField cells (#234)
	CompShareButton    ComponentType = "shareButton"    // ShareLink
	CompOpenURLButton  ComponentType = "openUrlButton"  // fixed URL link
	CompMapView        ComponentType = "mapView"        // MapKit pins from a location field
	CompLocationButton ComponentType = "locationButton" // CoreLocation -> location field
	CompLocationPicker ComponentType = "locationPicker" // search / map pin -> location field (+ nameField) (#212)
	CompQrScanner      ComponentType = "qrScanner"      // AVFoundation -> string field
	CompTimerDisplay   ComponentType = "timerDisplay"   // stopwatch / countdown
	CompWebView        ComponentType = "webView"        // in-app WKWebView (https only)
	// leaves: input batch
	CompSlider      ComponentType = "slider"      // bounded numeric
	CompStepper     ComponentType = "stepper"     // -/+ numeric
	CompTextEditor  ComponentType = "textEditor"  // multi-line text
	CompSecureField ComponentType = "secureField" // masked text
	CompTimePicker  ComponentType = "timePicker"  // "HH:mm" (string) or datetime
	CompColorPicker ComponentType = "colorPicker" // "#RRGGBB" string
	CompRating      ComponentType = "rating"      // 1..max stars -> numeric
	// display leaves
	CompLabel       ComponentType = "label"
	CompIcon        ComponentType = "icon"
	CompProgressBar ComponentType = "progressBar"
	CompGauge       ComponentType = "gauge"
	CompBadge       ComponentType = "badge"
	CompLink        ComponentType = "link"
	CompMarkdown    ComponentType = "markdown"
	CompAvatar      ComponentType = "avatar"
	CompTable       ComponentType = "table"
	CompTimeline    ComponentType = "timeline"
	CompEmptyState  ComponentType = "emptyState"
	// logic
	CompSettlement ComponentType = "settlement" // greedy "A pays B" list from a balance field
	// random pick (#263): one record per button press, not saved
	CompRandomPicker ComponentType = "randomPicker"
	// on-device AI (#78)
	CompAiButton ComponentType = "aiButton" // Foundation Models -> form field (never leaves the device)
	// peer-to-peer
	CompNearbyShare ComponentType = "nearbyShare" // MultipeerConnectivity: send records to a nearby device
	// location tracking (#168): start/stop recording points while the user opts in
	CompLocationTracker ComponentType = "locationTracker"
	// device health (#167)
	CompHealthStat ComponentType = "healthStat" // CoreMotion steps / HealthKit weight (no entity)
	// device sensors (#419): CoreMotion / CMAltimeter live value, optional save
	CompSensorValue ComponentType = "sensorValue"
)

// ContainerTypes lists component types that may have Children.
var ContainerTypes = map[ComponentType]bool{
	CompVStack: true, CompHStack: true, CompSection: true,
	CompCard: true, CompDisclosure: true, CompCarousel: true, CompGrid: true,
}

// Grid container column range.
const (
	MinGridContainerColumns = 2
	MaxGridContainerColumns = 4
)

// Component is a flattened union of all component kinds. Which fields are
// meaningful depends on Type (see docs/dsl/README.md).
type Component struct {
	Type ComponentType `json:"type"`
	ID   string        `json:"id,omitempty"`
	// Area places the component on the screen grid ("A1:C3", columns A-F,
	// rows of visibleHeight/12). Only direct children of a Screen; siblings
	// must not overlap. Empty = stacked below the grid (fallback).
	Area string `json:"area,omitempty"`

	// container
	Title    string      `json:"title,omitempty"` // section, chart
	Spacing  string      `json:"spacing,omitempty"`
	Children []Component `json:"children,omitempty"`

	// text / button / inputs
	Text        string  `json:"text,omitempty"`
	Style       string  `json:"style,omitempty"` // text style | button style | picker style
	Align       string  `json:"align,omitempty"`
	Label       string  `json:"label,omitempty"`
	Icon        string  `json:"icon,omitempty"`
	Action      *Action `json:"action,omitempty"`
	Field       string  `json:"field,omitempty"` // input binding / stat & chart value field
	Placeholder string  `json:"placeholder,omitempty"`
	Multiline   bool    `json:"multiline,omitempty"`
	Unit        string  `json:"unit,omitempty"`
	IncludeTime bool    `json:"includeTime,omitempty"`
	Source      string  `json:"source,omitempty"`      // photoField / videoField: camera|library|both
	MaxDuration int     `json:"maxDuration,omitempty"` // videoField: recording limit in seconds (default 60)
	Height      string  `json:"height,omitempty"`      // image: small|medium|large

	// data-bound (list / stat / chart)
	Entity        string    `json:"entity,omitempty"`
	Item          *ListItem `json:"item,omitempty"`
	Sort          *Sort     `json:"sort,omitempty"`
	Filter        *Filter   `json:"filter,omitempty"`
	EmptyText     string    `json:"emptyText,omitempty"`
	OnItemTap     *Action   `json:"onItemTap,omitempty"`
	SwipeToDelete bool      `json:"swipeToDelete,omitempty"`
	Aggregate     string    `json:"aggregate,omitempty"` // count|sum|avg|min|max (stat also latest|earliest)
	By            string    `json:"by,omitempty"`        // stat latest/earliest: date field to order by
	Format        string    `json:"format,omitempty"`    // plain|currency|percent
	Kind          string    `json:"kind,omitempty"`      // bar|line|pie
	GroupBy       string    `json:"groupBy,omitempty"`
	Bucket        string    `json:"bucket,omitempty"` // day|week|month

	// voiceInput / spacer
	Prompt string `json:"prompt,omitempty"`
	Size   string `json:"size,omitempty"`

	// structure containers
	Expanded    *bool  `json:"expanded,omitempty"`    // disclosure
	ItemWidth   string `json:"itemWidth,omitempty"`   // carousel: small|medium|large
	ColumnCount int    `json:"columnCount,omitempty"` // grid: 2..4 (`columns` is the table column list)

	// nav / native leaves (URL / DateField / TitleField are shared with link / timeline below)
	SearchField   string  `json:"searchField,omitempty"`   // list: searchBar.field on the same screen
	OnDateTap     *Action `json:"onDateTap,omitempty"`     // calendar
	LocationField string  `json:"locationField,omitempty"` // mapView
	Zoom          string  `json:"zoom,omitempty"`          // mapView: near|city|wide
	Mode          string  `json:"mode,omitempty"`          // timerDisplay: stopwatch|countdown
	Seconds       int     `json:"seconds,omitempty"`       // timerDisplay countdown start / locationTracker auto stop
	ShareTo       string  `json:"shareTo,omitempty"`       // shareButton: any|line|slack|discord (`target` is progressBar's number)
	// input batch (slider / stepper / rating / textEditor). Pointers so that
	// an explicit 0 round-trips through omitempty. Min/Max are shared with gauge.
	Min      *float64 `json:"min,omitempty"`      // slider / stepper / gauge
	Max      *float64 `json:"max,omitempty"`      // rating: star count (default 5) / gauge
	Step     *float64 `json:"step,omitempty"`     // slider: omit = continuous; stepper: default 1
	MinLines int      `json:"minLines,omitempty"` // textEditor: visible lines (default 4)

	// display (label / icon / progressBar / gauge / badge / link / markdown / avatar / table / timeline / emptyState)
	Name          string        `json:"name,omitempty"`          // icon: SF Symbols name / avatar: initials source
	Color         string        `json:"color,omitempty"`         // SemanticColor enum
	Value         *Expr         `json:"value,omitempty"`         // stat: Expr / progressBar (0..1) & gauge: number literal
	Target        *float64      `json:"target,omitempty"`        // progressBar entity mode denominator
	Tone          string        `json:"tone,omitempty"`          // progressBar: positive (goal) | negative (budget)
	URL           string        `json:"url,omitempty"`           // link
	Image         string        `json:"image,omitempty"`         // avatar literal (https / data URL)
	Message       string        `json:"message,omitempty"`       // emptyState
	Columns       []TableColumn `json:"columns,omitempty"`       // table
	Limit         int           `json:"limit,omitempty"`         // table / timeline
	DateField     string        `json:"dateField,omitempty"`     // timeline
	TitleField    string        `json:"titleField,omitempty"`    // timeline
	SubtitleField string        `json:"subtitleField,omitempty"` // timeline

	// settlement / locationPicker (#212: place name string field)
	NameField    string `json:"nameField,omitempty"`
	BalanceField string `json:"balanceField,omitempty"`

	// aiButton (#78). Label / Prompt / Entity / Limit are shared with the fields above.
	Task    string   `json:"task,omitempty"`    // summarize|classify|rewrite|extract|suggest
	Input   string   `json:"input,omitempty"`   // form field fed to the model ("" = whole draft)
	Output  string   `json:"output,omitempty"`  // form field the result is written to
	Options []string `json:"options,omitempty"` // classify candidates (else the enum output field's options)

	// healthStat (#167). Label / Target are shared.
	Metric string `json:"metric,omitempty"` // steps|weight
	Period string `json:"period,omitempty"` // today|week (default today)

	// gridTable (#234). Entity / TitleField / SubtitleField / OnItemTap are shared.
	ColumnField  string     `json:"columnField,omitempty"`  // x axis (e.g. weekday enum)
	RowField     string     `json:"rowField,omitempty"`     // y axis (e.g. period integer)
	ColumnValues []string   `json:"columnValues,omitempty"` // explicit column headers (else enum options)
	RowValues    []string   `json:"rowValues,omitempty"`    // explicit row headers for enum/string rowField
	RowRange     *GridRange `json:"rowRange,omitempty"`     // integer rowField range (default 1..6)

	// sensorValue (#419). Label / Style / Min / Max / Entity / Field / DateField /
	// Seconds (log interval) / Limit (log count) are shared.
	Sensor    string           `json:"sensor,omitempty"`
	Threshold *SensorThreshold `json:"threshold,omitempty"`
}

// GridRange is an inclusive integer range of gridTable rows.
type GridRange struct {
	From int `json:"from"`
	To   int `json:"to"`
}

// HealthMetrics / HealthPeriods are the closed sets of healthStat values;
// HealthFieldKinds the Field.health values.
var (
	HealthMetrics    = map[string]bool{"steps": true, "weight": true}
	HealthPeriods    = map[string]bool{"": true, "today": true, "week": true}
	HealthFieldKinds = map[string]bool{"weight": true}
)

// AITasks is the closed set of aiButton.task values.
var AITasks = map[string]bool{"summarize": true, "classify": true, "rewrite": true, "extract": true, "suggest": true}

// TableColumn is one column of a table component.
type TableColumn struct {
	Field     string `json:"field"`
	Title     string `json:"title,omitempty"`
	Aggregate string `json:"aggregate,omitempty"` // with table.groupBy: count|sum|avg|min|max (#261)
}

// ListItem maps record fields onto a list row.
type ListItem struct {
	Title    string `json:"title"`
	Subtitle string `json:"subtitle,omitempty"`
	Trailing string `json:"trailing,omitempty"`
	Icon     string `json:"icon,omitempty"`
	Image    string `json:"image,omitempty"`  // photo field id -> thumbnail
	Toggle   string `json:"toggle,omitempty"` // boolean field id -> one-tap check circle (#108)
	Resets   string `json:"resets,omitempty"` // "daily": toggle is a date field, checked = today (#132)
}

// Sort orders a list.
type Sort struct {
	Field     string `json:"field"`
	Direction string `json:"direction"` // asc|desc
}

// Filter narrows records for list/stat/chart.
type Filter struct {
	Field string `json:"field"`
	Op    string `json:"op"` // eq|neq|gt|gte|lt|lte|today|thisWeek|thisMonth|thisYear
	Value any    `json:"value"`
}

// Initial returns the seed DSL every new user starts from (voice screen only).
func Initial() App {
	return App{
		SchemaVersion: SchemaVersion,
		App: AppMeta{
			Name:        "はじめてのアプリ",
			Description: "話しかけるとアプリに変わる初期状態",
			Icon:        "waveform",
			VoiceEntry:  VoiceEntryFullscreen,
		},
		InitialScreen: "home",
		Entities:      []Entity{},
		Screens: []Screen{{
			ID:    "home",
			Title: "",
			Components: []Component{
				{Type: CompVoiceInput, ID: "mic", Area: "A1:F12", Prompt: "どんなアプリが欲しい？", Size: "large"},
			},
		}},
	}
}

// HasSharedEntity reports whether any entity is shared/public (appconfig then
// also publishes the DSL under apps_public/{appId} so other users can open it).
func (a *App) HasSharedEntity() bool {
	for i := range a.Entities {
		if a.Entities[i].IsShared() {
			return true
		}
	}
	return false
}

// EntityByID returns the entity or nil.
func (a *App) EntityByID(id string) *Entity {
	for i := range a.Entities {
		if a.Entities[i].ID == id {
			return &a.Entities[i]
		}
	}
	return nil
}

// FieldByID returns the field or nil.
func (e *Entity) FieldByID(id string) *Field {
	for i := range e.Fields {
		if e.Fields[i].ID == id {
			return &e.Fields[i]
		}
	}
	return nil
}
