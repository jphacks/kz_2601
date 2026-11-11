// Package paid is the list of Tailor It Pro features (paid_features.json) and
// the DSL checks the servers enforce with it (#77, #316).
//
// A feature is either gated at save (gate "save": an app that has it may be
// saved only by a Pro creator; ai's chat marks it requiresPro before
// generating) or at use (gate "use": the app may have it, the paid call
// answers 402). docs/backend/features/pro.md
package paid

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Gates.
const (
	GateSave = "save"
	GateUse  = "use"
)

// Feature ids referenced by code.
const (
	SharedData      = "sharedData"
	ServerRules     = "serverRules"
	OnlineAI        = "onlineAI"
	EncryptedFields = "encryptedFields"
)

// ProChecker reports whether uid has Tailor It Pro (entitlement.ProChecker).
type ProChecker interface {
	IsPro(ctx context.Context, uid string) bool
}

//go:embed paid_features.json
var raw []byte

// Localized is a ja / en string pair.
type Localized struct {
	JA string `json:"ja"`
	EN string `json:"en"`
}

// In returns the English text for lang "en" (when set), Japanese otherwise.
func (l Localized) In(lang string) string {
	if lang == "en" && l.EN != "" {
		return l.EN
	}
	return l.JA
}

// Def is one entry of paid_features.json.
type Def struct {
	ID                 string    `json:"id"`
	Enabled            bool      `json:"enabled"`
	Gate               string    `json:"gate"` // save (default) | use
	Title              Localized `json:"title"`
	Reason             Localized `json:"reason"`
	Keywords           []string  `json:"keywords"`
	ExcludeKeywords    []string  `json:"excludeKeywords"`
	DSLEntityScopes    []string  `json:"dslEntityScopes"`
	DSLServerRules     bool      `json:"dslServerRules"`
	DSLComponents      []string  `json:"dslComponents"`
	DSLEncryptedFields bool      `json:"dslEncryptedFields"`
	WithoutInstruction Localized `json:"withoutInstruction"`
}

// SaveGated reports whether the feature is checked when a DSL is saved.
func (d Def) SaveGated() bool { return d.Gate == "" || d.Gate == GateSave }

// Feature is one paid feature in an API body (openapi `PaidFeature`).
type Feature struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Reason string `json:"reason"`
}

// Public returns the API view in lang.
func (d Def) Public(lang string) Feature {
	return Feature{ID: d.ID, Title: d.Title.In(lang), Reason: d.Reason.In(lang)}
}

// Catalog is a parsed paid_features.json (enabled entries only).
type Catalog struct {
	defs []Def
}

// Default is the embedded list.
var Default = mustParse(raw)

func mustParse(b []byte) *Catalog {
	c, err := Parse(b)
	if err != nil {
		panic(err)
	}
	return c
}

