package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// addMinutes (bool): some expression uses op addMinutes (departure =
// arrival − travel minutes). Cases: internal/ai/eval/add_minutes_cases.json.
func checkAddMinutes(exp map[string]any, a *dsl.App) []string {
	want, ok := exp["addMinutes"].(bool)
	if !ok {
		return nil
	}
	raw, _ := json.Marshal(a)
	if got := strings.Contains(string(raw), `"op":"addMinutes"`); got != want {
		return []string{fmt.Sprintf("addMinutes=%v", got)}
	}
	return nil
}
