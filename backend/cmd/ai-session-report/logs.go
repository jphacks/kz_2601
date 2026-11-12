package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// logEvent is the subset of a structured log line the report uses. The
// backend logs metadata only (labels, counts, hashes), never what was said.
type logEvent struct {
	At        time.Time
	Event     string // ai.generate / ai.usage / http.request / app.opened ...
	Message   string
	UIDHash   string
	AppHash   string
	Operation string // ai.usage
	Mode      string // ai.generate
	Outcome   string
	ErrorCode string
	ErrorKind string
	Path      string // http.request
	Status    int
	Attempt   int
	Regens    int
	Duration  time.Duration // ai.generate duration_ms / http.request duration
	ClaudeMs  int64
	LatencyMs int64 // ai.usage
	FirstGen  bool
	AreaFB    bool
	Downgrade bool
	LintRetry bool
	ReplyKind string // ai.chat reply_kind
	Turn      int    // ai.chat turn (1-based within the conversation)
	Built     bool   // ai.chat clarify_ended_in_build
	// ai.generate retry_reasons (one label per rejected attempt, in order)
	// and validation_codes (dsl problem code -> problems, #1772).
	RetryReasons    []string
	ValidationCodes map[string]int
}

// Event names the report reads (backend/internal/ai, internal/httpx, internal/metrics).
const (
	evGenerate = "ai.generate"
	evUsage    = "ai.usage"
	evRequest  = "http.request"
	evOpened   = "app.opened"
	evChat     = "ai.chat"
)

// rawPayload decodes numbers loosely: slog writes int64 as JSON numbers,
// Cloud Logging may turn them into strings.
type rawPayload map[string]any

