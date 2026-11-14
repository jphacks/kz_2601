package appconfig

import (
	"reflect"
	"testing"
)

func TestNoticeBuilds_2965(t *testing.T) {
	t.Parallel()
	const base = `"id":"n1","message":{"ja":"x"}`
	tests := []struct {
		name string
		raw  string
		want *Notice // nil = rejected
	}{
		{"old form only is unchanged", `{` + base + `,"minIOSBuild":3,"maxIOSBuild":9}`,
			&Notice{ID: "n1", Severity: NoticeInfo, Message: NoticeText{Ja: "x"}, MinIOSBuild: 3, MaxIOSBuild: 9}},
		{"new form only", `{` + base + `,"builds":{"android":{"min":7,"max":20},"windows":{"max":5}}}`,
			&Notice{ID: "n1", Severity: NoticeInfo, Message: NoticeText{Ja: "x"},
				Builds: map[string]BuildRange{"android": {Min: 7, Max: 20}, "windows": {Max: 5}}}},
		{"new ios mirrors into the old fields", `{` + base + `,"builds":{"ios":{"min":4}}}`,
			&Notice{ID: "n1", Severity: NoticeInfo, Message: NoticeText{Ja: "x"}, MinIOSBuild: 4,
				Builds: map[string]BuildRange{"ios": {Min: 4}}}},
		{"both: builds.ios wins", `{` + base + `,"minIOSBuild":3,"maxIOSBuild":9,"builds":{"ios":{"min":5},"android":{"min":1}}}`,
			&Notice{ID: "n1", Severity: NoticeInfo, Message: NoticeText{Ja: "x"}, MinIOSBuild: 5,
				Builds: map[string]BuildRange{"ios": {Min: 5}, "android": {Min: 1}}}},
		{"both: no ios in builds keeps the old", `{` + base + `,"minIOSBuild":3,"builds":{"android":{"min":1}}}`,
			&Notice{ID: "n1", Severity: NoticeInfo, Message: NoticeText{Ja: "x"}, MinIOSBuild: 3,
				Builds: map[string]BuildRange{"android": {Min: 1}}}},
		{"key is normalised", `{` + base + `,"builds":{" Android ":{"min":2}}}`,
			&Notice{ID: "n1", Severity: NoticeInfo, Message: NoticeText{Ja: "x"}, Builds: map[string]BuildRange{"android": {Min: 2}}}},
		{"unknown OS rejected", `{` + base + `,"builds":{"plan9":{"min":1}}}`, nil},
		{"duplicate key after normalising rejected", `{` + base + `,"builds":{"ios":{"min":1},"IOS":{"min":2}}}`, nil},
		{"negative rejected", `{` + base + `,"builds":{"ios":{"min":-1}}}`, nil},
		{"max below min rejected", `{` + base + `,"builds":{"android":{"min":9,"max":3}}}`, nil},
		{"unknown field in range rejected", `{` + base + `,"builds":{"ios":{"low":1}}}`, nil},
		{"empty builds is dropped", `{` + base + `,"builds":{}}`,
			&Notice{ID: "n1", Severity: NoticeInfo, Message: NoticeText{Ja: "x"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := NoticeFromEnv(envOf(map[string]string{EnvNotice: tt.raw}))
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}
