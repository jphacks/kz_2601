package store

import (
	"context"
	"time"

	"cloud.google.com/go/firestore"
)

var (
	_ RuleStatuses   = (*Firestore)(nil)
	_ RunnerActivity = (*Firestore)(nil)
)

type ruleStatusDoc struct {
	AppID        string    `firestore:"appId"`
	RuleID       string    `firestore:"ruleId"`
	At           time.Time `firestore:"at"`
	Outcome      string    `firestore:"outcome"`
	Reason       string    `firestore:"reason,omitempty"`
	NotifyPaused bool      `firestore:"notifyPaused,omitempty"`
	LastOKAt     time.Time `firestore:"lastOkAt,omitempty"`
	QuotaDay     string    `firestore:"quotaDay,omitempty"`
	QuotaUsed    int       `firestore:"quotaUsed,omitempty"`
	QuotaLimit   int       `firestore:"quotaLimit,omitempty"`
	ExpireAt     time.Time `firestore:"expireAt"`
}

func (f *Firestore) ruleStatusCol(env string) *firestore.CollectionRef {
	return f.c.Collection(runnerCollection).Doc(env).Collection("rule_status")
}

// PutRuleStatus implements RuleStatuses: one write. The fields s leaves
// empty are left out of the merge, so the stored ones stay.
func (f *Firestore) PutRuleStatus(ctx context.Context, env string, s RuleStatus) error {
	data := map[string]any{
		"appId":        s.AppID,
		"ruleId":       s.RuleID,
		"at":           s.At.UTC(),
		"outcome":      s.Outcome,
		"reason":       s.Reason,
		"notifyPaused": s.NotifyPaused,
		"expireAt":     s.ExpireAt.UTC(),
	}
	if !s.LastOKAt.IsZero() {
		data["lastOkAt"] = s.LastOKAt.UTC()
	}
	if s.QuotaDay != "" {
		data["quotaDay"], data["quotaUsed"], data["quotaLimit"] = s.QuotaDay, int64(s.QuotaUsed), int64(s.QuotaLimit)
	}
	_, err := f.ruleStatusCol(env).Doc(s.ID).Set(ctx, data, firestore.MergeAll)
	return err
}

// GetRuleStatuses implements RunnerActivity (one batched read). A document
// past its expireAt that the TTL has not removed yet does not count.
func (f *Firestore) GetRuleStatuses(ctx context.Context, env string, ids []string) (map[string]RuleStatus, error) {
	out := map[string]RuleStatus{}
	if len(ids) == 0 {
		return out, nil
	}
	refs := make([]*firestore.DocumentRef, len(ids))
	for i, id := range ids {
		refs[i] = f.ruleStatusCol(env).Doc(id)
	}
	snaps, err := f.c.GetAll(ctx, refs)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	for _, snap := range snaps {
		if !snap.Exists() {
			continue
		}
		var d ruleStatusDoc
		if err := snap.DataTo(&d); err != nil {
			return nil, err
		}
		if !now.Before(d.ExpireAt) {
			continue
		}
		out[snap.Ref.ID] = RuleStatus{ID: snap.Ref.ID, AppID: d.AppID, RuleID: d.RuleID, At: d.At, Outcome: d.Outcome,
			Reason: d.Reason, NotifyPaused: d.NotifyPaused, LastOKAt: d.LastOKAt, QuotaDay: d.QuotaDay,
			QuotaUsed: d.QuotaUsed, QuotaLimit: d.QuotaLimit, ExpireAt: d.ExpireAt}
	}
	return out, nil
}

// ActiveStops implements RunnerActivity.
func (f *Firestore) ActiveStops(ctx context.Context, env string, ids []string, now time.Time) (map[string]StopState, error) {
	got, err := f.GetStops(ctx, env, ids)
	if err != nil {
		return nil, err
	}
	return activeStops(got, now), nil
}
