package appconfig

import (
	"net/http"
	"testing"
)

// singleDSL has a single settings entity (#267) next to an ordinary log.
const singleDSL = `{
  "schemaVersion": 1,
  "app": {"name": "生理の記録", "voiceEntry": "floating"},
  "initialScreen": "home",
  "entities": [
    {"id": "periods", "name": "生理", "fields": [{"id": "start_on", "name": "開始日", "type": "date"}]},
    {"id": "settings", "name": "設定", "single": true, "fields": [
      {"id": "cycle_days", "name": "周期", "type": "integer", "defaultValue": 28},
      {"id": "note", "name": "メモ", "type": "string"}
    ]}
  ],
  "screens": [
    {"id": "home", "title": "記録", "toolbar": [{"label": "周期", "action": {"type": "navigate", "screen": "settings_form"}}],
     "components": [{"type": "stat", "label": "周期", "value": {"setting": "cycle_days"}}]},
    {"id": "settings_form", "title": "周期の設定", "entity": "settings",
     "toolbar": [{"label": "保存", "action": {"type": "saveRecord", "entity": "settings"}}],
     "components": [{"type": "numberField", "field": "cycle_days", "label": "周期"}]}
  ]
}`

// #267: a single entity keeps one record per user under a fixed id; creating
// it again overwrites (another device, an older build that adds).
func TestSingleEntityUpsert(t *testing.T) {
	ownerStores(t, func(t *testing.T, e *env, sfx string) {
		u1 := "u1" + sfx
		app := "/v1/apps/SINGLE-APP" + sfx
		recs := app + "/entities/settings/records"
		e.ownApp(u1, app, []byte(singleDSL))

		r := e.do(u1, http.MethodPost, recs, `{"data":{"cycle_days":30,"note":"a"},"clientId":"c1"}`, nil)
		if r.code != http.StatusCreated {
			t.Fatalf("first create: %d %s", r.code, r.raw)
		}
		id := r.body["id"].(string)
		created := r.body["createdAt"]

		// a second create (other clientId) overwrites the same record
		r = e.do(u1, http.MethodPost, recs, `{"data":{"cycle_days":32},"clientId":"c2"}`, nil)
		if r.code != http.StatusOK || r.body["id"] != id || r.body["createdAt"] != created {
			t.Fatalf("second create must overwrite %s: %d %s", id, r.code, r.raw)
		}
		data := r.body["data"].(map[string]any)
		if data["cycle_days"] != float64(32) || data["note"] != nil {
			t.Fatalf("overwrite replaces the data: %v", data)
		}
		if r = e.do(u1, http.MethodGet, recs, nil, nil); r.code != http.StatusOK || len(r.body["items"].([]any)) != 1 {
			t.Fatalf("list: %d %s", r.code, r.raw)
		}

		// editing and deleting work as usual; after a delete a create starts over
		if r = e.do(u1, http.MethodPatch, recs+"/"+id, `{"data":{"note":"b"}}`, nil); r.code != http.StatusOK {
			t.Fatalf("patch: %d %s", r.code, r.raw)
		}
		if r = e.do(u1, http.MethodDelete, recs+"/"+id, nil, nil); r.code != http.StatusNoContent {
			t.Fatalf("delete: %d %s", r.code, r.raw)
		}
		if r = e.do(u1, http.MethodPost, recs, `{"data":{"cycle_days":29}}`, nil); r.code != http.StatusCreated || r.body["id"] != id {
			t.Fatalf("create after delete: %d %s", r.code, r.raw)
		}

		// an ordinary entity still adds
		logs := app + "/entities/periods/records"
		a := e.do(u1, http.MethodPost, logs, `{"data":{"start_on":"2026-10-01"}}`, nil)
		b := e.do(u1, http.MethodPost, logs, `{"data":{"start_on":"2026-10-29"}}`, nil)
		if a.code != http.StatusCreated || b.code != http.StatusCreated || a.body["id"] == b.body["id"] {
			t.Fatalf("periods: %d %d", a.code, b.code)
		}
	})
}
