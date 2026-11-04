package fsenv

import (
	"strings"
	"testing"
)

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestFromEnv(t *testing.T) {
	got := FromEnv(envOf(map[string]string{"GOOGLE_CLOUD_PROJECT": "p", "APP_ENV": "stg"}))
	if got.Project != "p" || got.Database != "(default)" || got.AppEnv != "stg" {
		t.Fatalf("defaults: %+v", got)
	}
	got = FromEnv(envOf(map[string]string{"GOOGLE_CLOUD_PROJECT": "p", "FIRESTORE_DATABASE": "stg"}))
	if got.Database != "stg" || got.String() != "p/stg" {
		t.Fatalf("database: %+v", got)
	}
}

func TestCheck(t *testing.T) {
	cases := []struct {
		name   string
		env    map[string]string
		ok     bool
		shared bool
	}{
		{"stg on prod db refused", map[string]string{"GOOGLE_CLOUD_PROJECT": ProdProject, "APP_ENV": "stg"}, false, false},
		{"staging alias refused", map[string]string{"GOOGLE_CLOUD_PROJECT": ProdProject, "APP_ENV": "staging"}, false, false},
		{"stg with override", map[string]string{"GOOGLE_CLOUD_PROJECT": ProdProject, "APP_ENV": "stg", OverrideEnv: "1"}, true, true},
		{"override must be 1", map[string]string{"GOOGLE_CLOUD_PROJECT": ProdProject, "APP_ENV": "stg", OverrideEnv: "true"}, false, false},
		{"stg on named db", map[string]string{"GOOGLE_CLOUD_PROJECT": ProdProject, "APP_ENV": "stg", "FIRESTORE_DATABASE": "stg"}, true, false},
		{"stg on named db with stale override", map[string]string{"GOOGLE_CLOUD_PROJECT": ProdProject, "APP_ENV": "stg", "FIRESTORE_DATABASE": "stg", OverrideEnv: "1"}, true, false},
		{"stg in its own project", map[string]string{"GOOGLE_CLOUD_PROJECT": "tailorit-stg", "APP_ENV": "stg"}, true, false},
		{"prod on prod db", map[string]string{"GOOGLE_CLOUD_PROJECT": ProdProject, "APP_ENV": "prod"}, true, false},
		{"no APP_ENV (purge job)", map[string]string{"GOOGLE_CLOUD_PROJECT": ProdProject}, true, false},
		{"no project", map[string]string{"APP_ENV": "stg"}, true, false},
		{"emulator", map[string]string{"GOOGLE_CLOUD_PROJECT": ProdProject, "APP_ENV": "stg", "FIRESTORE_EMULATOR_HOST": "localhost:8080"}, true, false},
		{"configured prod project", map[string]string{"GOOGLE_CLOUD_PROJECT": "other", "APP_ENV": "stg", "PROD_FIRESTORE_PROJECT": "other"}, false, false},
		{"configured prod database elsewhere", map[string]string{"GOOGLE_CLOUD_PROJECT": ProdProject, "APP_ENV": "stg", "PROD_FIRESTORE_DATABASE": "prod"}, true, false},
		{"configured prod database hit", map[string]string{"GOOGLE_CLOUD_PROJECT": ProdProject, "APP_ENV": "stg", "FIRESTORE_DATABASE": "prod", "PROD_FIRESTORE_DATABASE": "prod"}, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			getenv := envOf(c.env)
			shared, err := Check(FromEnv(getenv), getenv)
			if (err == nil) != c.ok || shared != c.shared {
				t.Fatalf("want ok=%v shared=%v, got shared=%v err=%v", c.ok, c.shared, shared, err)
			}
			if err != nil && !strings.Contains(err.Error(), OverrideEnv) {
				t.Fatalf("error should name the override: %v", err)
			}
		})
	}
}
