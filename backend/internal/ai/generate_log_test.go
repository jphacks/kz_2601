package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/appconfigclient"
	"github.com/jphacks/kz_2601/backend/internal/auth"
)

// genLog runs fn with a JSON logger on svc and returns the single ai.generate line.
func genLog(t *testing.T, svc *Service, fn func()) (map[string]any, string) {
	t.Helper()
	var buf bytes.Buffer
	svc.Logger = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	fn()
	var lines []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("not JSON: %q", l)
		}
		if m["event"] == GenerateEvent {
			lines = append(lines, m)
		}
	}
	if len(lines) != 1 {
		t.Fatalf("want 1 ai.generate line, got %d: %s", len(lines), buf.String())
	}
	return lines[0], buf.String()
}

func TestGenerateLog_FirstCreateValidated(t *testing.T) {
	kb := kakeibo(t)
	svc, _ := newSvc(&fakeCaller{replies: []string{string(kb)}})
	ctx := auth.ContextWithUID(context.Background(), rawUID)
	m, raw := genLog(t, svc, func() {
		if _, err := svc.Generate(ctx, Request{AppID: "app-RAW-1", Transcript: secretPrompt}); err != nil {
			t.Fatal(err)
		}
	})
	want := map[string]any{
		"msg": GenerateEvent, "level": "INFO", "mode": "create", "outcome": GenValidated,
		"validation_passed": true, "first_generation": true, "stream": false,
		"attempts": float64(1), "regenerations": float64(0), "uid_hash": UIDHash(rawUID),
	}
	for k, v := range want {
		if m[k] != v {
			t.Errorf("%s = %v, want %v", k, m[k], v)
		}
	}
	if _, ok := m["duration_ms"].(float64); !ok {
		t.Errorf("duration_ms missing: %v", m)
	}
	if _, ok := m["error_code"]; ok {
		t.Errorf("error_code on success: %v", m)
	}
	// no transcript, summary, DSL, appId or raw uid
	for _, leak := range []string{"SECRET-PROMPT", "app-RAW-1", rawUID, "家計簿", "entities"} {
		if strings.Contains(raw, leak) {
			t.Errorf("log leaks %q: %s", leak, raw)
		}
	}
}

func TestGenerateLog_Regenerated(t *testing.T) {
	kb := kakeibo(t)
	svc, _ := newSvc(&fakeCaller{replies: []string{withOverlap(t, kb), string(kb)}})
	m, _ := genLog(t, svc, func() {
		if _, err := svc.Generate(context.Background(), Request{AppID: "a1", Transcript: "家計簿アプリを作って"}); err != nil {
			t.Fatal(err)
		}
	})
	if m["outcome"] != GenRegenerated || m["validation_passed"] != true || m["regenerations"] != float64(1) {
		t.Fatalf("line: %v", m)
	}
	if _, ok := m["uid_hash"]; ok {
		t.Fatalf("anonymous call has uid_hash: %v", m)
	}
}

func TestGenerateLog_ValidationFailed(t *testing.T) {
	bad := withBadEntity(t, kakeibo(t))
	svc, _ := newSvc(&fakeCaller{replies: []string{bad, bad, bad}})
	m, _ := genLog(t, svc, func() {
		if _, err := svc.Generate(context.Background(), Request{AppID: "a1", Transcript: "家計簿"}); err == nil {
			t.Fatal("want error")
		}
	})
	if m["outcome"] != GenFailed || m["validation_passed"] != false || m["error_code"] != "generation_failed" ||
		m["attempts"] != float64(3) || m["level"] != "WARN" || m["first_generation"] != true {
		t.Fatalf("line: %v", m)
	}
}

func TestGenerateLog_EditIsNotFirst(t *testing.T) {
	kb := kakeibo(t)
	store := appconfigclient.NewMemory()
	if _, err := store.Put(context.Background(), "a1", 1, kb, appconfigclient.Source{Kind: "system"}); err != nil {
		t.Fatal(err)
	}
	goodPatch := `{"patch":[{"op":"add","path":"/entities/0/fields/1/options/-","value":"カフェ"}],"summary":"カフェを追加しました"}`
	svc := &Service{Claude: &fakeCaller{replies: []string{goodPatch}}, Store: store, Model: "test-model"}
	m, raw := genLog(t, svc, func() {
		if _, err := svc.Generate(context.Background(), Request{AppID: "a1", Transcript: "カテゴリにカフェを追加して"}); err != nil {
			t.Fatal(err)
		}
	})
	if m["mode"] != "edit" || m["first_generation"] != false || m["validation_passed"] != true {
		t.Fatalf("line: %v", m)
	}
	if strings.Contains(raw, "カフェ") {
		t.Fatalf("log leaks summary: %s", raw)
	}
}

func TestGenerateLog_BadRequest(t *testing.T) {
	svc, _ := newSvc(&fakeCaller{})
	m, _ := genLog(t, svc, func() {
		if _, err := svc.Generate(context.Background(), Request{AppID: "a1"}); err == nil {
			t.Fatal("want error")
		}
	})
	if m["mode"] != "unknown" || m["outcome"] != GenBadRequest || m["validation_passed"] != false || m["level"] != "INFO" {
		t.Fatalf("line: %v", m)
	}
}

func TestGenerateOutcome(t *testing.T) {
	tests := []struct {
		name string
		res  *Result
		err  error
		want string
	}{
		{"refused", nil, &Error{Status: http.StatusBadRequest, Code: "refused"}, GenRefused},
		{"claude 429 is ours", nil, &Error{Status: http.StatusTooManyRequests, Code: "rate_limited"}, GenFailed},
		{"timeout", nil, &Error{Status: http.StatusGatewayTimeout, Code: "upstream_timeout"}, GenFailed},
		{"not found", nil, &Error{Status: http.StatusNotFound, Code: "not_found"}, GenBadRequest},
		{"plain error", nil, context.Canceled, GenFailed},
		{"undo", &Result{Mode: "restore"}, nil, GenRestored},
		{"unchanged", &Result{Mode: "patch", Unchanged: true}, nil, GenUnchanged},
		{"lint retry", &Result{Mode: "full", Meta: Meta{LintRetried: true}}, nil, GenRegenerated},
		{"area fallback", &Result{Mode: "full", Meta: Meta{AreaFallback: true}}, nil, GenRegenerated},
		{"clean", &Result{Mode: "full"}, nil, GenValidated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := generateOutcome(tt.res, tt.err); got != tt.want {
				t.Fatalf("got %s, want %s", got, tt.want)
			}
		})
	}
}
