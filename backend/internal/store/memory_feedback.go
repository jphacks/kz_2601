package store

import (
	"context"
	"sort"
	"time"
)

func cloneFeedback(fb *Feedback, withBodies bool) *Feedback {
	cp := *fb
	cp.DSL, cp.Screenshot = nil, nil
	if withBodies {
		cp.DSL = cloneRaw(fb.DSL)
		cp.Screenshot = append([]byte(nil), fb.Screenshot...)
	}
	if fb.Device != nil {
		d := *fb.Device
		cp.Device = &d
	}
	return &cp
}

// CreateFeedback implements Store.
func (m *Memory) CreateFeedback(_ context.Context, fb Feedback, perDay int) (*Feedback, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.feedback == nil {
		m.feedback = map[string]*Feedback{}
	}
	now := m.now().UTC()
	day := FeedbackDay(now)
	if perDay > 0 {
		n := 0
		for _, old := range m.feedback {
			if old.UID == fb.UID && old.Day == day {
				n++
			}
		}
		if n >= perDay {
			return nil, ErrFeedbackQuota
		}
	}
	fb.ID, fb.Status, fb.Day, fb.CreatedAt = NewID("fb_"), FeedbackReceived, day, now
	m.feedback[fb.ID] = cloneFeedback(&fb, true)
	return cloneFeedback(&fb, false), nil
}

// ListFeedback implements FeedbackReader.
func (m *Memory) ListFeedback(_ context.Context, q FeedbackQuery) ([]Feedback, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Feedback{}
	for _, fb := range m.feedback {
		if (q.Since.IsZero() || !fb.CreatedAt.Before(q.Since)) && (q.Status == "" || fb.Status == q.Status) {
			cp := cloneFeedback(fb, false)
			cp.Listed = fb.Attachments()
			out = append(out, *cp)
		}
	}
	sortFeedback(out)
	return page(out, feedbackLimit(q.Limit), 0), nil
}

// GetFeedback implements FeedbackReader.
func (m *Memory) GetFeedback(_ context.Context, id string) (*Feedback, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	fb, ok := m.feedback[id]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneFeedback(fb, true), nil
}

// PurgeFeedbackBefore implements FeedbackPurger.
func (m *Memory) PurgeFeedbackBefore(_ context.Context, cutoff time.Time, limit int, dryRun bool) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for id, fb := range m.feedback {
		if limit > 0 && n >= limit {
			break
		}
		if fb.CreatedAt.Before(cutoff) {
			n++
			if !dryRun {
				delete(m.feedback, id)
			}
		}
	}
	return n, nil
}

// SetFeedbackCreatedAt backdates a document (tests only).
func (m *Memory) SetFeedbackCreatedAt(id string, t time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	fb, ok := m.feedback[id]
	if !ok {
		return ErrNotFound
	}
	fb.CreatedAt, fb.Day = t, FeedbackDay(t)
	return nil
}

// dropFeedback deletes uid's feedback. Callers hold m.mu.
func (m *Memory) dropFeedback(uid string) int {
	n := 0
	for id, fb := range m.feedback {
		if fb.UID == uid {
			delete(m.feedback, id)
			n++
		}
	}
	return n
}

func feedbackLimit(n int) int {
	if n <= 0 {
		return 50
	}
	return min(n, 500)
}

func sortFeedback(fs []Feedback) {
	sort.SliceStable(fs, func(i, j int) bool {
		if !fs[i].CreatedAt.Equal(fs[j].CreatedAt) {
			return fs[i].CreatedAt.After(fs[j].CreatedAt)
		}
		return fs[i].ID > fs[j].ID
	})
}
