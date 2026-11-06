package main

import (
	"strings"
	"testing"
	"time"
)

func TestParseLogsFormats(t *testing.T) {
	keep := map[string]bool{"aa": true}
	array := `[
 {"timestamp":"2026-11-05T01:00:00Z","jsonPayload":{"event":"ai.generate","outcome":"failed","error_code":"generation_failed","duration_ms":"4200","uid_hash":"aa","app_hash":"x1"}},
 {"timestamp":"2026-11-05T01:00:01Z","jsonPayload":{"event":"ai.generate","uid_hash":"bb"}},
 {"timestamp":"2026-11-05T01:00:02Z","textPayload":"not structured"}
]`
	evs, err := parseLogs(strings.NewReader(array), keep)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 || evs[0].Outcome != "failed" || evs[0].ErrorCode != "generation_failed" || evs[0].Duration != 4200*time.Millisecond {
		t.Fatalf("array: %+v", evs)
	}
	lines := `{"time":"2026-11-05T10:00:00+09:00","msg":"request","event":"http.request","path":"/v1/chat","status":402,"duration":2000000,"uid_hash":"aa"}
garbage
{"time":"2026-11-05T10:00:01+09:00","event":"ai.usage","operation":"chat_first","attempt":1,"uid_hash":"aa"}`
	evs, err = parseLogs(strings.NewReader(lines), keep)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 2 || !evs[0].isAIError() || evs[0].Duration != 2*time.Millisecond || !evs[1].isChatTurn() {
		t.Fatalf("lines: %+v", evs)
	}
	if (logEvent{Event: evRequest, Path: "/v1/apps", Status: 500}).isAIError() {
		t.Error("appconfig paths are not AI errors")
	}
}
