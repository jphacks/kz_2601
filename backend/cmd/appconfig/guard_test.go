package main

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/fsenv"
)

// #290: a stg process must not start on the production Firestore unless the
// shared database is accepted; the emulator is never guarded.
func TestCheckTarget(t *testing.T) {
	prod := fsenv.ProdProject
	cases := []struct {
		name string
		env  map[string]string
		ok   bool
		warn bool
	}{
		{"no project (memory store)", map[string]string{"APP_ENV": "stg"}, true, false},
		{"prod on prod", map[string]string{"GOOGLE_CLOUD_PROJECT": prod, "APP_ENV": "prod"}, true, false},
		{"no APP_ENV on prod db", map[string]string{"GOOGLE_CLOUD_PROJECT": prod}, true, false},
		{"stg on prod db refused", map[string]string{"GOOGLE_CLOUD_PROJECT": prod, "APP_ENV": "stg"}, false, false},
		{"staging on prod db refused", map[string]string{"GOOGLE_CLOUD_PROJECT": prod, "APP_ENV": "staging"}, false, false},
		{"override other than 1 refused", map[string]string{"GOOGLE_CLOUD_PROJECT": prod, "APP_ENV": "stg", fsenv.OverrideEnv: "true"}, false, false},
		{"stg on prod db accepted with a warning", map[string]string{"GOOGLE_CLOUD_PROJECT": prod, "APP_ENV": "stg", fsenv.OverrideEnv: "1"}, true, true},
		{"stg on its own database", map[string]string{"GOOGLE_CLOUD_PROJECT": prod, "APP_ENV": "stg", "FIRESTORE_DATABASE": "stg"}, true, false},
		{"stg on its own project", map[string]string{"GOOGLE_CLOUD_PROJECT": "tailorit-stg", "APP_ENV": "stg"}, true, false},
		{"configured prod identity refused", map[string]string{"GOOGLE_CLOUD_PROJECT": "other-prod", "PROD_FIRESTORE_PROJECT": "other-prod", "APP_ENV": "stg"}, false, false},
		{"emulator is not guarded", map[string]string{"GOOGLE_CLOUD_PROJECT": prod, "APP_ENV": "stg", "FIRESTORE_EMULATOR_HOST": "127.0.0.1:8089"}, true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&buf, nil))
			getenv := func(k string) string { return c.env[k] }
			err := checkTarget(fsenv.FromEnv(getenv), getenv, logger)
			if (err == nil) != c.ok {
				t.Fatalf("err = %v, want ok=%v", err, c.ok)
			}
			if warned := strings.Contains(buf.String(), "level=WARN"); warned != c.warn {
				t.Fatalf("warned = %v, want %v: %s", warned, c.warn, buf.String())
			}
		})
	}
}
