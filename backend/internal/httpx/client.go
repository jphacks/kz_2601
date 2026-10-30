package httpx

import (
	"log/slog"
	"net/http"
)

// Client identity headers (#302). iOS sends them on every request so the
// logs show which builds still call the backend (when to raise the minimum
// supported build). They are informational: component filtering is driven by
// the allowedComponents body field, not by these.
const (
	ClientPlatformHeader   = "X-Client-Platform"    // ios
	ClientVersionHeader    = "X-Client-Version"     // CFBundleShortVersionString (1.0.0)
	ClientBuildHeader      = "X-Client-Build"       // CFBundleVersion (12)
	ClientDSLVersionHeader = "X-Client-DSL-Version" // highest DSL schemaVersion the client renders
)

// clientAttrs returns the client headers present on r as log attributes.
func clientAttrs(r *http.Request) []any {
	var out []any
	for _, h := range []struct{ key, header string }{
		{"clientPlatform", ClientPlatformHeader},
		{"clientVersion", ClientVersionHeader},
		{"clientBuild", ClientBuildHeader},
		{"clientDslVersion", ClientDSLVersionHeader},
	} {
		if v := r.Header.Get(h.header); v != "" {
			if len(v) > 32 {
				v = v[:32]
			}
			out = append(out, slog.String(h.key, v))
		}
	}
	return out
}
