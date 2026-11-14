package appconfig

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
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
		{"android both set", map[string]string{EnvMinAndroidBuild: "7", EnvLatestAndroidBuild: " 9 "}, ClientConfigValues{MinAndroidBuild: 7, LatestAndroidBuild: 9}},
		{"android invalid ignored", map[string]string{EnvMinAndroidBuild: "1.0", EnvLatestAndroidBuild: "0"}, ClientConfigValues{}},
		{"android latest below min is raised", map[string]string{EnvMinAndroidBuild: "20", EnvLatestAndroidBuild: "10"}, ClientConfigValues{MinAndroidBuild: 20, LatestAndroidBuild: 20}},
		{"android and ios are independent", map[string]string{EnvMinIOSBuild: "30", EnvLatestAndroidBuild: "5"}, ClientConfigValues{MinIOSBuild: 30, LatestAndroidBuild: 5}},
		{"android store url", map[string]string{EnvAndroidStoreURL: "https://play.google.com/store/apps/details?id=jp.kz2601.metamon"}, ClientConfigValues{AndroidStoreURL: "https://play.google.com/store/apps/details?id=jp.kz2601.metamon"}},
		{"android market url", map[string]string{EnvAndroidStoreURL: "market://details?id=jp.kz2601.metamon"}, ClientConfigValues{AndroidStoreURL: "market://details?id=jp.kz2601.metamon"}},
		{"android bad store url", map[string]string{EnvAndroidStoreURL: "itms-apps://apps.apple.com/app/id1"}, ClientConfigValues{}},
		{"macos both set", map[string]string{EnvMinMacOSBuild: "40", EnvLatestMacOSBuild: " 42 "}, ClientConfigValues{MinMacOSBuild: 40, LatestMacOSBuild: 42}},
		{"macos invalid ignored", map[string]string{EnvMinMacOSBuild: "0", EnvLatestMacOSBuild: "x"}, ClientConfigValues{}},
		{"macos latest below min is raised", map[string]string{EnvMinMacOSBuild: "20", EnvLatestMacOSBuild: "10"}, ClientConfigValues{MinMacOSBuild: 20, LatestMacOSBuild: 20}},
		{"macos is independent of ios", map[string]string{EnvMinIOSBuild: "30", EnvMinMacOSBuild: "5"}, ClientConfigValues{MinIOSBuild: 30, MinMacOSBuild: 5}},
		{"macos store url", map[string]string{EnvMacAppStoreURL: "https://apps.apple.com/app/id123"}, ClientConfigValues{MacAppStoreURL: "https://apps.apple.com/app/id123"}},
		{"macos macappstore url", map[string]string{EnvMacAppStoreURL: "macappstore://apps.apple.com/app/id123"}, ClientConfigValues{MacAppStoreURL: "macappstore://apps.apple.com/app/id123"}},
		{"macos bad store url", map[string]string{EnvMacAppStoreURL: "market://details?id=x"}, ClientConfigValues{}},
		{"windows both set", map[string]string{EnvMinWindowsBuild: "50", EnvLatestWindowsBuild: " 52 "}, ClientConfigValues{MinWindowsBuild: 50, LatestWindowsBuild: 52}},
		{"windows invalid ignored", map[string]string{EnvMinWindowsBuild: "0", EnvLatestWindowsBuild: "1.2"}, ClientConfigValues{}},
		{"windows latest below min is raised", map[string]string{EnvMinWindowsBuild: "20", EnvLatestWindowsBuild: "10"}, ClientConfigValues{MinWindowsBuild: 20, LatestWindowsBuild: 20}},
		{"linux both set", map[string]string{EnvMinLinuxBuild: "60", EnvLatestLinuxBuild: "61"}, ClientConfigValues{MinLinuxBuild: 60, LatestLinuxBuild: 61}},
		{"linux invalid ignored", map[string]string{EnvMinLinuxBuild: "-1", EnvLatestLinuxBuild: "x"}, ClientConfigValues{}},
		{"linux latest below min is raised", map[string]string{EnvMinLinuxBuild: "30", EnvLatestLinuxBuild: "3"}, ClientConfigValues{MinLinuxBuild: 30, LatestLinuxBuild: 30}},
		{"desktop platforms are independent", map[string]string{EnvMinMacOSBuild: "40", EnvMinWindowsBuild: "5", EnvLatestLinuxBuild: "7"}, ClientConfigValues{MinMacOSBuild: 40, MinWindowsBuild: 5, LatestLinuxBuild: 7}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClientConfigFromEnv(envOf(tt.env)); !reflect.DeepEqual(got, tt.want) {
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
	body = get(ClientConfigValues{MinAndroidBuild: 4, LatestAndroidBuild: 6, AndroidStoreURL: "https://play.google.com/store/apps/details?id=x"})
	if len(body) != 3 || body["minAndroidBuild"] != float64(4) || body["latestAndroidBuild"] != float64(6) || body["androidStoreUrl"] != "https://play.google.com/store/apps/details?id=x" {
		t.Fatalf("android body: %v", body)
	}
	body = get(ClientConfigValues{MinMacOSBuild: 8, LatestMacOSBuild: 9, MacAppStoreURL: "https://apps.apple.com/app/id1"})
	if len(body) != 3 || body["minMacOSBuild"] != float64(8) || body["latestMacOSBuild"] != float64(9) || body["macAppStoreUrl"] != "https://apps.apple.com/app/id1" {
		t.Fatalf("macos body: %v", body)
	}
	body = get(ClientConfigValues{MinWindowsBuild: 10, LatestWindowsBuild: 11, MinLinuxBuild: 12, LatestLinuxBuild: 13})
	if len(body) != 4 || body["minWindowsBuild"] != float64(10) || body["latestWindowsBuild"] != float64(11) || body["minLinuxBuild"] != float64(12) || body["latestLinuxBuild"] != float64(13) {
		t.Fatalf("windows/linux body: %v", body)
	}
}

func TestNoticeFromEnv(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want *Notice
	}{
		{"unset", "", nil},
		{"minimal defaults to info", `{"id":"n1","message":{"ja":"お知らせ"}}`,
			&Notice{ID: "n1", Severity: NoticeInfo, Message: NoticeText{Ja: "お知らせ"}}},
		{"full", `{"id":"2026-11-02-ai","severity":"warning","message":{"ja":" 生成を\n止めています ","en":"Paused"},"linkUrl":"https://example.com/s","minIOSBuild":3,"maxIOSBuild":9}`,
			&Notice{ID: "2026-11-02-ai", Severity: NoticeWarning, Message: NoticeText{Ja: "生成を 止めています", En: "Paused"}, LinkURL: "https://example.com/s", MinIOSBuild: 3, MaxIOSBuild: 9}},
		{"not json", `{"id":`, nil},
		{"unknown field", `{"id":"n1","message":{"ja":"x"},"link":"https://e.com"}`, nil},
		{"missing id", `{"message":{"ja":"x"}}`, nil},
		{"bad id", `{"id":"a b","message":{"ja":"x"}}`, nil},
		{"bad severity", `{"id":"n1","severity":"critical","message":{"ja":"x"}}`, nil},
		{"missing ja", `{"id":"n1","message":{"en":"x"}}`, nil},
		{"blank ja", `{"id":"n1","message":{"ja":"  \n "}}`, nil},
		{"too long", `{"id":"n1","message":{"ja":"` + strings.Repeat("あ", 201) + `"}}`, nil},
		{"http link", `{"id":"n1","message":{"ja":"x"},"linkUrl":"http://e.com"}`, nil},
		{"script link", `{"id":"n1","message":{"ja":"x"},"linkUrl":"javascript:alert(1)"}`, nil},
		{"max below min", `{"id":"n1","message":{"ja":"x"},"minIOSBuild":5,"maxIOSBuild":4}`, nil},
		{"negative build", `{"id":"n1","message":{"ja":"x"},"minIOSBuild":-1}`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NoticeFromEnv(envOf(map[string]string{EnvNotice: tt.raw}))
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestClientConfigServesNotice(t *testing.T) {
	v := ClientConfigFromEnv(envOf(map[string]string{
		EnvMinIOSBuild: "4",
		EnvNotice:      `{"id":"n1","severity":"warning","message":{"ja":"止めています","en":"Paused"},"maxIOSBuild":20}`,
	}))
	rec := httptest.NewRecorder()
	ClientConfig(v).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/client-config", nil))
	want := `{"minIOSBuild":4,"notice":{"id":"n1","severity":"warning","message":{"ja":"止めています","en":"Paused"},"maxIOSBuild":20}}`
	if got := strings.TrimSpace(rec.Body.String()); got != want {
		t.Fatalf("body:\n got %s\nwant %s", got, want)
	}

	// an invalid notice must not take the update fields down with it
	v = ClientConfigFromEnv(envOf(map[string]string{EnvMinIOSBuild: "4", EnvNotice: "{broken"}))
	if v.MinIOSBuild != 4 || v.Notice != nil {
		t.Fatalf("got %+v", v)
	}
}
