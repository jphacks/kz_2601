package entitlement

import (
	"context"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/googleplay"
)

// VoidedPurchases: the fake has no voided purchases (googleplay.API).
func (f *fakePlay) VoidedPurchases(_ context.Context, _ string, _ time.Time, _ string) (*googleplay.VoidedPage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	return &googleplay.VoidedPage{}, nil
}

var _ googleplay.API = (*fakePlay)(nil)
