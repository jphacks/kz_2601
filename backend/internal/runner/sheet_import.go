package runner

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"

	"github.com/jphacks/kz_2601/backend/internal/appconfig"
	"github.com/jphacks/kz_2601/backend/internal/extapi"
	"github.com/jphacks/kz_2601/backend/internal/runner/quota"
	"github.com/jphacks/kz_2601/backend/internal/runner/schedule"
	"github.com/jphacks/kz_2601/backend/internal/runner/scope"
	"github.com/jphacks/kz_2601/backend/internal/store"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// The scheduled sheet import (#1712): a server rule run reads a range of a
// Google Sheet (googleSheets readRange) and upserts its rows into an entity,
// matching on an id column. The DSL action that names it waits on #1643, so
// nothing in the DSL reaches it yet: this is the executor a rule action (or
// an internal caller) runs with the run's quota.Decision.
//
// The first row of the range is the header. Each later row is one record
// whose id is derived from the entity, the import's key and the id cell
// (importClientID), so running it again updates the same record and never
// adds a second one. The rule and the action are left out of the key on
// purpose: renaming the rule or inserting an action before this one must
// not duplicate every row. A row whose converted values equal the stored
// record is not written (and not counted as a write).

// Row caps of one import run (header not counted). readRange reads at most
// 100 rows, the header included.
const (
	DefaultImportRows = 50
	MaxImportRows     = 99
)

// Errors of SheetImporter.Import.
var (
	// ErrImportSpec: the spec does not fit the app (entity, fields, types).
	ErrImportSpec = errors.New("runner: bad sheet import")
	// ErrImportNoIDColumn: the header has no id column.
	ErrImportNoIDColumn = errors.New("runner: sheet import: the header has no id column")
)

// SheetImport is what to import.
type SheetImport struct {
	SpreadsheetID string
	Range         string
	// Entity receives the rows.
	Entity string
	// IDColumn is the header of the column rows are matched on.
	IDColumn string
	// ImportKey, when set, names the import in the record ids instead of
	// IDColumn, so the id column's header can be renamed without
	// duplicating rows. "" = IDColumn.
	ImportKey string
	// Fields maps a header to a field id of Entity. nil: a header that is a
	// field id or a field name goes to that field. Other columns are ignored.
	Fields map[string]string
	// MaxRows caps the rows of one run: 0 = DefaultImportRows, at most
	// MaxImportRows.
	MaxRows int
}

// ImportRun is the server rule run the import is part of.
type ImportRun struct {
	AppID            string
	CreatorUID       string
	CreatorAnonymous bool
	EventID          string
	RuleID           string
	Action           int
	Version          int
	ActorUID         string
	// Location reads a date or time cell without a zone (the run's time
	// zone, Runner.location). nil = schedule.DefaultZone (Japan).
	Location *time.Location
	// Decision is the run's quota.Begin answer: the ext call and the writes
	// are counted against it, like the rule's other actions.
	Decision quota.Decision
}

// ExtCaller is the external API door (*extapi.Service).
type ExtCaller interface {
	Call(ctx context.Context, r extapi.CallRequest) (extapi.CallResult, error)
}

// ImportTarget reads and writes the records (*scope.Handle).
type ImportTarget interface {
	ImportedRecord(ctx context.Context, entity, id string) (data map[string]any, found bool, err error)
	UpsertRecord(ctx context.Context, entity, id string, data map[string]any, origin store.RecordOrigin, key string, fn func(map[string]any) map[string]any) (created, applied bool, err error)
}

// WriteCounter grants the run's writes (*quota.Quota).
type WriteCounter interface {
	Writes(ctx context.Context, dec quota.Decision, n int) (quota.Grant, error)
}

var (
	_ ExtCaller    = (*extapi.Service)(nil)
	_ ImportTarget = (*scope.Handle)(nil)
	_ WriteCounter = (*quota.Quota)(nil)
)

// SheetImporter runs imports.
type SheetImporter struct {
	Ext    ExtCaller
	Quota  WriteCounter
	Target ImportTarget
}

// ImportResult counts what one run did.
type ImportResult struct {
	Rows    int // data rows read (header not counted)
	Created int
	Updated int
	// Unchanged: rows equal to the stored record (not written, not
	// counted), or already applied by an earlier attempt of the same event.
	Unchanged int
	NoID      int // rows with an empty id cell
	Duplicate int // later rows with an id an earlier row had
	// Required: rows missing a required field that were not updated (no
	// record to update, or already applied by an earlier attempt).
	Required  int
	BadValues int // cells that did not fit their field's type (left out)
	Dropped   int // rows the write limits cut
	// Charge is what quota.Writes counted (kept with the run for a refund):
	// only the rows that were to be written.
	Charge quota.Charge
	// Block is why rows were dropped (nil when none).
	Block *quota.Block
}

