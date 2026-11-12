package runner_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/extapi"
	"github.com/jphacks/kz_2601/backend/internal/runner"
	"github.com/jphacks/kz_2601/backend/internal/runner/quota"
	"github.com/jphacks/kz_2601/backend/internal/runner/scope"
	"github.com/jphacks/kz_2601/backend/internal/store"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// fakeSheet answers readRange with cells and records the calls.
type fakeSheet struct {
	cells [][]string
	err   error
	calls []extapi.CallRequest
}

func (f *fakeSheet) Call(_ context.Context, r extapi.CallRequest) (extapi.CallResult, error) {
	f.calls = append(f.calls, r)
	if f.err != nil {
		return extapi.CallResult{}, f.err
	}
	n := len(f.cells)
	out := extapi.Output{Rows: make([]extapi.Row, n), Cells: f.cells}
	return extapi.CallResult{Output: out}, nil
}

// fakeWrites grants up to allow writes (0 = all) and records the asks.
type fakeWrites struct {
	allow int
	asked []int
}

func (f *fakeWrites) Writes(_ context.Context, _ quota.Decision, n int) (quota.Grant, error) {
	f.asked = append(f.asked, n)
	if f.allow > 0 && n > f.allow {
		return quota.Grant{Allowed: f.allow, Dropped: n - f.allow, Block: &quota.Block{Scope: quota.ScopeRunWrites}}, nil
	}
	return quota.Grant{Allowed: n}, nil
}

func importApp() *dsl.App {
	return &dsl.App{Entities: []dsl.Entity{{ID: "stock", Name: "在庫", Scope: dsl.ScopeShared, Fields: []dsl.Field{
		{ID: "code", Name: "品番", Type: dsl.FieldString, Required: true},
		{ID: "name", Name: "品名", Type: dsl.FieldString},
		{ID: "qty", Name: "数", Type: dsl.FieldInteger},
		{ID: "ok", Name: "有効", Type: dsl.FieldBoolean},
	}}}}
}

type importEnv struct {
	st    *store.Memory
	ext   *fakeSheet
	q     *fakeWrites
	im    runner.SheetImporter
	app   *dsl.App
	appID string
}

func newImportEnv(t *testing.T, cells [][]string) *importEnv {
	t.Helper()
	return newImportEnvApp(t, importApp(), cells)
}

