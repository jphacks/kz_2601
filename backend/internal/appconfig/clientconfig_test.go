package appconfig

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestClientConfigFromEnv(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want ClientConfigValues
	}{
		{"unset = no enforcement", nil, ClientConfigValues{}},
		{"both set", map[string]string{EnvMinIOSBuild: "12", EnvLatestIOSBuild: " 15 "}, ClientConfigValues{MinIOSBuild: 12, LatestIOSBuild: 15}},
		{"invalid ignored", map[string]string{EnvMinIOSBuild: "1.0", EnvLatestIOSBuild: "-3"}, ClientConfigValues{}},
		{"latest below min is raised", map[string]string{EnvMinIOSBuild: "20", EnvLatestIOSBuild: "10"}, ClientConfigValues{MinIOSBuild: 20, LatestIOSBuild: 20}},
		{"store url", map[string]string{EnvIOSAppStoreURL: "https://apps.apple.com/app/id123"}, ClientConfigValues{IOSAppStoreURL: "https://apps.apple.com/app/id123"}},
		{"bad store url", map[string]string{EnvIOSAppStoreURL: "javascript:alert(1)"}, ClientConfigValues{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClientConfigFromEnv(envOf(tt.env)); got != tt.want {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestClientConfigHandler(t *testing.T) {
	get := func(v ClientConfigValues) map[string]any {
		rec := httptest.NewRecorder()
		ClientConfig(v).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/client-config", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d", rec.Code)
		}
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body
	}
	if body := get(ClientConfigValues{}); len(body) != 0 {
		t.Fatalf("unset values must be omitted (iOS: no enforcement): %v", body)
	}
	body := get(ClientConfigValues{MinIOSBuild: 3, LatestIOSBuild: 5, IOSAppStoreURL: "https://apps.apple.com/app/id1"})
	if body["minIOSBuild"] != float64(3) || body["latestIOSBuild"] != float64(5) || body["iosAppStoreUrl"] != "https://apps.apple.com/app/id1" {
		t.Fatalf("body: %v", body)
	}
}
