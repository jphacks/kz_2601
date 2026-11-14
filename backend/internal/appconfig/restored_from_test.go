package appconfig

import (
	"net/http"
	"testing"
)

// #1774: only POST .../restore records restoredFrom; GET /versions returns it.
func TestVersionsRestoredFrom(t *testing.T) {
	e := newEnv(t)
	const app = "/v1/apps/RESTORED-FROM-1"
	if r := e.do("u1", http.MethodGet, app+"/dsl", nil, nil); r.code != http.StatusOK {
		t.Fatalf("init: %d %s", r.code, r.raw)
	}
	// v2: normal save
	if r := e.do("u1", http.MethodPut, app+"/dsl", putBody(kakeibo(t)), map[string]string{"If-Match": `"1"`}); r.code != http.StatusCreated {
		t.Fatalf("put: %d %s", r.code, r.raw)
	}
	// v3: a PUT claiming to be a restore is saved as manual without restoredFrom
	spoof := `{"dsl":` + string(kakeibo(t)) + `,"source":{"kind":"restore","restoredFrom":1}}`
	if r := e.do("u1", http.MethodPut, app+"/dsl", spoof, map[string]string{"If-Match": `"2"`}); r.code != http.StatusCreated {
		t.Fatalf("spoof put: %d %s", r.code, r.raw)
	}
	// v4: a PATCH carrying restoredFrom loses it too
	patch := `{"patch":[{"op":"add","path":"/entities/0/fields/-","value":{"id":"shop","name":"店名","type":"string"}}],"source":{"kind":"ai","restoredFrom":2}}`
	if r := e.do("u1", http.MethodPatch, app+"/dsl", patch, map[string]string{"If-Match": `"3"`}); r.code != http.StatusCreated {
		t.Fatalf("patch: %d %s", r.code, r.raw)
	}
	// v5: real restore of v2
	if r := e.do("u1", http.MethodPost, app+"/versions/2/restore", nil, nil); r.code != http.StatusCreated || r.version() != 5 {
		t.Fatalf("restore: %d %s", r.code, r.raw)
	}

	r := e.do("u1", http.MethodGet, app+"/versions", nil, nil)
	items, _ := r.body["items"].([]any)
	if r.code != http.StatusOK || len(items) != 5 {
		t.Fatalf("versions: %d %s", r.code, r.raw)
	}
	want := map[float64]struct {
		kind string
		from any
	}{
		5: {"restore", float64(2)},
		4: {"ai", nil},
		3: {"manual", nil},
		2: {"ai", nil},
	}
	for _, it := range items {
		m := it.(map[string]any)
		w, ok := want[m["version"].(float64)]
		if !ok {
			continue
		}
		src, _ := m["source"].(map[string]any)
		if src["kind"] != w.kind || src["restoredFrom"] != w.from {
			t.Fatalf("v%v source = %v, want kind=%s restoredFrom=%v", m["version"], src, w.kind, w.from)
		}
	}
}
