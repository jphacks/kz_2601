package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Settings (#267) expect keys, checked by checkSingle:
//   - single (bool): some entity has "single": true.
//   - setting (bool): some expression reads {"setting": ...}.
//
// Cases: internal/ai/eval/single_cases.json (-cases) and
// single_chat_cases.json (-chat -cases); m0disc_cases.json d_period.
func checkSingle(exp map[string]any, a *dsl.App) []string {
	var miss []string
	if want, ok := exp["single"].(bool); ok {
		if got := len(a.SingleEntities()) > 0; got != want {
			miss = append(miss, fmt.Sprintf("single=%v", got))
		}
	}
	if want, ok := exp["setting"].(bool); ok {
		raw, _ := json.Marshal(a)
		if got := strings.Contains(string(raw), `{"setting":`); got != want {
			miss = append(miss, fmt.Sprintf("setting=%v", got))
		}
	}
	return miss
}