type importRow struct {
	recID  string
	values map[string]any // nil value: the cell is empty
	create map[string]any // nil: update only (the record is there)
}

// Import reads the range and upserts its rows into spec.Entity of app.
func (im SheetImporter) Import(ctx context.Context, app *dsl.App, run ImportRun, spec SheetImport) (ImportResult, error) {
	var res ImportResult
	ent, cols, err := checkImport(app, spec)
	if err != nil {
		return res, err
	}
	maxRows := spec.MaxRows
	if maxRows <= 0 {
		maxRows = DefaultImportRows
	}
	maxRows = min(maxRows, MaxImportRows)

	dec := run.Decision
	out, err := im.Ext.Call(ctx, extapi.CallRequest{
		Provider: extapi.GoogleSheets, Op: "readRange", AppID: run.AppID,
		Args:       map[string]string{"spreadsheetId": spec.SpreadsheetID, "range": spec.Range, "limit": strconv.Itoa(maxRows + 1)},
		CreatorUID: run.CreatorUID, CreatorAnonymous: run.CreatorAnonymous,
		RequestID: "import/" + run.EventID + "/" + run.RuleID + "/" + strconv.Itoa(run.Action),
		Run:       &dec,
	})
	if err != nil {
		return res, fmt.Errorf("runner: sheet import read: %w", err)
	}
	rows := out.Output.Cells
	if len(rows) == 0 {
		return res, fmt.Errorf("%w (the range is empty)", ErrImportNoIDColumn)
	}
	header := rows[0]
	idCol := slices.Index(header, strings.TrimSpace(spec.IDColumn))
	if idCol < 0 {
		return res, fmt.Errorf("%w: %q", ErrImportNoIDColumn, spec.IDColumn)
	}
	fieldOf := make([]*dsl.Field, len(header)) // column → field (nil = ignored)
	for i, h := range header {
		fieldOf[i] = cols(h)
	}

	data := rows[1:]
	if len(data) > maxRows {
		data = data[:maxRows]
	}
	loc := run.Location
	if loc == nil {
		loc = schedule.Location("")
	}
	ns := importNamespace(spec)
	res.Rows = len(data)
	seen := map[string]bool{}
	var read []importRow
	for _, r := range data {
		id := normalizeImportID(cell(r, idCol))
		if id == "" {
			res.NoID++
			continue
		}
		if seen[id] {
			res.Duplicate++
			continue
		}
		seen[id] = true
		vals := map[string]any{}
		for i, f := range fieldOf {
			if f == nil {
				continue
			}
			raw := cell(r, i)
			if raw == "" {
				vals[f.ID] = nil
				continue
			}
			v, ok := importValue(f, raw, loc)
			if !ok {
				res.BadValues++
				continue
			}
			vals[f.ID] = v
		}
		read = append(read, importRow{recID: store.IDFromClientID(importClientID(ent.ID, ns, id)), values: vals})
	}

	// only the rows that change something are written and counted
	var todo []importRow
	for _, r := range read {
		cur, found, err := im.Target.ImportedRecord(ctx, ent.ID, r.recID)
		if err != nil {
			return res, fmt.Errorf("runner: sheet import: reading %s: %w", ent.ID, err)
		}
		if found {
			if sameRecord(ent, cur, r.values) {
				res.Unchanged++
				continue
			}
		} else {
			r.create = map[string]any{}
			for k, v := range r.values {
				if v != nil {
					r.create[k] = v
				}
			}
			if missingRequired(ent, r.create) {
				res.Required++ // a new record needs it
				continue
			}
		}
		todo = append(todo, r)
	}
	if len(todo) == 0 {
		return res, nil
	}

	grant, err := im.Quota.Writes(ctx, run.Decision, len(todo))
	if err != nil {
		return res, err
	}
	res.Charge, res.Block = grant.Charge, grant.Block
	if grant.Allowed < len(todo) {
		res.Dropped = len(todo) - grant.Allowed
		todo = todo[:grant.Allowed]
	}

	origin := store.RecordOrigin{RuleID: run.RuleID, Version: run.Version, EventID: run.EventID, ActorUID: run.ActorUID}
	for _, r := range todo {
		fn := func(d map[string]any) map[string]any {
			if d == nil {
				d = map[string]any{}
			}
			for k, v := range r.values {
				if v != nil {
					d[k] = v
				} else if f := ent.FieldByID(k); f != nil && !f.Required {
					delete(d, k)
				}
			}
			return d
		}
		created, applied, err := im.Target.UpsertRecord(ctx, ent.ID, r.recID, r.create, origin, markKeyOf(run, r.recID), fn)
		if err != nil {
			return res, rejectedWrite(err)
		}
		switch {
		case created:
			res.Created++
		case applied:
			res.Updated++
		default:
			// applied by an earlier attempt of this event, or deleted since
			// it was read (the write it was counted for is not refunded)
			res.Unchanged++
		}
	}
	return res, nil
}

