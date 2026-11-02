package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/jphacks/kz_2601/backend/internal/entitlement"
)

// appStoreNotifications mounts App Store Server Notifications v2 (#315) on the
// public mux (no Firebase / App Check: Apple signs the payload). It shares the
// verifier and store of the entitlement API.
func appStoreNotifications(ents *entitlement.Service, logger *slog.Logger) func(*http.ServeMux) {
	ns, ok := ents.Store.(entitlement.NotifyStore)
	if !ok {
		logger.Error("entitlement store does not support app store notifications")
		os.Exit(1)
	}
	n := &entitlement.Notifications{Verifier: ents.Verifier, Store: ns, Log: logger}
	return func(mux *http.ServeMux) { entitlement.RegisterNotifications(mux, n) }
}
