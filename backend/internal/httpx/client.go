package httpx

import (
	"log/slog"
	"net/http"
	"strings"
)

// Client identity headers (#302). iOS sends them on every request so the
// logs show which builds still call the backend (when to raise the minimum
// supported build). They are informational: component filtering is driven by
// the allowedComponents body field, not by these.
const (
	ClientPlatformHeader   = "X-Client-Platform"    // ios / android / macos / windows / linux
	ClientVersionHeader    = "X-Client-Version"     // CFBundleShortVersionString (1.0.0)
	ClientBuildHeader      = "X-Client-Build"       // CFBundleVersion (12)
	ClientDSLVersionHeader = "X-Client-DSL-Version" // highest DSL schemaVersion the client renders
)

// Platform vocabulary of X-Client-Platform (#2132). Anything else is PlatformOther.
const (
	PlatformIOS     = "ios"
	PlatformAndroid = "android"
	PlatformMacOS   = "macos"
	PlatformWindows = "windows"
	PlatformLinux   = "linux"
	PlatformOther   = "other"
)

// Platforms lists the whole vocabulary, PlatformOther last.
var Platforms = []string{PlatformIOS, PlatformAndroid, PlatformMacOS, PlatformWindows, PlatformLinux, PlatformOther}

// maxClientHeader caps a logged client header value.
const maxClientHeader = 32

// NormalizePlatform maps a raw X-Client-Platform value to the vocabulary:
// case and surrounding spaces are ignored; empty, unknown or over-long values
// are PlatformOther.
func NormalizePlatform(v string) string {
	if len(v) > maxClientHeader {
		return PlatformOther
	}
	switch p := strings.ToLower(strings.TrimSpace(v)); p {
	case PlatformIOS, PlatformAndroid, PlatformMacOS, PlatformWindows, PlatformLinux:
		return p
	}
	return PlatformOther
}

// ClientPlatform is the normalised X-Client-Platform of r.
func ClientPlatform(r *http.Request) string {
	return NormalizePlatform(r.Header.Get(ClientPlatformHeader))
}

// ClientAttrs returns the client headers present on r as log attributes.
// clientPlatform is always set (normalised; a missing header is "other").
func ClientAttrs(r *http.Request) []any {
	out := []any{slog.String("clientPlatform", ClientPlatform(r))}
	for _, h := range []struct{ key, header string }{
		{"clientVersion", ClientVersionHeader},
		{"clientBuild", ClientBuildHeader},
		{"clientDslVersion", ClientDSLVersionHeader},
	} {
		if v := r.Header.Get(h.header); v != "" {
			if len(v) > maxClientHeader {
				v = v[:maxClientHeader]
			}
			out = append(out, slog.String(h.key, v))
		}
	}
	return out
}
