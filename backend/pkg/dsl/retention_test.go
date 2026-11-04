package dsl

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func kakeiboWithRetention(t *testing.T, days string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(docsDir, "examples", "kakeibo.json"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	// the first entity of the example is the expense log
	i := strings.Index(s, `"entities"`)
	j := strings.Index(s[i:], `"name"`)
	if i < 0 || j < 0 {
		t.Fatal("kakeibo example has no entity name")
	}
	return []byte(s[:i+j] + `"retentionDays": ` + days + `, ` + s[i+j:])
}

// #482: retentionDays is an optional per-entity period in days.
func TestRetentionDaysParses(t *testing.T) {
	app, err := Parse(kakeiboWithRetention(t, "30"))
	if err != nil {
		t.Fatal(err)
	}
	ids := app.RetentionEntities()
	if len(ids) != 1 || app.EntityByID(ids[0]).RetentionDays != 30 {
		t.Fatalf("retention entities = %v", ids)
	}
	plain, err := os.ReadFile(filepath.Join(docsDir, "examples", "kakeibo.json"))
	if err != nil {
		t.Fatal(err)
	}
	app, err = Parse(plain)
	if err != nil {
		t.Fatal(err)
	}
	if got := app.RetentionEntities(); len(got) != 0 {
		t.Fatalf("no retention expected: %v", got)
	}
}

func TestRetentionDaysOutOfRange(t *testing.T) {
	for _, days := range []string{"0", "-7", "3651", "1.5"} {
		if _, err := Parse(kakeiboWithRetention(t, days)); err == nil {
			t.Errorf("retentionDays %s must fail", days)
		}
	}
	// the semantic check alone (a DSL that skipped the schema)
	var problems []string
	add := func(f string, a ...any) { problems = append(problems, f) }
	checkEntityRetention(&Entity{ID: "logs", RetentionDays: -1}, add)
	checkEntityRetention(&Entity{ID: "logs", RetentionDays: MaxRetentionDays + 1}, add)
	checkEntityRetention(&Entity{ID: "logs", RetentionDays: MaxRetentionDays}, add)
	checkEntityRetention(&Entity{ID: "logs"}, add)
	if len(problems) != 2 || !slices.ContainsFunc(problems, func(p string) bool { return strings.Contains(p, "retentionDays") }) {
		t.Fatalf("problems = %v", problems)
	}
}
