package main

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/appconfig"
	"github.com/jphacks/kz_2601/backend/internal/appstore"
	"github.com/jphacks/kz_2601/backend/internal/entitlement"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// Apple calls the notification URL without Firebase or App Check: it must not
// be behind the authenticated chain, and other methods stay 404/405.
func TestAppStoreNotificationsArePublic(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	ents := &entitlement.Service{Verifier: &appstore.Verifier{Policy: appstore.Policy{
		BundleIDs: []string{"b"}, ProductIDs: []string{"p"}, Environments: entitlement.DefaultEnvironments,
	}}, Store: entitlement.NewMemory()}
	denyAll := func(http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) })
	}
	h := newHandler(denyAll, logger, appconfig.New(store.NewMemory()), nil, appStoreNotifications(ents, logger))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, entitlement.NotificationsPath, strings.NewReader(`{"signedPayload":"a.b.c"}`)))
	// reached the handler (not signed by Apple), not the auth chain
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("POST status = %d (body %s)", rec.Code, rec.Body)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, entitlement.NotificationsPath, nil))
	if rec.Code == http.StatusOK || rec.Code == http.StatusUnauthorized {
		t.Fatalf("GET status = %d", rec.Code)
	}
}
