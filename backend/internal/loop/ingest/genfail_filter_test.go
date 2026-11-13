package ingest

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestGenFailFilter(t *testing.T) {
	since := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
	until := since.Add(24 * time.Hour)
	jst := time.FixedZone("JST", 9*60*60)
	for _, tc := range []struct {
		name string
		f    GenFailFilter
		want string
	}{
		{"all lines of the range", GenFailFilter{Since: since, Until: until},
			`resource.type="cloud_run_revision" AND jsonPayload.event="ai.generate" AND timestamp>="2026-11-12T00:00:00Z" AND timestamp<="2026-11-13T00:00:00Z"`},
		{"another zone is written in UTC", GenFailFilter{Since: since.In(jst), Until: until.In(jst)},
			`resource.type="cloud_run_revision" AND jsonPayload.event="ai.generate" AND timestamp>="2026-11-12T00:00:00Z" AND timestamp<="2026-11-13T00:00:00Z"`},
		{"sub-second ends are kept", GenFailFilter{Since: since.Add(1500 * time.Millisecond), Until: until.Add(time.Nanosecond)},
			`resource.type="cloud_run_revision" AND jsonPayload.event="ai.generate" AND timestamp>="2026-11-12T00:00:01.5Z" AND timestamp<="2026-11-13T00:00:00.000000001Z"`},
		{"same instant (an empty range is still a range)", GenFailFilter{Since: since, Until: since},
			`resource.type="cloud_run_revision" AND jsonPayload.event="ai.generate" AND timestamp>="2026-11-12T00:00:00Z" AND timestamp<="2026-11-12T00:00:00Z"`},
		{"services", GenFailFilter{Since: since, Until: until, Services: []string{"ai-prod", " ai-stg "}},
			`resource.type="cloud_run_revision" AND resource.labels.service_name=("ai-prod" OR "ai-stg") AND jsonPayload.event="ai.generate" AND timestamp>="2026-11-12T00:00:00Z" AND timestamp<="2026-11-13T00:00:00Z"`},
		{"failures only", GenFailFilter{Since: since, Until: until, FailuresOnly: true},
			`resource.type="cloud_run_revision" AND jsonPayload.event="ai.generate" AND (jsonPayload.outcome="failed" OR jsonPayload.outcome="refused" OR jsonPayload.error_code:* OR jsonPayload.retry_reasons:* OR jsonPayload.validation_codes:*) AND timestamp>="2026-11-12T00:00:00Z" AND timestamp<="2026-11-13T00:00:00Z"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.f.Build()
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("got\n%s\nwant\n%s", got, tc.want)
			}
		})
	}
}

func TestGenFailFilterRefuses(t *testing.T) {
	since := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
	for name, f := range map[string]GenFailFilter{
		"no since":       {Until: since},
		"no until":       {Since: since},
		"until first":    {Since: since, Until: since.Add(-time.Nanosecond)},
		"empty service":  {Since: since, Until: since.Add(time.Hour), Services: []string{" "}},
		"nothing at all": {},
	} {
		if got, err := f.Build(); err == nil {
			t.Errorf("%s: %s", name, got)
		}
	}
}

// A quote in a service name stays inside its string.
func TestGenFailFilterQuotes(t *testing.T) {
	since := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
	got, err := GenFailFilter{Since: since, Until: since, Services: []string{`a" OR timestamp>"0`}}.Build()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `service_name=("a\" OR timestamp>\"0")`) {
		t.Fatal(got)
	}
}

// The fields asked for are exactly the ones the file reader decodes.
func TestGenFailFieldsMatchTheDecoder(t *testing.T) {
	var tags []string
	for f := range reflect.TypeFor[genPayload]().Fields() {
		tags = append(tags, strings.Split(f.Tag.Get("json"), ",")[0])
	}
	if !slices.Equal(tags, GenFailFields) {
		t.Fatalf("decoder %v, filter %v", tags, GenFailFields)
	}
	for _, f := range GenFailFields {
		if strings.Contains(f, "uid") || strings.Contains(f, "app") {
			t.Fatalf("%s must not be read", f)
		}
	}
}
