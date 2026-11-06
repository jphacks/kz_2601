package quota

import (
	"context"
	"log/slog"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

// UsageEvent is the message (and `event` attr) of the per-run usage log
// (#338, same shape as ai.usage). Cloud Logging filter: jsonPayload.event="runner.usage".
const UsageEvent = "runner.usage"

// Triggers of a run.
const (
	TriggerRecordCreated = "record_created"
	TriggerRecordUpdated = "record_updated"
	TriggerSchedule      = "schedule"
)

// PricingAsOf is the date of the unit costs below (#338 「語彙ごとの原価の目安」).
// They are estimates to correct from runner.usage; change them together.
const PricingAsOf = "2026-11-06"

// Unit costs in USD (#338).
const (
	costExecUSD      = 0.000016
	costRecipientUSD = 0.0000006
	costWriteUSD     = 0.000002
)

// Usage is one runner.usage line: labels and counts only. Never record
// values, notification text or raw uids (hashed with the ai.usage key).
type Usage struct {
	AppID      string
	CreatorUID string
	ActorUID   string
	RuleID     string
	Trigger    string
	Outcome    string
	// Limit is the scope that stopped the run (or dropped part of it).
	Limit             string
	Reads             int
	Writes            int
	WritesDropped     int
	NotifyRecipients  int
	RecipientsDropped int
	TasksOps          int
	CPUMs             int64
}

// EstimateCostUSD prices one run from the #338 unit costs. A run stopped
// before executing (skipped_*) costs nothing here.
func EstimateCostUSD(u Usage) float64 {
	switch u.Outcome {
	case OutcomeOK, OutcomeError:
	default:
		return 0
	}
	return costExecUSD + float64(u.NotifyRecipients)*costRecipientUSD + float64(u.Writes)*costWriteUSD
}

// LogUsage writes one runner.usage line.
func (q *Quota) LogUsage(ctx context.Context, u Usage) {
	attrs := []slog.Attr{
		slog.String("event", UsageEvent),
		slog.String("app_id", u.AppID),
		slog.String("rule_id", u.RuleID),
		slog.String("trigger", u.Trigger),
		slog.String("outcome", u.Outcome),
		slog.Int("reads", u.Reads),
		slog.Int("writes", u.Writes),
		slog.Int("notify_recipients", u.NotifyRecipients),
		slog.Int("tasks_ops", u.TasksOps),
		slog.Int64("cpu_ms", u.CPUMs),
	}
	if u.Limit != "" {
		attrs = append(attrs, slog.String("limit", u.Limit))
	}
	if u.WritesDropped > 0 {
		attrs = append(attrs, slog.Int("writes_dropped", u.WritesDropped))
	}
	if u.RecipientsDropped > 0 {
		attrs = append(attrs, slog.Int("recipients_dropped", u.RecipientsDropped))
	}
	if h := httpx.UIDHash(u.CreatorUID); h != "" {
		attrs = append(attrs, slog.String("creator_uid_hash", h))
	}
	if h := httpx.UIDHash(u.ActorUID); h != "" {
		attrs = append(attrs, slog.String("actor_uid_hash", h))
	}
	attrs = append(attrs, slog.Float64("est_cost_usd", EstimateCostUSD(u)), slog.String("pricing_as_of", PricingAsOf))
	q.logger().LogAttrs(ctx, slog.LevelInfo, UsageEvent, attrs...)
}
