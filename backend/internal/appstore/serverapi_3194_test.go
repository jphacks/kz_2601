package appstore

import (
	"context"
	"strings"
	"testing"
)

// TestServerAPIBaseURL: production and sandbox default to Apple's hosts, a
// BaseURL entry overrides one environment only, and an unknown environment
// (Xcode, wrong case, empty) has no host.
func TestServerAPIBaseURL_3194(t *testing.T) {
	override := map[string]string{EnvSandbox: "http://127.0.0.1:9999"}
	for _, tc := range []struct {
		name string
		api  *ServerAPI
		env  string
		want string
	}{
		{"production default", &ServerAPI{}, EnvProduction, ServerAPIProduction},
		{"sandbox default", &ServerAPI{}, EnvSandbox, ServerAPISandbox},
		{"nil map", &ServerAPI{BaseURL: nil}, EnvProduction, ServerAPIProduction},
		{"empty map", &ServerAPI{BaseURL: map[string]string{}}, EnvSandbox, ServerAPISandbox},
		{"sandbox override", &ServerAPI{BaseURL: override}, EnvSandbox, "http://127.0.0.1:9999"},
		{"override leaves production alone", &ServerAPI{BaseURL: override}, EnvProduction, ServerAPIProduction},
		{"production override", &ServerAPI{BaseURL: map[string]string{EnvProduction: "http://p.test"}}, EnvProduction, "http://p.test"},
		{"override of an unknown environment", &ServerAPI{BaseURL: map[string]string{"Xcode": "http://x.test"}}, "Xcode", "http://x.test"},
		{"Xcode", &ServerAPI{}, "Xcode", ""},
		{"empty", &ServerAPI{}, "", ""},
		{"wrong case is unknown", &ServerAPI{}, "production", ""},
		{"padded is unknown", &ServerAPI{}, " Sandbox", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.api.baseURL(tc.env); got != tc.want {
				t.Fatalf("baseURL(%q) = %q, want %q", tc.env, got, tc.want)
			}
		})
	}
	if ServerAPIProduction == ServerAPISandbox || !strings.HasPrefix(ServerAPISandbox, "https://") {
		t.Fatal("production and sandbox hosts must differ and be https")
	}
}

// An unknown environment is refused before any key or network is touched.
func TestServerAPIUnknownEnvironment_3194(t *testing.T) {
	_, err := (&ServerAPI{}).SubscriptionStatuses(context.Background(), "Xcode", "1")
	if err == nil || !strings.Contains(err.Error(), `unknown environment "Xcode"`) {
		t.Fatalf("err = %v", err)
	}
}
