package runner

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/extapi"
	"github.com/jphacks/kz_2601/backend/internal/runner/quota"
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
// whose id is derived from the rule, the action and the id cell, so running
// it again updates the same record and never adds a second one.

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
	// Decision is the run's quota.Begin answer: the ext call and the writes
	// are counted against it, like the rule's other actions.
	Decision quota.Decision
}

// ExtCaller is the external API door (*extapi.Service).
type ExtCaller interface {
	Call(ctx context.Context, r extapi.CallRequest) (extapi.CallResult, error)
}

// ImportTarget writes the records (*scope.Handle).
type ImportTarget interface {
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
	Rows      int // data rows read (header not counted)
	Created   int
	Updated   int
	Unchanged int // already applied by an earlier attempt of the same event
	NoID      int // rows with an empty id cell
	Duplicate int // later rows with an id an earlier row had
	// Required: rows missing a required field that were not updated (no
	// record to update, or already applied by an earlier attempt).
	Required  int
	BadValues int // cells that did not fit their field's type (left out)
	Dropped   int // rows the write limits cut
	// Charge is what quota.Writes counted (kept with the run for a refund).
	Charge quota.Charge
	// Block is why rows were dropped (nil when none).
	Block *quota.Block
}

type importRow struct {
	id     string
	values map[string]any // nil value: the cell is empty
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
	res.Rows = len(data)
	seen := map[string]bool{}
	var todo []importRow
	for _, r := range data {
		id := cell(r, idCol)
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
			v, ok := importValue(f, raw)
			if !ok {
				res.BadValues++
				continue
			}
			vals[f.ID] = v
		}
		todo = append(todo, importRow{id: id, values: vals})
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
		recID := store.IDFromClientID(importClientID(run.RuleID, run.Action, r.id))
		create := map[string]any{}
		for k, v := range r.values {
			if v != nil {
				create[k] = v
			}
		}
		if missingRequired(ent, create) {
			// a new record needs it; an existing one keeps what it has
			create = nil
		}
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
		created, applied, err := im.Target.UpsertRecord(ctx, ent.ID, recID, create, origin, markKeyOf(run, recID), fn)
		if err != nil {
			return res, rejectedWrite(err)
		}
		switch {
		case created:
			res.Created++
		case applied:
			res.Updated++
		case create == nil:
			res.Required++
		default:
			res.Unchanged++
		}
	}
	return res, nil
}

// importClientID names the record of one id cell: the same rule, action
// and id give the same record on every run.
func importClientID(rule string, action int, id string) string {
	return "import/" + rule + "/" + strconv.Itoa(action) + "/" + id
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

// importZone: a date or time without a zone is Japan time.
var importZone = time.FixedZone("JST", 9*60*60)

// importValue turns a cell into f's stored form; ok=false when it does not fit.
func importValue(f *dsl.Field, s string) (any, bool) {
	switch f.Type {
	case dsl.FieldString, dsl.FieldText:
		return s, true
	case dsl.FieldInteger, dsl.FieldNumber, dsl.FieldCurrency:
		n, err := strconv.ParseFloat(strings.NewReplacer(",", "", "¥", "", "￥", "").Replace(s), 64)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
			return nil, false
		}
		if f.Type == dsl.FieldInteger {
			n = math.Round(n)
		}
		return n, true
	case dsl.FieldBoolean:
		switch strings.ToLower(s) {
		case "true", "1", "yes", "はい", "○", "◯":
			return true, true
		case "false", "0", "no", "いいえ", "×":
			return false, true
		}
	case dsl.FieldDate:
		for _, l := range []string{"2006-01-02", "2006/1/2", "2006-1-2"} {
			if t, err := time.ParseInLocation(l, s, importZone); err == nil {
				return t.Format("2006-01-02"), true
			}
		}
	case dsl.FieldDatetime:
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t.UTC().Format("2006-01-02T15:04:05Z"), true
		}
		for _, l := range []string{"2006-01-02 15:04", "2006/1/2 15:04", "2006-01-02 15:04:05", "2006/1/2 15:04:05"} {
			if t, err := time.ParseInLocation(l, s, importZone); err == nil {
				return t.UTC().Format("2006-01-02T15:04:05Z"), true
			}
		}
	case dsl.FieldEnum:
		if slices.Contains(f.Options, s) {
			return s, true
		}
	}
	return nil, false
}
