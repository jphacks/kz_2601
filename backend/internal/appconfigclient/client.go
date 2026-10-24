// Package appconfigclient is the thin client the ai service uses to read and
// write the current DSL. Two implementations: HTTP (real appconfig, Bearer
// forwarded) and Memory (in-process stub while appconfig is not deployed).
package appconfigclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Envelope mirrors DSLEnvelope in openapi-appconfig.yaml.
type Envelope struct {
	AppID   string          `json:"appId"`
	Version int             `json:"version"`
	DSL     json.RawMessage `json:"dsl"`
}

// Source mirrors VersionSource.
type Source struct {
	Kind         string `json:"kind"`
	Mode         string `json:"mode,omitempty"`
	Transcript   string `json:"transcript,omitempty"`
	Summary      string `json:"summary,omitempty"`
	Model        string `json:"model,omitempty"`
	RestoredFrom int    `json:"restoredFrom,omitempty"`
}

// VersionSummary mirrors VersionSummary (GET /versions item, no DSL body).
type VersionSummary struct {
	Version       int     `json:"version"`
	ParentVersion int     `json:"parentVersion,omitempty"`
	Source        *Source `json:"source,omitempty"`
}

// Store is what the ai service needs from appconfig.
type Store interface {
	Get(ctx context.Context, appID string) (*Envelope, error)
	Put(ctx context.Context, appID string, baseVersion int, doc json.RawMessage, src Source) (*Envelope, error)
	Patch(ctx context.Context, appID string, baseVersion int, ops []dsl.PatchOp, src Source) (*Envelope, error)
	// Versions lists history newest first (the first page is enough for undo).
	Versions(ctx context.Context, appID string) ([]VersionSummary, error)
	// Restore appends a new version whose DSL is that of version n.
	Restore(ctx context.Context, appID string, n int) (*Envelope, error)
}

// Error is an appconfig-side failure with its HTTP status.
type Error struct {
	Status         int
	Code           string
	Message        string
	CurrentVersion int // set on 409
}

func (e *Error) Error() string {
	return fmt.Sprintf("appconfig: %d %s: %s", e.Status, e.Code, e.Message)
}

type bearerKey struct{}

// WithBearer stores the caller's Authorization value so HTTP forwards it.
func WithBearer(ctx context.Context, authorization string) context.Context {
	return context.WithValue(ctx, bearerKey{}, authorization)
}

func bearerFrom(ctx context.Context) string {
	v, _ := ctx.Value(bearerKey{}).(string)
	return v
}

// ---- HTTP ------------------------------------------------------------------

// HTTP talks to a deployed appconfig at BaseURL (e.g. http://localhost:8080).
type HTTP struct {
	BaseURL string
	Client  *http.Client
}

// NewHTTP builds a client with a sane timeout.
func NewHTTP(baseURL string) *HTTP {
	return &HTTP{BaseURL: strings.TrimRight(baseURL, "/"), Client: &http.Client{Timeout: 10 * time.Second}}
}