// importNamespace is the import's part of its record ids: ImportKey, or
// else IDColumn (NFKC, trimmed).
func importNamespace(spec SheetImport) string {
	if k := strings.TrimSpace(norm.NFKC.String(spec.ImportKey)); k != "" {
		return "key:" + k
	}
	return "col:" + strings.TrimSpace(norm.NFKC.String(spec.IDColumn))
}

// importClientID names the record of one id: the same entity, namespace and
// normalized id give the same record on every run, whatever rule or action
// runs the import. Each part is length-prefixed so no value can make two
// different triples meet.
func importClientID(entity, ns, id string) string {
	var b strings.Builder
	b.WriteString("import")
	for _, p := range []string{entity, ns, id} {
		b.WriteString("/")
		b.WriteString(strconv.Itoa(len(p)))
		b.WriteString(":")
		b.WriteString(p)
	}
	return b.String()
}

// importNumberRe is an id cell that is a number: digits with optional
// thousands separators and an optional fraction.
var importNumberRe = regexp.MustCompile(`^([+-]?)(\d{1,3}(?:,\d{3})+|\d+)(?:\.(\d+))?$`)

// normalizeImportID is the id an id cell is matched on: NFKC (full-width
// digits and letters become ASCII), trimmed, and a number written in one
// canonical form, so "1", "1.00", "001" and "１" match, and so do "1,000"
// and "1000", whatever display format the column has.
func normalizeImportID(s string) string {
	s = strings.TrimSpace(norm.NFKC.String(s))
	m := importNumberRe.FindStringSubmatch(s)
	if m == nil {
		return s
	}
	n := strings.TrimLeft(strings.ReplaceAll(m[2], ",", ""), "0")
	if n == "" {
		n = "0"
	}
	if frac := strings.TrimRight(m[3], "0"); frac != "" {
		n += "." + frac
	}
	if m[1] == "-" && n != "0" {
		n = "-" + n
	}
	return n
}

// sameRecord reports whether writing values to cur would change nothing.
func sameRecord(ent *dsl.Entity, cur, values map[string]any) bool {
	for k, v := range values {
		f := ent.FieldByID(k)
		old := cur[k]
		if v == nil {
			if old != nil && f != nil && !f.Required {
				return false // the write would clear it
			}
			continue
		}
		if old == nil || !sameValue(f, old, v) {
			return false
		}
	}
	return true
}

// sameValue compares a stored value with a converted one: numbers by value
// (the store may give back int64 or float64), datetimes by instant.
func sameValue(f *dsl.Field, old, v any) bool {
	if a, ok := importNumber(old); ok {
		b, ok := importNumber(v)
		return ok && a == b
	}
	if f != nil && f.Type == dsl.FieldDatetime {
		as, _ := old.(string)
		bs, _ := v.(string)
		a, aerr := time.Parse(time.RFC3339, as)
		b, berr := time.Parse(time.RFC3339, bs)
		if aerr == nil && berr == nil {
			return a.Equal(b)
		}
	}
	return reflect.DeepEqual(old, v)
}

func importNumber(v any) (float64, bool) {
	switch n := v.(type) {
	case int64:
		return float64(n), true
	case int:
		return float64(n), true
	case float64:
		return n, true
	}
	return 0, false
}

// markKeyOf is the once-per-event mark of one record's update, so a retry
// of the same event does not apply it twice.
func markKeyOf(run ImportRun, recID string) string {
	return run.EventID + "_" + run.RuleID + "_" + strconv.Itoa(run.Action) + "_" + recID
}

