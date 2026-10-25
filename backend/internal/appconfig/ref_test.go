package appconfig

import (
	"net/http"
	"os"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// 旅行の割り勘(stg 9049ed9c…): メンバーを 2 人作った直後に、払った人(type ref・required)付きの支出を保存する。
// iOS の FormDraft.payload と同じ形(computed は送らない・date は YYYY-MM-DD・payer はメンバーの record id)。
// 以前は normalizeValue に ref の分岐が無く `field "payer" has unsupported type "ref"` の 422 になっていた。
func TestRecordsRefFieldWarikan(t *testing.T) {
	e := newEnv(t)
	const app = "/v1/apps/dev-app"
	b, err := os.ReadFile("testdata/warikan.json")
	if err != nil {
		t.Fatal(err)
	}
	e.do("u1", http.MethodGet, app+"/dsl", nil, nil)
	if r := e.do("u1", http.MethodPut, app+"/dsl", putBody(b), map[string]string{"If-Match": `"1"`}); r.code != http.StatusCreated {
		t.Fatalf("put: %d %s", r.code, r.raw)
	}

	var ids []string
	for _, name := range []string{"たろう", "はなこ"} {
		r := e.do("u1", http.MethodPost, app+"/entities/members/records", map[string]any{"data": map[string]any{"name": name}, "clientId": "m-" + name}, nil)
		if r.code != http.StatusCreated {
			t.Fatalf("member %s: %d %s", name, r.code, r.raw)
		}
		ids = append(ids, r.body["id"].(string))
	}

	exp := app + "/entities/expenses/records"
	r := e.do("u1", http.MethodPost, exp, map[string]any{"data": map[string]any{
		"amount": 3000, "category": "食費", "payer": ids[1], "date": "2026-10-25", "memo": "夕食",
	}, "clientId": "e-1"}, nil)
	if r.code != http.StatusCreated {
		t.Fatalf("expense with ref payer: %d %s", r.code, r.raw)
	}
	if got := r.body["data"].(map[string]any)["payer"]; got != ids[1] {
		t.Errorf("payer = %v, want %s", got, ids[1])
	}
	expID := r.body["id"].(string)

	// defaultValue(category 交通費・date now)だけで payer を付けても通る
	if r := e.do("u1", http.MethodPost, exp, map[string]any{"data": map[string]any{"amount": 500, "payer": ids[0]}}, nil); r.code != http.StatusCreated {
		t.Fatalf("expense with defaults: %d %s", r.code, r.raw)
	}

	// PATCH で払った人を差し替えられる
	r = e.do("u1", http.MethodPatch, exp+"/"+expID, map[string]any{"data": map[string]any{"payer": ids[0]}}, map[string]string{"If-Match": `"1"`})
	if r.code != http.StatusOK {
		t.Fatalf("patch payer: %d %s", r.code, r.raw)
	}
	if got := r.body["data"].(map[string]any)["payer"]; got != ids[0] {
		t.Errorf("patched payer = %v, want %s", got, ids[0])
	}

	// required ref の欠落・空文字・型違いは 422 で details.field=payer
	for name, payer := range map[string]any{"missing": nil, "empty": "", "number": 1} {
		data := map[string]any{"amount": 100, "category": "食費", "date": "2026-10-25"}
		if payer != nil {
			data["payer"] = payer
		}
		r := e.do("u1", http.MethodPost, exp, map[string]any{"data": data}, nil)
		if r.code != http.StatusUnprocessableEntity || r.errCode() != "invalid_record" {
			t.Fatalf("%s payer: %d %s", name, r.code, r.raw)
		}
		details, _ := r.body["error"].(map[string]any)["details"].(map[string]any)
		if details["field"] != "payer" {
			t.Errorf("%s payer: details = %v", name, details)
		}
	}
}

func TestNormalizeValueRef(t *testing.T) {
	f := &dsl.Field{ID: "payer", Type: dsl.FieldRef, RefEntity: "members"}
	if v, err := normalizeValue(f, "rec_abc"); err != nil || v != "rec_abc" {
		t.Errorf("record id: %v %v", v, err)
	}
	// 端末の未送信の仮 id(local-<clientId>)も形としては受ける(参照先の存在は見ない)
	if _, err := normalizeValue(f, "local-0f0e"); err != nil {
		t.Errorf("local id: %v", err)
	}
	for _, bad := range []any{"", "   ", 12, true, map[string]any{"id": "x"}, string(make([]byte, maxRefLen+1))} {
		if _, err := normalizeValue(f, bad); err == nil {
			t.Errorf("%v should be rejected", bad)
		}
	}
}

// #231: 「割り勘する人」(type refs)は id の配列。iOS が送る形(選んだメンバーの id・未選択はキーなし)で 422 にならない。
func TestRecordsRefsFieldWarikan(t *testing.T) {
	e := newEnv(t)
	const app = "/v1/apps/dev-app"
	b, err := os.ReadFile("../../../docs/dsl/examples/warikan.json")
	if err != nil {
		t.Fatal(err)
	}
	e.do("u1", http.MethodGet, app+"/dsl", nil, nil)
	if r := e.do("u1", http.MethodPut, app+"/dsl", putBody(b), map[string]string{"If-Match": `"1"`}); r.code != http.StatusCreated {
		t.Fatalf("put: %d %s", r.code, r.raw)
	}
	var ids []string
	for _, name := range []string{"たろう", "はなこ", "じろう"} {
		r := e.do("u1", http.MethodPost, app+"/entities/members/records", map[string]any{"data": map[string]any{"name": name}}, nil)
		if r.code != http.StatusCreated {
			t.Fatalf("member %s: %d %s", name, r.code, r.raw)
		}
		ids = append(ids, r.body["id"].(string))
	}
	exp := app + "/entities/expenses/records"

	// 2 人で割る(重複は 1 つに)
	r := e.do("u1", http.MethodPost, exp, map[string]any{"data": map[string]any{
		"title": "夕食", "amount": 3000, "payer": ids[0], "split_with": []any{ids[0], ids[1], ids[1]}, "date": "2026-10-25",
	}}, nil)
	if r.code != http.StatusCreated {
		t.Fatalf("expense with refs: %d %s", r.code, r.raw)
	}
	got, _ := r.body["data"].(map[string]any)["split_with"].([]any)
	if len(got) != 2 || got[0] != ids[0] || got[1] != ids[1] {
		t.Errorf("split_with = %v", got)
	}
	expID := r.body["id"].(string)

	// 未選択(キーなし)・空配列 = 全員
	for name, data := range map[string]map[string]any{
		"missing": {"title": "タクシー", "amount": 1200, "payer": ids[1]},
		"empty":   {"title": "宿", "amount": 9000, "payer": ids[2], "split_with": []any{}},
	} {
		if r := e.do("u1", http.MethodPost, exp, map[string]any{"data": data}, nil); r.code != http.StatusCreated {
			t.Fatalf("%s split_with: %d %s", name, r.code, r.raw)
		}
	}

	// PATCH で割り勘する人を変えられる(削除済みメンバーの id が残っていても形としては受ける)
	r = e.do("u1", http.MethodPatch, exp+"/"+expID, map[string]any{"data": map[string]any{"split_with": []any{ids[2], "rec_deleted"}}}, map[string]string{"If-Match": `"1"`})
	if r.code != http.StatusOK {
		t.Fatalf("patch split_with: %d %s", r.code, r.raw)
	}

	// 文字列 1 つ・数値の配列は 422(details.field=split_with)
	for name, v := range map[string]any{"string": ids[0], "numbers": []any{1, 2}} {
		r := e.do("u1", http.MethodPost, exp, map[string]any{"data": map[string]any{"title": "x", "amount": 1, "payer": ids[0], "split_with": v}}, nil)
		if r.code != http.StatusUnprocessableEntity || r.errCode() != "invalid_record" {
			t.Fatalf("%s split_with: %d %s", name, r.code, r.raw)
		}
		details, _ := r.body["error"].(map[string]any)["details"].(map[string]any)
		if details["field"] != "split_with" {
			t.Errorf("%s split_with: details = %v", name, details)
		}
	}
}

func TestNormalizeValueRefs(t *testing.T) {
	f := &dsl.Field{ID: "split_with", Type: dsl.FieldRefs, RefEntity: "members"}
	v, err := normalizeValue(f, []any{"a", "", "b", "a", "local-1"})
	if err != nil {
		t.Fatal(err)
	}
	if got := v.([]any); len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "local-1" {
		t.Errorf("got %v", got)
	}
	if v, err := normalizeValue(f, []any{}); err != nil || len(v.([]any)) != 0 {
		t.Errorf("empty: %v %v", v, err)
	}
	for _, bad := range []any{"a", 1, []any{1}, []any{map[string]any{"id": "x"}}, []any{string(make([]byte, maxRefLen+1))}, make([]any, maxRefs+1)} {
		if _, err := normalizeValue(f, bad); err == nil {
			t.Errorf("%v should be rejected", bad)
		}
	}
}