func (p rawPayload) str(k string) string {
	switch v := p[k].(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return ""
}

func (p rawPayload) num(k string) float64 {
	switch v := p[k].(type) {
	case float64:
		return v
	case string:
		f, _ := strconv.ParseFloat(v, 64)
		return f
	}
	return 0
}

func (p rawPayload) flag(k string) bool {
	switch v := p[k].(type) {
	case bool:
		return v
	case string:
		return v == "true"
	}
	return false
}

type rawEntry struct {
	Timestamp   string     `json:"timestamp"`
	JSONPayload rawPayload `json:"jsonPayload"`
}

// parseLogs reads `gcloud logging read --format=json` output (a JSON array of
// entries) or JSON lines (entries, or bare slog records with `time`).
// Lines whose uid_hash is not in keep are dropped at once, so other users'
// metadata is never held.
func parseLogs(r io.Reader, keep map[string]bool) ([]logEvent, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var raws []json.RawMessage
	if t := bytes.TrimSpace(data); len(t) > 0 && t[0] == '[' {
		if err := json.Unmarshal(t, &raws); err != nil {
			return nil, fmt.Errorf("decode JSON array: %w", err)
		}
	} else {
		sc := bufio.NewScanner(bytes.NewReader(data))
		sc.Buffer(make([]byte, 1<<20), 1<<24)
		for sc.Scan() {
			line := bytes.TrimSpace(sc.Bytes())
			if len(line) == 0 || line[0] != '{' {
				continue
			}
			raws = append(raws, append(json.RawMessage(nil), line...))
		}
		if err := sc.Err(); err != nil {
			return nil, err
		}
	}
	var out []logEvent
	for _, raw := range raws {
		ev, ok := decodeEntry(raw)
		if !ok || !keep[ev.UIDHash] {
			continue
		}
		out = append(out, ev)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out, nil
}

func decodeEntry(raw json.RawMessage) (logEvent, bool) {
	var e rawEntry
	if err := json.Unmarshal(raw, &e); err != nil {
		return logEvent{}, false
	}
	p, ts := e.JSONPayload, e.Timestamp
	if p == nil { // bare slog record
		if err := json.Unmarshal(raw, &p); err != nil {
			return logEvent{}, false
		}
		ts = p.str("time")
	}
	at, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return logEvent{}, false
	}
	ev := logEvent{
		At:        at,
		Event:     p.str("event"),
		Message:   firstNonEmpty(p.str("message"), p.str("msg")),
		UIDHash:   p.str("uid_hash"),
		AppHash:   p.str("app_hash"),
		Operation: p.str("operation"),
		Mode:      p.str("mode"),
		Outcome:   p.str("outcome"),
		ErrorCode: firstNonEmpty(p.str("error_code"), p.str("code")),
		ErrorKind: p.str("error_kind"),
		Path:      p.str("path"),
		Status:    int(p.num("status")),
		Attempt:   int(p.num("attempt")),
		Regens:    int(p.num("regenerations")),
		ClaudeMs:  int64(p.num("claude_ms")),
		LatencyMs: int64(p.num("latency_ms")),
		FirstGen:  p.flag("first_generation"),
		AreaFB:    p.flag("area_fallback"),
		Downgrade: p.flag("downgraded_to_full"),
		LintRetry: p.flag("lint_retried"),
		ReplyKind: p.str("reply_kind"),
		Turn:      int(p.num("turn")),
		Built:     p.flag("clarify_ended_in_build"),
	}
	switch ev.Event {
	case evGenerate:
		ev.Duration = time.Duration(p.num("duration_ms")) * time.Millisecond
		ev.RetryReasons = parseLabels(p.str("retry_reasons"))
		ev.ValidationCodes = parseCodeCounts(p.str("validation_codes"))
	case evRequest:
		ev.Duration = parseDuration(p["duration"])
	case evChat:
		ev.Duration = time.Duration(p.num("duration_ms")) * time.Millisecond
	}
	if ev.Event == "" {
		return logEvent{}, false
	}
	return ev, true
}

// parseDuration reads slog.Duration: nanoseconds (JSON handler) or "1.2s".
func parseDuration(v any) time.Duration {
	switch d := v.(type) {
	case float64:
		return time.Duration(d)
	case string:
		if x, err := time.ParseDuration(d); err == nil {
			return x
		}
		if n, err := strconv.ParseFloat(d, 64); err == nil {
			return time.Duration(n)
		}
	}
	return 0
}

var labelRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,40}$`)

// parseLabels reads "a,b,a". Anything that is not a plain label is dropped.
func parseLabels(s string) []string {
	var out []string
	for _, l := range strings.Split(s, ",") {
		if l = strings.TrimSpace(l); labelRe.MatchString(l) {
			out = append(out, l)
		}
	}
	return out
}

// parseCodeCounts reads "code:n,code:n" (validation_codes).
func parseCodeCounts(s string) map[string]int {
	var out map[string]int
	for _, part := range strings.Split(s, ",") {
		code, n, ok := strings.Cut(strings.TrimSpace(part), ":")
		c, err := strconv.Atoi(n)
		if !ok || err != nil || c <= 0 || !labelRe.MatchString(code) {
			continue
		}
		if out == nil {
			out = map[string]int{}
		}
		out[code] += c
	}
	return out
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

// isChatTurn reports a clarify / chat-first Claude call (first attempt only:
// a retried HTTP attempt is the same turn).
func (e logEvent) isChatTurn() bool {
	return e.Event == evUsage && (e.Operation == "chat" || e.Operation == "chat_first") && e.Attempt <= 1
}

// isAIError reports an error response of the AI endpoints worth showing:
// 402 (Pro required), 429 (limit) and 5xx.
func (e logEvent) isAIError() bool {
	if e.Event != evRequest {
		return false
	}
	ai := strings.HasPrefix(e.Path, "/v1/chat") || strings.HasPrefix(e.Path, "/v1/generate") || strings.HasPrefix(e.Path, "/v1/text")
	return ai && (e.Status == 402 || e.Status == 429 || e.Status >= 500)
}
