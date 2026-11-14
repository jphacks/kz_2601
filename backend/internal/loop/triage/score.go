package triage

import "github.com/jphacks/kz_2601/backend/internal/loop"

// Agreement compares the stored categories with a human's labels (item id
// -> category) for #376's completion check (8 割以上). Items without a
// label, or not triaged by the model, are not counted.
func Agreement(items []loop.Item, labels map[string]string) (matched, total int) {
	for _, it := range items {
		want, ok := labels[it.ID]
		if !ok || it.Category == "" {
			continue
		}
		total++
		if it.Category == want {
			matched++
		}
	}
	return matched, total
}