// Parse reads a paid_features.json document; disabled entries are dropped.
func Parse(b []byte) (*Catalog, error) {
	var doc struct {
		Features []Def `json:"features"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("paid_features.json: %w", err)
	}
	c := &Catalog{}
	seen := map[string]bool{}
	for _, f := range doc.Features {
		if f.ID == "" || seen[f.ID] {
			return nil, fmt.Errorf("paid_features.json: empty or duplicate id %q", f.ID)
		}
		if f.Gate != "" && f.Gate != GateSave && f.Gate != GateUse {
			return nil, fmt.Errorf("paid_features.json: %s: gate must be save|use, got %q", f.ID, f.Gate)
		}
		seen[f.ID] = true
		if f.Enabled {
			c.defs = append(c.defs, f)
		}
	}
	return c, nil
}

// Defs returns the enabled entries.
func (c *Catalog) Defs() []Def { return c.defs }

// ByID returns the enabled entry id.
func (c *Catalog) ByID(id string) (Def, bool) {
	for _, d := range c.defs {
		if d.ID == id {
			return d, true
		}
	}
	return Def{}, false
}

// Uses reports whether app a has feature d (by its DSL markers).
func (d Def) Uses(a *dsl.App) bool {
	if a == nil {
		return false
	}
	if d.DSLServerRules && len(a.ServerRules) > 0 {
		return true
	}
	if d.DSLEncryptedFields && a.HasEncryptedFields() {
		return true
	}
	for i := range a.Entities {
		if slices.Contains(d.DSLEntityScopes, string(a.Entities[i].Scope)) {
			return true
		}
	}
	if len(d.DSLComponents) > 0 {
		for i := range a.Screens {
			if hasComponent(a.Screens[i].Components, d.DSLComponents) {
				return true
			}
		}
	}
	return false
}

func hasComponent(cs []dsl.Component, types []string) bool {
	for i := range cs {
		if slices.Contains(types, string(cs[i].Type)) || hasComponent(cs[i].Children, types) {
			return true
		}
	}
	return false
}

// InApp returns the save-gated features app a has.
func (c *Catalog) InApp(a *dsl.App, lang string) []Feature {
	var out []Feature
	for _, d := range c.defs {
		if d.SaveGated() && d.Uses(a) {
			out = append(out, d.Public(lang))
		}
	}
	return out
}

// Added returns the save-gated features next has and prev does not (prev nil
// = a new app). Saving next needs Pro when this is not empty; an app that
// already had a feature keeps it (a lapsed Pro can still edit the app).
func (c *Catalog) Added(next, prev *dsl.App, lang string) []Feature {
	var out []Feature
	for _, d := range c.defs {
		if d.SaveGated() && d.Uses(next) && !d.Uses(prev) {
			out = append(out, d.Public(lang))
		}
	}
	return out
}

// IDs lists the ids of fs.
func IDs(fs []Feature) []string {
	out := make([]string, len(fs))
	for i, f := range fs {
		out[i] = f.ID
	}
	return out
}

// Mode is PRO_GATE_MODE: how hard the paid features are enforced (#316), so
// the gate can be rolled out in steps. ai, appconfig and runner read the same
// variable.
type Mode string

// Modes.
const (
	// ModeEnforce (default): 402 pro_required / the runner skips (skipped_plan).
	ModeEnforce Mode = "enforce"
	// ModeLog: nothing is stopped; what would have been is logged as
	// pro_gate.would_block.
	ModeLog Mode = "log"
	// ModeOff: no check at all (the behavior before #316).
	ModeOff Mode = "off"
)

// ModeEnv is the environment variable of Mode.
const ModeEnv = "PRO_GATE_MODE"

// WouldBlockEvent is the log event of ModeLog (and of every stop in enforce).
const WouldBlockEvent = "pro_gate.would_block"

// ModeFromEnv reads PRO_GATE_MODE (unset = enforce). Another value is an
// error: a typo must not switch the gate off silently.
func ModeFromEnv(getenv func(string) string) (Mode, error) {
	switch m := Mode(strings.ToLower(strings.TrimSpace(getenv(ModeEnv)))); m {
	case "":
		return ModeEnforce, nil
	case ModeEnforce, ModeLog, ModeOff:
		return m, nil
	default:
		return ModeEnforce, fmt.Errorf("%s must be enforce|log|off, got %q", ModeEnv, getenv(ModeEnv))
	}
}

// Checks reports whether the gate looks at all (enforce or log).
func (m Mode) Checks() bool { return m != ModeOff }

// Stop is called where the gate would stop something (where = the API or
// runner step, uid = whose Pro was missing). It logs pro_gate.would_block
// and reports whether to really stop (enforce; the zero Mode counts as
// enforce).
func (m Mode) Stop(ctx context.Context, log *slog.Logger, where, uid string, features []string) bool {
	if m == ModeOff {
		return false
	}
	enforce := m != ModeLog
	if log == nil {
		log = slog.Default()
	}
	log.InfoContext(ctx, WouldBlockEvent,
		slog.String("event", WouldBlockEvent),
		slog.String("mode", string(m.orEnforce())),
		slog.Bool("blocked", enforce),
		slog.String("where", where),
		slog.String("uid_hash", httpx.UIDHash(uid)),
		slog.Any("features", features),
	)
	return enforce
}

func (m Mode) orEnforce() Mode {
	if m == "" {
		return ModeEnforce
	}
	return m
}