func newImportEnvApp(t *testing.T, app *dsl.App, cells [][]string) *importEnv {
	t.Helper()
	st := store.NewMemory()
	h, err := scope.New(st, "rt", store.OutboxEvent{AppID: "app1", Scope: string(dsl.ScopeShared)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	h.Bind(app, "", "", nil)
	e := &importEnv{st: st, ext: &fakeSheet{cells: cells}, q: &fakeWrites{}, app: app, appID: "app1"}
	e.im = runner.SheetImporter{Ext: e.ext, Quota: e.q, Target: h}
	return e
}

func importRun(event string) runner.ImportRun {
	return runner.ImportRun{AppID: "app1", CreatorUID: "creator", EventID: event, RuleID: "morning", Action: 0, Version: 3,
		Decision: quota.Decision{Allowed: true, Plan: quota.PlanPro}}
}

var importSpec = runner.SheetImport{SpreadsheetID: "1AbCdEfGhIjKlMnOpQrStUvWxYz012345", Range: "在庫!A:D", Entity: "stock", IDColumn: "品番"}

func (e *importEnv) records(t *testing.T) map[string]map[string]any {
	t.Helper()
	recs, err := e.st.ListRecords(t.Context(), store.PublicRecords(e.appID, "stock"), 0)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]map[string]any{}
	for _, r := range recs {
		out[r.Data["code"].(string)] = r.Data
	}
	if len(out) != len(recs) {
		t.Fatalf("records with the same code: %d records, %d codes", len(recs), len(out))
	}
	return out
}

// Running twice upserts by the id column: no second record, and a changed
// row changes its record.
func TestSheetImportTwiceNoDuplicates(t *testing.T) {
	e := newImportEnv(t, [][]string{
		{"品番", "品名", "数", "有効", "メモ"},
		{"A-1", "りんご", "3", "はい", "無視"},
		{"A-2", "みかん", "1,200", "FALSE"},
	})
	res, err := e.im.Import(t.Context(), e.app, importRun("ev1"), importSpec)
	if err != nil {
		t.Fatal(err)
	}
	if res.Created != 2 || res.Updated != 0 || res.Rows != 2 {
		t.Fatalf("first run = %+v", res)
	}
	got := e.records(t)
	if got["A-1"]["qty"] != int64(3) || got["A-1"]["ok"] != true || got["A-2"]["qty"] != int64(1200) || got["A-2"]["ok"] != false {
		t.Fatalf("records = %v", got)
	}
	if _, ok := got["A-1"]["メモ"]; ok {
		t.Fatal("a column without a field was saved")
	}

	// the next morning: A-1 changed, A-2 the same, A-3 new
	e.ext.cells = [][]string{
		{"品番", "品名", "数", "有効"},
		{"A-1", "りんご", "0", "はい"},
		{"A-2", "みかん", "1,200", "FALSE"},
		{"A-3", "ぶどう", "5", ""},
	}
	res, err = e.im.Import(t.Context(), e.app, importRun("ev2"), importSpec)
	if err != nil {
		t.Fatal(err)
	}
	// A-2 is the same: not written, not counted
	if res.Created != 1 || res.Updated != 1 || res.Unchanged != 1 || e.q.asked[len(e.q.asked)-1] != 2 {
		t.Fatalf("second run = %+v, asked %v", res, e.q.asked)
	}
	got = e.records(t)
	if len(got) != 3 {
		t.Fatalf("records = %d, want 3", len(got))
	}
	if got["A-1"]["qty"] != int64(0) {
		t.Fatalf("A-1 qty = %v, want 0", got["A-1"]["qty"])
	}

	// a retry of the same event adds nothing, writes nothing and asks for
	// no writes: every row equals its record
	asked := len(e.q.asked)
	res, err = e.im.Import(t.Context(), e.app, importRun("ev2"), importSpec)
	if err != nil {
		t.Fatal(err)
	}
	if res.Created != 0 || res.Updated != 0 || res.Unchanged != 3 || len(e.records(t)) != 3 || len(e.q.asked) != asked {
		t.Fatalf("retry = %+v, asked %v", res, e.q.asked)
	}
}

// An updated row changes the record; an emptied optional cell clears it.
func TestSheetImportUpdatesRow(t *testing.T) {
	e := newImportEnv(t, [][]string{{"品番", "品名", "数"}, {"B-1", "古い名前", "2"}})
	if _, err := e.im.Import(t.Context(), e.app, importRun("ev1"), importSpec); err != nil {
		t.Fatal(err)
	}
	e.ext.cells = [][]string{{"品番", "品名", "数"}, {"B-1", "新しい名前", ""}}
	res, err := e.im.Import(t.Context(), e.app, importRun("ev2"), importSpec)
	if err != nil {
		t.Fatal(err)
	}
	got := e.records(t)["B-1"]
	if res.Updated != 1 || got["name"] != "新しい名前" {
		t.Fatalf("res = %+v, record = %v", res, got)
	}
	if _, ok := got["qty"]; ok {
		t.Fatalf("qty = %v, want cleared", got["qty"])
	}
}

// A header without the id column is an error and writes nothing.
func TestSheetImportMissingIDColumn(t *testing.T) {
	e := newImportEnv(t, [][]string{{"品名", "数"}, {"りんご", "3"}})
	_, err := e.im.Import(t.Context(), e.app, importRun("ev1"), importSpec)
	if !errors.Is(err, runner.ErrImportNoIDColumn) {
		t.Fatalf("err = %v, want ErrImportNoIDColumn", err)
	}
	if len(e.records(t)) != 0 || len(e.q.asked) != 0 {
		t.Fatal("wrote or counted writes without an id column")
	}

	e.ext.cells = nil // an empty range has no header either
	if _, err := e.im.Import(t.Context(), e.app, importRun("ev2"), importSpec); !errors.Is(err, runner.ErrImportNoIDColumn) {
		t.Fatalf("empty range: err = %v", err)
	}
}

// The ext call carries the run's Decision (counted as one `runs` of the
// run), a request id per event and action, and limit = rows + header.
func TestSheetImportCallAndRowCap(t *testing.T) {
	cells := [][]string{{"品番"}}
	for _, c := range []string{"C-1", "C-2", "C-3", "C-4"} {
		cells = append(cells, []string{c})
	}
	e := newImportEnv(t, cells)
	spec := importSpec
	spec.MaxRows = 3
	run := importRun("ev1")
	res, err := e.im.Import(t.Context(), e.app, run, spec)
	if err != nil {
		t.Fatal(err)
	}
	c := e.ext.calls[0]
	if c.Provider != extapi.GoogleSheets || c.Op != "readRange" || c.Args["limit"] != "4" || c.Run == nil || c.Run.Plan != quota.PlanPro ||
		c.CreatorUID != "creator" || c.UserUID != "" || c.RequestID != "import/ev1/morning/0" {
		t.Fatalf("call = %+v", c)
	}
	if res.Rows != 3 || res.Created != 3 || len(e.records(t)) != 3 {
		t.Fatalf("res = %+v", res)
	}

	// spec.MaxRows over the cap is the cap (99 rows + header = readRange's 100)
	spec.MaxRows = 500
	if _, err := e.im.Import(t.Context(), e.app, importRun("ev2"), spec); err != nil {
		t.Fatal(err)
	}
	if got := e.ext.calls[1].Args["limit"]; got != "100" {
		t.Fatalf("limit = %s, want 100", got)
	}
}

// The run's write limits cut rows; rows without an id, repeated ids and
// new rows missing a required field are skipped.
func TestSheetImportSkipsAndLimits(t *testing.T) {
	e := newImportEnv(t, [][]string{
		{"品番", "数"},
		{"", "1"},
		{"D-1", "x"},
		{"D-1", "9"},
		{"D-2", "2"},
		{"D-3", "3"},
	})
	e.q.allow = 2
	res, err := e.im.Import(t.Context(), e.app, importRun("ev1"), importSpec)
	if err != nil {
		t.Fatal(err)
	}
	if res.NoID != 1 || res.Duplicate != 1 || res.BadValues != 1 || res.Dropped != 1 || res.Block == nil || res.Created != 2 {
		t.Fatalf("res = %+v", res)
	}
	got := e.records(t)
	if _, ok := got["D-1"]["qty"]; ok {
		t.Fatalf("D-1 kept a bad value: %v", got["D-1"])
	}
	if _, ok := got["D-3"]; ok {
		t.Fatal("a dropped row was written")
	}
}

// A failed read writes nothing and passes the ext error on.
func TestSheetImportReadError(t *testing.T) {
	e := newImportEnv(t, nil)
	e.ext.err = extapi.ErrNotConnected
	_, err := e.im.Import(t.Context(), e.app, importRun("ev1"), importSpec)
	if !errors.Is(err, extapi.ErrNotConnected) || len(e.q.asked) != 0 {
		t.Fatalf("err = %v, asked = %v", err, e.q.asked)
	}
}

// A spec that does not fit the app fails before calling out.
func TestSheetImportBadSpec(t *testing.T) {
	for name, spec := range map[string]runner.SheetImport{
		"no entity":   {SpreadsheetID: "x", Range: "A:B", Entity: "nope", IDColumn: "品番"},
		"no idColumn": {SpreadsheetID: "x", Range: "A:B", Entity: "stock"},
		"bad field":   {SpreadsheetID: "x", Range: "A:B", Entity: "stock", IDColumn: "品番", Fields: map[string]string{"品番": "nope"}},
	} {
		t.Run(name, func(t *testing.T) {
			e := newImportEnv(t, nil)
			if _, err := e.im.Import(t.Context(), e.app, importRun("ev1"), spec); !errors.Is(err, runner.ErrImportSpec) || len(e.ext.calls) != 0 {
				t.Fatalf("err = %v, calls = %d", err, len(e.ext.calls))
			}
		})
	}
}

// The record of a row is keyed on the entity, the id column (or ImportKey)
// and the normalized id: renaming the rule, moving the action and changing
// the id column's display format find the same records.
func TestSheetImportKeyIgnoresRuleActionAndFormat(t *testing.T) {
	e := newImportEnv(t, [][]string{{"品番", "数"}, {"1", "5"}, {"2", "6"}, {"1000", "7"}, {"ab", "8"}})
	if res, err := e.im.Import(t.Context(), e.app, importRun("ev1"), importSpec); err != nil || res.Created != 4 {
		t.Fatalf("first run = %+v, %v", res, err)
	}
	run := importRun("ev2")
	run.RuleID, run.Action = "renamed", 3
	e.ext.cells = [][]string{{"品番", "数"}, {"1.00", "5"}, {"００２", "6"}, {"1,000", "7"}, {" ａｂ ", "8"}}
	res, err := e.im.Import(t.Context(), e.app, run, importSpec)
	if err != nil {
		t.Fatal(err)
	}
	// the code field takes the new text; no row makes a second record
	if res.Created != 0 || res.Updated != 4 {
		t.Fatalf("reformatted run = %+v", res)
	}
	if got := e.records(t); len(got) != 4 || got["1.00"]["qty"] != int64(5) || got["ａｂ"]["qty"] != int64(8) {
		t.Fatalf("records = %v", got)
	}

	// two cells that normalize to one id are one row
	e.ext.cells = [][]string{{"品番", "数"}, {"001", "9"}, {"1", "10"}}
	if res, err := e.im.Import(t.Context(), e.app, importRun("ev3"), importSpec); err != nil || res.Duplicate != 1 || res.Created != 0 {
		t.Fatalf("same id twice = %+v, %v", res, err)
	}
}

// ImportKey replaces the id column in the key: the column can be renamed.
func TestSheetImportImportKey(t *testing.T) {
	e := newImportEnv(t, [][]string{{"品番", "数"}, {"K-1", "1"}})
	spec := importSpec
	spec.ImportKey = "stock-sheet"
	if res, err := e.im.Import(t.Context(), e.app, importRun("ev1"), spec); err != nil || res.Created != 1 {
		t.Fatalf("first run = %+v, %v", res, err)
	}
	e.ext.cells = [][]string{{"コード", "数"}, {"K-1", "2"}}
	spec.IDColumn, spec.Fields = "コード", map[string]string{"コード": "code", "数": "qty"}
	res, err := e.im.Import(t.Context(), e.app, importRun("ev2"), spec)
	if err != nil || res.Created != 0 || res.Updated != 1 {
		t.Fatalf("renamed column = %+v, %v", res, err)
	}
	if got := e.records(t); len(got) != 1 || got["K-1"]["qty"] != int64(2) {
		t.Fatalf("records = %v", got)
	}
}

// Rows equal to their records are not written and not counted; only the
// changed rows are asked of quota.Writes.
func TestSheetImportSkipsUnchangedRows(t *testing.T) {
	cells := [][]string{{"品番", "品名", "数"}, {"U-1", "a", "1"}, {"U-2", "b", "2"}, {"U-3", "c", ""}}
	e := newImportEnv(t, cells)
	if _, err := e.im.Import(t.Context(), e.app, importRun("ev1"), importSpec); err != nil {
		t.Fatal(err)
	}
	res, err := e.im.Import(t.Context(), e.app, importRun("ev2"), importSpec)
	if err != nil || res.Unchanged != 3 || res.Updated != 0 || len(e.q.asked) != 1 || !res.Charge.Empty() {
		t.Fatalf("same sheet = %+v, %v, asked %v", res, err, e.q.asked)
	}
	e.ext.cells = [][]string{{"品番", "品名", "数"}, {"U-1", "a", "1"}, {"U-2", "b", "3"}, {"U-3", "c", ""}}
	res, err = e.im.Import(t.Context(), e.app, importRun("ev3"), importSpec)
	if err != nil || res.Unchanged != 2 || res.Updated != 1 || e.q.asked[len(e.q.asked)-1] != 1 {
		t.Fatalf("one change = %+v, %v, asked %v", res, err, e.q.asked)
	}
}

// Cells go through the device write's checks (appconfig.NormalizeValue);
// a date or time without a zone is read in the run's location.
func TestSheetImportValuesLikeDeviceWrites(t *testing.T) {
	app := &dsl.App{Entities: []dsl.Entity{{ID: "stock", Name: "在庫", Scope: dsl.ScopeShared, Fields: []dsl.Field{
		{ID: "code", Name: "品番", Type: dsl.FieldString, Required: true},
		{ID: "price", Name: "価格", Type: dsl.FieldCurrency},
		{ID: "qty", Name: "数", Type: dsl.FieldInteger},
		{ID: "kind", Name: "種類", Type: dsl.FieldEnum},
		{ID: "at", Name: "日時", Type: dsl.FieldDatetime},
	}}}}
	e := newImportEnvApp(t, app, [][]string{
		{"品番", "価格", "数", "種類", "日時"},
		{"V-1", "¥1,200", "3.00", "なんでも", "2026-11-12"},
		{"V-2", "100.5", "2.5", "", "2026-11-12 06:00"},
		{"V-3", "", "1e19", "", "2026-11-12T06:00:00+09:00"},
	})
	run := importRun("ev1")
	run.Location = time.FixedZone("EST", -5*60*60)
	res, err := e.im.Import(t.Context(), e.app, run, importSpec)
	if err != nil {
		t.Fatal(err)
	}
	// 100.5 (currency), 2.5 (integer, not rounded), 1e19 (int64 bounds)
	if res.BadValues != 3 || res.Created != 3 {
		t.Fatalf("res = %+v", res)
	}
	got := e.records(t)
	v1, v2, v3 := got["V-1"], got["V-2"], got["V-3"]
	if v1["price"] != int64(1200) || v1["qty"] != int64(3) || v1["kind"] != "なんでも" || v1["at"] != "2026-11-12T00:00:00-05:00" {
		t.Fatalf("V-1 = %v", v1)
	}
	if _, ok := v2["price"]; ok || v2["at"] != "2026-11-12T06:00:00-05:00" {
		t.Fatalf("V-2 = %v", v2)
	}
	if _, ok := v3["qty"]; ok || v3["at"] != "2026-11-12T06:00:00+09:00" {
		t.Fatalf("V-3 = %v", v3)
	}
}
