package appconfig

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestNoticePlatforms_2964(t *testing.T) {
	t.Parallel()
	const base = `"id":"n1","message":{"ja":"止めています"}`
	tests := []struct {
		name     string
		raw      string
		header   string
		wantShow bool
		wantVary bool
		wantNil  bool // notice rejected at parse time
	}{
		{"no platforms: ios", `{` + base + `}`, "ios", true, false, false},
		{"no platforms: no header", `{` + base + `}`, "", true, false, false},
		{"match", `{` + base + `,"platforms":["android","ios"]}`, "ios", true, true, false},
		{"match is case-insensitive", `{` + base + `,"platforms":["Android"]}`, " ANDROID ", true, true, false},
		{"no match", `{` + base + `,"platforms":["android"]}`, "ios", false, true, false},
		{"unknown OS in header", `{` + base + `,"platforms":["android"]}`, "plan9", false, true, false},
		{"no header with platforms", `{` + base + `,"platforms":["ios"]}`, "", false, true, false},
		{"unknown OS in config invalidates", `{` + base + `,"platforms":["plan9"]}`, "ios", false, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			v := ClientConfigFromEnv(envOf(map[string]string{EnvNotice: tt.raw}))
			if tt.wantNil {
				if v.Notice != nil {
					t.Fatalf("want nil notice, got %+v", v.Notice)
				}
				return
			}
			req := httptest.NewRequest(http.MethodGet, "/v1/client-config", nil)
			if tt.header != "" {
				req.Header.Set("X-Client-Platform", tt.header)
			}
			rec := httptest.NewRecorder()
			ClientConfig(v).ServeHTTP(rec, req)
			body := rec.Body.String()
			if shown := strings.Contains(body, `"notice"`); shown != tt.wantShow {
				t.Fatalf("notice shown = %v, want %v: %s", shown, tt.wantShow, body)
			}
			if strings.Contains(body, `"platforms"`) {
				t.Fatalf("platforms leaked to the client: %s", body)
			}
			if got := rec.Header().Get("Vary") == "X-Client-Platform"; got != tt.wantVary {
				t.Fatalf("Vary X-Client-Platform = %v, want %v", got, tt.wantVary)
			}
		})
	}
}

func TestNoticePlatformsParse_2964(t *testing.T) {
	t.Parallel()
	n := NoticeFromEnv(envOf(map[string]string{EnvNotice: `{"id":"n1","message":{"ja":"x"},"platforms":["iOS","ios"," linux "]}`}))
	if n == nil || !reflect.DeepEqual(n.Platforms, []string{"ios", "linux"}) {
		t.Fatalf("got %+v", n)
	}
}
