package appconfig

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// publicAppDownStore fails GetPublicApp for one appId on demand.
type publicAppDownStore struct {
	store.Store
	failApp string
}

var errPublicAppDown = errors.New("injected public app failure")

func (s *publicAppDownStore) GetPublicApp(ctx context.Context, appID string) (*store.PublicApp, error) {
	if appID == s.failApp {
		return nil, errPublicAppDown
	}
	return s.Store.GetPublicApp(ctx, appID)
}

// #2572: matchApp resolves {appId} of every matching endpoint; each failure
// is the same answer on all of them, before the 18+ check.
func TestMatchAppFailurePaths(t *testing.T) {
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", "localhost:9099")
	mem := store.NewMemory()
	st := &publicAppDownStore{Store: mem}
	e := &env{t: t, h: auth.Middleware(nil, auth.ModeDev)(New(st))}

	e.ownApp("owner", "/v1/apps/MAF-SHARED", sharedKakeibo(t)) // published, no matching
	e.ownApp("owner", "/v1/apps/MAF-PRIVATE", kakeibo(t))      // never published
	e.ownApp("owner", "/v1/apps/MAF-HIDDEN", matchingDSL(t))
	if err := mem.SetPublicAppHidden(t.Context(), "MAF-HIDDEN", true, "運営が停止"); err != nil {
		t.Fatal(err)
	}
	members := strings.Replace(string(matchingDSL(t)), `"scope": "public",
      "match": "profile"`, `"scope": "shared",
      "match": "profile"`, 1)
	if members == string(matchingDSL(t)) {
		t.Fatal("matching.json changed")
	}
	e.ownApp("owner", "/v1/apps/MAF-MEMBERS", []byte(members))
	e.ownApp("owner", "/v1/apps/MAF-DOWN", matchingDSL(t))
	st.failApp = "MAF-DOWN"

	type want struct {
		code            int
		errCode, reason string
	}
	noMatching := want{http.StatusNotFound, "not_found", reasonNoMatching}
	for _, c := range []struct {
		name, app string
		want      want
	}{
		{"invalid appId", "null", want{http.StatusBadRequest, "bad_request", ""}},
		{"unknown app", "MAF-NONE", noMatching},
		{"unpublished app", "MAF-PRIVATE", noMatching},
		{"app without matching", "MAF-SHARED", noMatching},
		{"stopped by the operator", "MAF-HIDDEN", want{http.StatusForbidden, "unpublished", "運営が停止"}},
		{"shared profile, not a member", "MAF-MEMBERS", want{http.StatusForbidden, "forbidden", notMemberReason}},
		{"store error", "MAF-DOWN", want{http.StatusInternalServerError, "internal", ""}},
	} {
		base := "/v1/apps/" + c.app
		// "dave" never declared 18+: the app check answers first
		for _, ep := range []struct{ method, path, body string }{
			{http.MethodGet, base + "/interests", ""},
			{http.MethodGet, base + "/matches", ""},
			{http.MethodPost, base + "/interests", `{"recordId":"rec_a","fromRecordId":"rec_b"}`},
			{http.MethodPost, base + "/interests/alice/accept", `{}`},
			{http.MethodDelete, base + "/interests/alice", ""},
			{http.MethodDelete, base + "/matches/alice", ""},
		} {
			var body any
			if ep.body != "" {
				body = ep.body
			}
			r := e.do("dave", ep.method, ep.path, body, nil)
			if r.code != c.want.code || r.errCode() != c.want.errCode || r.reason() != c.want.reason {
				t.Errorf("%s: %s %s: %d %s", c.name, ep.method, ep.path, r.code, r.raw)
			}
			if strings.Contains(string(r.raw), errPublicAppDown.Error()) {
				t.Errorf("%s: cause leaked: %s", c.name, r.raw)
			}
		}
	}

	// a published matching app gets past matchApp: undeclared callers then
	// hear about the 18+ check instead
	e.ownApp("owner", "/v1/apps/MAF-OK", matchingDSL(t))
	if r := e.do("dave", http.MethodGet, "/v1/apps/MAF-OK/interests", nil, nil); r.code != http.StatusBadRequest || r.reason() != reasonAgeUnconfirmed {
		t.Errorf("matching app, undeclared: %d %s", r.code, r.raw)
	}
}
