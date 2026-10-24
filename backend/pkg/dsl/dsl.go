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
// appconfig under apps/{appId}/entities/{entityId}/records.
type Entity struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	TitleField string  `json:"titleField,omitempty"`
	Fields     []Field `json:"fields"`
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
	FieldLocation FieldType = "location" // "lat,lng"
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
	CompPhotoField  ComponentType = "photoField" // camera / library -> photo field
	CompImage       ComponentType = "image"      // displays a photo field
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
	CompShareButton    ComponentType = "shareButton"    // ShareLink
	CompOpenURLButton  ComponentType = "openUrlButton"  // fixed URL link
	CompMapView        ComponentType = "mapView"        // MapKit pins from a location field
	CompLocationButton ComponentType = "locationButton" // CoreLocation -> location field
	CompQrScanner      ComponentType = "qrScanner"      // AVFoundation -> string field
	CompTimerDisplay   ComponentType = "timerDisplay"   // stopwatch / countdown
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
	Source      string  `json:"source,omitempty"` // photoField: camera|library|both
	Height      string  `json:"height,omitempty"` // image: small|medium|large

	// data-bound (list / stat / chart)
	Entity        string    `json:"entity,omitempty"`
	Item          *ListItem `json:"item,omitempty"`
	Sort          *Sort     `json:"sort,omitempty"`
	Filter        *Filter   `json:"filter,omitempty"`
	EmptyText     string    `json:"emptyText,omitempty"`
	OnItemTap     *Action   `json:"onItemTap,omitempty"`
	SwipeToDelete bool      `json:"swipeToDelete,omitempty"`
	Aggregate     string    `json:"aggregate,omitempty"` // count|sum|avg|min|max
	Format        string    `json:"format,omitempty"`    // plain|currency|percent
	Kind          string    `json:"kind,omitempty"`      // bar|line|pie
	GroupBy       string    `json:"groupBy,omitempty"`
	Bucket        string    `json:"bucket,omitempty"` // day|week|month

	// voiceInput / spacer
	Prompt string `json:"prompt,omitempty"`
	Size   string `json:"size,omitempty"`

	// structure containers
	Expanded  *bool  `json:"expanded,omitempty"`  // disclosure
	ItemWidth string `json:"itemWidth,omitempty"` // carousel: small|medium|large
	Columns   int    `json:"columns,omitempty"`   // grid: 2..4

	// nav / native leaves
	SearchField   string  `json:"searchField,omitempty"`   // list: searchBar.field on the same screen
	DateField     string  `json:"dateField,omitempty"`     // calendar
	OnDateTap     *Action `json:"onDateTap,omitempty"`     // calendar
	URL           string  `json:"url,omitempty"`           // openUrlButton
	LocationField string  `json:"locationField,omitempty"` // mapView
	TitleField    string  `json:"titleField,omitempty"`    // mapView pin label
	Zoom          string  `json:"zoom,omitempty"`          // mapView: near|city|wide
	Mode          string  `json:"mode,omitempty"`          // timerDisplay: stopwatch|countdown
	Seconds       int     `json:"seconds,omitempty"`       // timerDisplay countdown start
}

// ListItem maps record fields onto a list row.
type ListItem struct {
	Title    string `json:"title"`
	Subtitle string `json:"subtitle,omitempty"`
	Trailing string `json:"trailing,omitempty"`
	Icon     string `json:"icon,omitempty"`
	Image    string `json:"image,omitempty"` // photo field id -> thumbnail
}

// Sort orders a list.
type Sort struct {
	Field     string `json:"field"`
	Direction string `json:"direction"` // asc|desc
}

// Filter narrows records for list/stat/chart.
type Filter struct {
	Field string `json:"field"`
	Op    string `json:"op"` // eq|neq|gt|gte|lt|lte|thisMonth|today
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
