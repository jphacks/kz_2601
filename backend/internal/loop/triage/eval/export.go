package eval

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/loop"
	"github.com/jphacks/kz_2601/backend/internal/loop/pii"
)

// Export is the outcome of Template.
type Export struct {
	// InWindow is the items ingested in the window that the model may see.
	InWindow int
	// Written is the lines written (InWindow, or MaxCases spread over it).
	Written int
	// Skipped is the items left out because their text still looked like
	// personal data after masking again (or masked to nothing).
	Skipped int
}

// templateLine is one line of the labelling template: a Case whose labels
// are empty. bundle is written even when empty so the person sees the key.
type templateLine struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	Category string `json:"category"`
	Priority string `json:"priority"`
	Bundle   string `json:"bundle"`
}

const templateHeader = `# triage-eval labelling template (#376). Fill category, priority and bundle on every line,
# then run: feedback-loop triage-eval -set THIS_FILE -model ai
# Keep this file out of the repository: the text is masked feedback of real users.
# category: bug / feature / question / praise / spam ("the app could not make it" is feature)
# priority: P1 / P2 / P3 as you would give the bundle, counting the bundle's reports
# bundle:   any short name; lines with the same name are the same problem, "" = a problem of its own
`

// Template writes the labelling template of the items ingested in
// [since, until]: one JSON line per item in the format Load reads, with the
// labels empty. Only items the triage model may see are written (AIAllowed,
// or held because the model's answer was rejected); the text is the stored
// masked text, masked again, and an item whose text still looks like
// personal data is skipped. The ids are the inbox ids (hashes), never the
// source's. More than MaxCases items are thinned evenly across the window.
func Template(w io.Writer, items []loop.Item, since, until time.Time) (Export, error) {
	var ex Export
	var picked []loop.Item
	for _, it := range items {
		if it.IngestedAt.Before(since) || it.IngestedAt.After(until) {
			continue
		}
		if !it.AIAllowed && it.HoldReason != loop.HoldRejected {
			continue
		}
		picked = append(picked, it)
	}
	sortByIngest(picked)
	ex.InWindow = len(picked)
	if n := len(picked); n > MaxCases {
		thin := make([]loop.Item, MaxCases)
		for i := range thin {
			thin[i] = picked[i*n/MaxCases]
		}
		picked = thin
	}
	if _, err := io.WriteString(w, templateHeader); err != nil {
		return ex, err
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	for _, it := range picked {
		text := pii.Mask(it.Text).Text
		if len(pii.Find(text)) > 0 || strings.TrimSpace(text) == "" {
			ex.Skipped++
			continue
		}
		if err := enc.Encode(templateLine{ID: it.ID, Text: text}); err != nil {
			return ex, fmt.Errorf("write template: %w", err)
		}
		ex.Written++
	}
	return ex, nil
}

func sortByIngest(items []loop.Item) {
	sort.Slice(items, func(i, j int) bool {
		if !items[i].IngestedAt.Equal(items[j].IngestedAt) {
			return items[i].IngestedAt.Before(items[j].IngestedAt)
		}
		return items[i].ID < items[j].ID
	})
}
