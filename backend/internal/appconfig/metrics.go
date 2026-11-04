package appconfig

import (
	"log/slog"
	"net/http"

	"github.com/jphacks/kz_2601/backend/internal/metrics"
)

// emitMetric writes a product-metric event (#297) for the caller.
func emitMetric(r *http.Request, event string, attrs ...slog.Attr) {
	metrics.Emit(r.Context(), event, uidOf(r), attrs...)
}

func appHashAttr(appID string) slog.Attr { return slog.String("app_hash", metrics.AppHash(appID)) }