// checkImport checks spec against app and returns the entity and the
// header → field lookup.
func checkImport(app *dsl.App, spec SheetImport) (*dsl.Entity, func(string) *dsl.Field, error) {
	if app == nil {
		return nil, nil, fmt.Errorf("%w: no app", ErrImportSpec)
	}
	if spec.SpreadsheetID == "" || spec.Range == "" || strings.TrimSpace(spec.IDColumn) == "" {
		return nil, nil, fmt.Errorf("%w: spreadsheetId, range and idColumn are required", ErrImportSpec)
	}
	ent := app.EntityByID(spec.Entity)
	if ent == nil {
		return nil, nil, fmt.Errorf("%w: entity %q is not in the app", ErrImportSpec, spec.Entity)
	}
	// the same targets as fileImport (pkg/dsl/file_import.go)
	switch {
	case ent.EffectiveScope() == dsl.ScopePublic:
		return nil, nil, fmt.Errorf("%w: cannot add to public entity %q", ErrImportSpec, ent.ID)
	case ent.Single:
		return nil, nil, fmt.Errorf("%w: cannot add to single entity %q", ErrImportSpec, ent.ID)
	case ent.Match != "":
		return nil, nil, fmt.Errorf("%w: cannot add to match entity %q", ErrImportSpec, ent.ID)
	}
	usable := func(f *dsl.Field) error {
		if f.Computed != nil || f.Encrypted {
			return fmt.Errorf("%w: field %q is computed or encrypted", ErrImportSpec, f.ID)
		}
		if !importable(f.Type) {
			return fmt.Errorf("%w: field %q of type %s cannot be imported", ErrImportSpec, f.ID, f.Type)
		}
		return nil
	}
	if spec.Fields != nil {
		m := map[string]*dsl.Field{}
		for h, id := range spec.Fields {
			f := ent.FieldByID(id)
			if f == nil {
				return nil, nil, fmt.Errorf("%w: field %q is not in %s", ErrImportSpec, id, ent.ID)
			}
			if err := usable(f); err != nil {
				return nil, nil, err
			}
			m[h] = f
		}
		return ent, func(h string) *dsl.Field { return m[h] }, nil
	}
	return ent, func(h string) *dsl.Field {
		for i := range ent.Fields {
			f := &ent.Fields[i]
			if (f.ID == h || f.Name == h) && usable(f) == nil {
				return f
			}
		}
		return nil
	}, nil
}

func importable(t dsl.FieldType) bool {
	switch t {
	case dsl.FieldString, dsl.FieldText, dsl.FieldInteger, dsl.FieldNumber, dsl.FieldCurrency,
		dsl.FieldBoolean, dsl.FieldDate, dsl.FieldDatetime, dsl.FieldEnum:
		return true
	}
	return false
}

func missingRequired(ent *dsl.Entity, vals map[string]any) bool {
	for _, f := range ent.Fields {
		if _, ok := vals[f.ID]; f.Required && f.Computed == nil && !ok {
			return true
		}
	}
	return false
}

func cell(cells []string, i int) string {
	if i < len(cells) {
		return strings.TrimSpace(cells[i])
	}
	return ""
}

// importValue turns a cell into f's stored form; ok=false when it does not
// fit. The cell is read into a typed value (a number, a bool, an RFC 3339
// time) and then goes through the same check a device write does
// (appconfig.NormalizeValue), so an import stores what the app would: an
// integer or currency with a fraction is refused (not rounded), an enum
// without options takes any string, the int64 bounds hold. A date or time
// without a zone is read in loc.
func importValue(f *dsl.Field, s string, loc *time.Location) (any, bool) {
	var v any = s
	switch f.Type {
	case dsl.FieldInteger, dsl.FieldNumber, dsl.FieldCurrency:
		n, err := strconv.ParseFloat(strings.NewReplacer(",", "", "¥", "", "￥", "").Replace(s), 64)
		if err != nil {
			return nil, false
		}
		v = n
	case dsl.FieldBoolean:
		switch strings.ToLower(s) {
		case "true", "1", "yes", "はい", "○", "◯":
			v = true
		case "false", "0", "no", "いいえ", "×":
			v = false
		default:
			return nil, false
		}
	case dsl.FieldDate:
		for _, l := range []string{"2006-01-02", "2006/1/2", "2006-1-2"} {
			if t, err := time.Parse(l, s); err == nil {
				v = t.Format("2006-01-02")
				break
			}
		}
	case dsl.FieldDatetime:
		if _, err := time.Parse(time.RFC3339, s); err != nil {
			for _, l := range []string{"2006-01-02 15:04", "2006/1/2 15:04", "2006-01-02 15:04:05", "2006/1/2 15:04:05",
				"2006-01-02", "2006/1/2", "2006-1-2"} {
				if t, err := time.ParseInLocation(l, s, loc); err == nil {
					v = t.Format(time.RFC3339)
					break
				}
			}
		}
	}
	out, err := appconfig.NormalizeValue(f, v)
	if err != nil {
		return nil, false
	}
	return out, true
}
