package store

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
)

func TestRawToMap(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  json.RawMessage
		want map[string]any
	}{
		{"空のオブジェクト", json.RawMessage(`{}`), map[string]any{}},
		{"値のあるオブジェクト", json.RawMessage(`{"name":"app","version":2}`), map[string]any{"name": "app", "version": float64(2)}},
		{"null は nil", json.RawMessage(`null`), nil},
	} {
		got, err := rawToMap(tc.raw)
		if err != nil {
			t.Errorf("%s: rawToMap(%s) はエラーを返した: %v", tc.name, tc.raw, err)
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: rawToMap(%s) = %#v, want %#v", tc.name, tc.raw, got, tc.want)
		}
	}
}

func TestRawToMapRejectsWhatIsNotAnObject(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  json.RawMessage
	}{
		{"配列", json.RawMessage(`[1,2]`)},
		{"数", json.RawMessage(`3`)},
		{"文字", json.RawMessage(`"x"`)},
		{"空", json.RawMessage(``)},
	} {
		got, err := rawToMap(tc.raw)
		if err == nil {
			t.Errorf("%s: rawToMap(%s) = %#v, エラーが欲しい", tc.name, tc.raw, got)
			continue
		}
		if got != nil {
			t.Errorf("%s: rawToMap(%s) はエラーと一緒に %#v を返した, want nil", tc.name, tc.raw, got)
		}
		if want := "dsl is not an object"; !strings.Contains(err.Error(), want) {
			t.Errorf("%s: エラーの文 = %q, want %q を含む", tc.name, err.Error(), want)
		}
	}
}

func TestSourceToMapKeepsNilAsNil(t *testing.T) {
	got, err := sourceToMap(nil)
	if err != nil {
		t.Fatalf("sourceToMap(nil) はエラーを返した: %v", err)
	}
	if got != nil {
		t.Errorf("sourceToMap(nil) = %#v, want nil", got)
	}
}

func TestSourceToMap(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  *Source
		want map[string]any
	}{
		{"kind だけ", &Source{Kind: "system"}, map[string]any{"kind": "system"}},
		{
			"全部の欄",
			&Source{Kind: "ai", Mode: "full", Transcript: "t", Summary: "s", Model: "m", RestoredFrom: 3, PromptVersion: "v1"},
			map[string]any{"kind": "ai", "mode": "full", "transcript": "t", "summary": "s", "model": "m", "restoredFrom": float64(3), "promptVersion": "v1"},
		},
		{"欄が空", &Source{}, map[string]any{"kind": ""}},
	} {
		got, err := sourceToMap(tc.src)
		if err != nil {
			t.Errorf("%s: sourceToMap はエラーを返した: %v", tc.name, err)
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: sourceToMap(%#v) = %#v, want %#v", tc.name, tc.src, got, tc.want)
		}
	}
}

func TestHiddenUpdatesSetsThreeValuesWhenHiding(t *testing.T) {
	got := hiddenUpdates(true, "spam")
	if len(got) != 3 {
		t.Fatalf("hiddenUpdates(true, \"spam\") = %#v, want 3 件", got)
	}
	if want := []string{"hidden", "hiddenReason", "hiddenAt"}; !reflect.DeepEqual(updatePaths(got), want) {
		t.Errorf("path = %v, want %v", updatePaths(got), want)
	}
	if got[0].Value != true {
		t.Errorf("hidden = %#v, want true", got[0].Value)
	}
	if got[1].Value != "spam" {
		t.Errorf("hiddenReason = %#v, want %q", got[1].Value, "spam")
	}
	at, ok := got[2].Value.(time.Time)
	if !ok {
		t.Fatalf("hiddenAt = %#v, want time.Time", got[2].Value)
	}
	if at.IsZero() {
		t.Error("hiddenAt はゼロ値だった, want 時刻")
	}
	if at != at.Truncate(time.Microsecond) {
		t.Errorf("hiddenAt = %s, want マイクロ秒まで", at.Format(time.RFC3339Nano))
	}
}

func TestHiddenUpdatesKeepsAnEmptyReason(t *testing.T) {
	got := hiddenUpdates(true, "")
	if got[1].Value != "" {
		t.Errorf("hiddenReason = %#v, want %q", got[1].Value, "")
	}
}

func TestHiddenUpdatesDeletesThreeFieldsWhenUnhiding(t *testing.T) {
	for _, reason := range []string{"", "spam"} {
		got := hiddenUpdates(false, reason)
		if len(got) != 3 {
			t.Fatalf("hiddenUpdates(false, %q) = %#v, want 3 件", reason, got)
		}
		if want := []string{"hidden", "hiddenReason", "hiddenAt"}; !reflect.DeepEqual(updatePaths(got), want) {
			t.Errorf("reason=%q: path = %v, want %v", reason, updatePaths(got), want)
		}
		for _, u := range got {
			if u.Value != firestore.Delete {
				t.Errorf("reason=%q: %s = %#v, want firestore.Delete", reason, u.Path, u.Value)
			}
		}
	}
}

func updatePaths(us []firestore.Update) []string {
	out := make([]string, 0, len(us))
	for _, u := range us {
		out = append(out, u.Path)
	}
	return out
}

func TestNewRunnerMarkExpiresAfterTheTTL(t *testing.T) {
	got := newRunnerMark()
	if got.CreatedAt.IsZero() {
		t.Error("createdAt はゼロ値だった, want 時刻")
	}
	if got.CreatedAt != got.CreatedAt.Truncate(time.Microsecond) {
		t.Errorf("createdAt = %s, want マイクロ秒まで", got.CreatedAt.Format(time.RFC3339Nano))
	}
	if want := got.CreatedAt.Add(RunnerMarkTTL); !got.ExpireAt.Equal(want) {
		t.Errorf("expireAt = %s, want %s", got.ExpireAt.Format(time.RFC3339Nano), want.Format(time.RFC3339Nano))
	}
	if want := 7 * 24 * time.Hour; got.ExpireAt.Sub(got.CreatedAt) != want {
		t.Errorf("expireAt - createdAt = %s, want %s", got.ExpireAt.Sub(got.CreatedAt), want)
	}
}