func (h *HTTP) do(ctx context.Context, method, path string, headers map[string]string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, h.BaseURL+path, rd)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if auth := bearerFrom(ctx); auth != "" {
		req.Header.Set("Authorization", auth)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := h.Client.Do(req)
	if err != nil {
		return &Error{Status: http.StatusBadGateway, Code: "appconfig_unreachable", Message: err.Error()}
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		var e struct {
			Error struct {
				Code    string         `json:"code"`
				Message string         `json:"message"`
				Details map[string]any `json:"details"`
			} `json:"error"`
		}
		_ = json.Unmarshal(data, &e)
		ae := &Error{Status: resp.StatusCode, Code: e.Error.Code, Message: e.Error.Message}
		if ae.Code == "" {
			ae.Code = http.StatusText(resp.StatusCode)
		}
		if v, ok := e.Error.Details["currentVersion"].(float64); ok {
			ae.CurrentVersion = int(v)
		}
		return ae
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

// Get fetches the current DSL envelope.
func (h *HTTP) Get(ctx context.Context, appID string) (*Envelope, error) {
	var env Envelope
	if err := h.do(ctx, http.MethodGet, "/v1/apps/"+appID+"/dsl", nil, nil, &env); err != nil {
		return nil, err
	}
	return &env, nil
}

// Put replaces the whole DSL (PUT /dsl, If-Match).
func (h *HTTP) Put(ctx context.Context, appID string, baseVersion int, doc json.RawMessage, src Source) (*Envelope, error) {
	var out struct {
		Version int `json:"version"`
	}
	body := map[string]any{"dsl": doc, "source": src}
	if err := h.do(ctx, http.MethodPut, "/v1/apps/"+appID+"/dsl", ifMatch(baseVersion), body, &out); err != nil {
		return nil, err
	}
	return &Envelope{AppID: appID, Version: out.Version, DSL: doc}, nil
}

// Patch applies RFC 6902 ops (PATCH /dsl, If-Match); appconfig re-validates.
func (h *HTTP) Patch(ctx context.Context, appID string, baseVersion int, ops []dsl.PatchOp, src Source) (*Envelope, error) {
	var env Envelope
	body := map[string]any{"patch": ops, "source": src}
	if err := h.do(ctx, http.MethodPatch, "/v1/apps/"+appID+"/dsl", ifMatch(baseVersion), body, &env); err != nil {
		return nil, err
	}
	return &env, nil
}

// Versions is GET /versions (newest first, first page).
func (h *HTTP) Versions(ctx context.Context, appID string) ([]VersionSummary, error) {
	var out struct {
		Items []VersionSummary `json:"items"`
	}
	if err := h.do(ctx, http.MethodGet, "/v1/apps/"+appID+"/versions?pageSize=50", nil, nil, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

// Restore is POST /versions/{n}/restore followed by GET /dsl: appconfig
// answers with a VersionSummary (no DSL), so the new document is re-read.
func (h *HTTP) Restore(ctx context.Context, appID string, n int) (*Envelope, error) {
	var out struct {
		Version int `json:"version"`
	}
	if err := h.do(ctx, http.MethodPost, fmt.Sprintf("/v1/apps/%s/versions/%d/restore", appID, n), nil, nil, &out); err != nil {
		return nil, err
	}
	env, err := h.Get(ctx, appID)
	if err != nil {
		return nil, err
	}
	if env.Version != out.Version {
		return nil, &Error{Status: http.StatusConflict, Code: "version_conflict",
			Message: fmt.Sprintf("restored as version %d but current is %d", out.Version, env.Version), CurrentVersion: env.Version}
	}
	return env, nil
}

func ifMatch(v int) map[string]string {
	return map[string]string{"If-Match": fmt.Sprintf("%q", fmt.Sprint(v))}
}

// ---- Memory ----------------------------------------------------------------

// Memory is an in-process Store. Unknown appIds are seeded with dsl.Initial()
// at version 1 so a fresh ai process can be exercised without appconfig.
// Every write is kept as a snapshot so Versions/Restore behave like appconfig.
type Memory struct {
	mu   sync.Mutex
	apps map[string]*Envelope
	hist map[string][]memVersion // appID -> versions ascending
}

type memVersion struct {
	summary VersionSummary
	dsl     json.RawMessage
}

// NewMemory returns an empty stub store.
func NewMemory() *Memory {
	return &Memory{apps: map[string]*Envelope{}, hist: map[string][]memVersion{}}
}

func (m *Memory) getOrSeed(appID string) *Envelope {
	if e, ok := m.apps[appID]; ok {
		return e
	}
	raw, _ := json.Marshal(dsl.Initial())
	e := &Envelope{AppID: appID, Version: 1, DSL: raw}
	m.apps[appID] = e
	m.hist[appID] = []memVersion{{summary: VersionSummary{Version: 1, Source: &Source{Kind: "system", Mode: "full"}}, dsl: raw}}
	return e
}

// commit bumps the version, stores doc and records the snapshot. Caller holds mu.
func (m *Memory) commit(e *Envelope, doc json.RawMessage, src Source) {
	parent := e.Version
	e.Version++
	e.DSL = append(json.RawMessage(nil), doc...)
	m.hist[e.AppID] = append(m.hist[e.AppID], memVersion{
		summary: VersionSummary{Version: e.Version, ParentVersion: parent, Source: &src},
		dsl:     e.DSL,
	})
}

func (m *Memory) Get(_ context.Context, appID string) (*Envelope, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.getOrSeed(appID)
	cp := *e
	return &cp, nil
}

func (m *Memory) Put(_ context.Context, appID string, baseVersion int, doc json.RawMessage, src Source) (*Envelope, error) {
	if _, err := dsl.Parse(doc); err != nil {
		return nil, &Error{Status: http.StatusUnprocessableEntity, Code: "invalid_dsl", Message: err.Error()}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.getOrSeed(appID)
	if baseVersion != 0 && baseVersion != e.Version {
		return nil, &Error{Status: http.StatusConflict, Code: "version_conflict", Message: "base version is stale", CurrentVersion: e.Version}
	}
	m.commit(e, doc, src)
	cp := *e
	return &cp, nil
}

func (m *Memory) Patch(_ context.Context, appID string, baseVersion int, ops []dsl.PatchOp, src Source) (*Envelope, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.getOrSeed(appID)
	if baseVersion != 0 && baseVersion != e.Version {
		return nil, &Error{Status: http.StatusConflict, Code: "version_conflict", Message: "base version is stale", CurrentVersion: e.Version}
	}
	next, _, err := dsl.ApplyPatch(e.DSL, ops)
	if err != nil {
		return nil, &Error{Status: http.StatusUnprocessableEntity, Code: "invalid_dsl", Message: err.Error()}
	}
	m.commit(e, next, src)
	cp := *e
	return &cp, nil
}

// Versions returns the history newest first.
func (m *Memory) Versions(_ context.Context, appID string) ([]VersionSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getOrSeed(appID)
	h := m.hist[appID]
	out := make([]VersionSummary, 0, len(h))
	for i := len(h) - 1; i >= 0; i-- {
		out = append(out, h[i].summary)
	}
	return out, nil
}

// Restore appends version n's DSL as a new version (source.kind=restore).
func (m *Memory) Restore(_ context.Context, appID string, n int) (*Envelope, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.getOrSeed(appID)
	for _, v := range m.hist[appID] {
		if v.summary.Version == n {
			m.commit(e, v.dsl, Source{Kind: "restore", Mode: "full", RestoredFrom: n})
			cp := *e
			return &cp, nil
		}
	}
	return nil, &Error{Status: http.StatusNotFound, Code: "not_found", Message: fmt.Sprintf("version %d not found", n)}
}
