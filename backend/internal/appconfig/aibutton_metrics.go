package appconfig

// aiButton metrics (#446): the iOS app counts on-device AI availability and
// failed aiButton runs per local day and sends them once a day. This handler
// only validates the fixed labels and writes structured log lines (Cloud
// Logging → BigQuery). No Firestore writes, no uid, no user text.
// docs/analytics/ai-button-metrics.md

import (
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

// Log events (the message and the `event` attr). One availability line per
// device-day, one task line per (day, task), one failure line per (day, task, reason).
const (
	AIButtonAvailabilityEvent = "ai.button.availability"
	AIButtonTaskEvent         = "ai.button.task"
	AIButtonFailureEvent      = "ai.button.failure"
)

// Payload limits.
const (
	maxAIButtonDays     = 7
	maxAIButtonTasks    = 8
	maxAIButtonCount    = 10000
	aiButtonDaySkewDays = 9 // accept days from today-9 to today+1 (time zones, late flushes)
)

var (
	aiButtonAvailabilities = set("available", "device_not_eligible", "apple_intelligence_off", "model_not_ready", "os_too_old", "other")
	aiButtonTasks          = set("summarize", "classify", "rewrite", "extract", "suggest", "generate", "other")
	aiButtonReasons        = set("unavailable", "invalid_response", "context_too_long", "guardrail", "unsupported_language",
		"refusal", "rate_limited", "assets_unavailable", "model_error", "bad_config")
	reportIDRe = regexp.MustCompile(`^[A-Za-z0-9-]{8,64}$`)
)

func set(vs ...string) map[string]bool {
	m := make(map[string]bool, len(vs))
	for _, v := range vs {
		m[v] = true
	}
	return m
}

type aiButtonTask struct {
	Task     string         `json:"task"`
	Attempts int            `json:"attempts"`
	Failures map[string]int `json:"failures"`
}

type aiButtonDay struct {
	ReportID     string         `json:"reportId"`
	Day          string         `json:"day"`
	Availability string         `json:"availability"`
	Tasks        []aiButtonTask `json:"tasks"`
}

type aiButtonReport struct {
	Days []aiButtonDay `json:"days"`
}

// postAIButtonMetrics: POST /v1/metrics/ai-button. Authenticated only to keep
// junk out; the uid is never logged.
func (h *Handler) postAIButtonMetrics(w http.ResponseWriter, r *http.Request) {
	var body aiButtonReport
	if !httpx.DecodeBody(w, r, &body) {
		return
	}
	if err := validateAIButtonReport(body, time.Now()); err != nil {
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, err.Error()))
		return
	}
	build := r.Header.Get(httpx.ClientBuildHeader)
	if len(build) > 16 {
		build = build[:16]
	}
	for _, d := range body.Days {
		common := []any{slog.String("report_id", d.ReportID), slog.String("day", d.Day),
			slog.String("availability", d.Availability), slog.String("client_build", build)}
		slog.Info(AIButtonAvailabilityEvent, append([]any{slog.String("event", AIButtonAvailabilityEvent)}, common...)...)
		for _, t := range d.Tasks {
			total := 0
			for reason, n := range t.Failures {
				if n == 0 {
					continue
				}
				total += n
				slog.Info(AIButtonFailureEvent, append([]any{slog.String("event", AIButtonFailureEvent)}, append(common,
					slog.String("task", t.Task), slog.String("reason", reason), slog.Int("count", n))...)...)
			}
			slog.Info(AIButtonTaskEvent, append([]any{slog.String("event", AIButtonTaskEvent)}, append(common,
				slog.String("task", t.Task), slog.Int("attempts", t.Attempts), slog.Int("failures", total))...)...)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func validateAIButtonReport(b aiButtonReport, now time.Time) error {
	if len(b.Days) == 0 || len(b.Days) > maxAIButtonDays {
		return fmt.Errorf("days must have 1 to %d entries", maxAIButtonDays)
	}
	today := now.UTC().Truncate(24 * time.Hour)
	for i, d := range b.Days {
		if !reportIDRe.MatchString(d.ReportID) {
			return fmt.Errorf("days[%d].reportId is invalid", i)
		}
		day, err := time.Parse(time.DateOnly, d.Day)
		if err != nil || day.Before(today.AddDate(0, 0, -aiButtonDaySkewDays)) || day.After(today.AddDate(0, 0, 1)) {
			return fmt.Errorf("days[%d].day must be a recent YYYY-MM-DD", i)
		}
		if !aiButtonAvailabilities[d.Availability] {
			return fmt.Errorf("days[%d].availability is unknown", i)
		}
		if len(d.Tasks) > maxAIButtonTasks {
			return fmt.Errorf("days[%d].tasks must have at most %d entries", i, maxAIButtonTasks)
		}
		seen := map[string]bool{}
		for j, t := range d.Tasks {
			if !aiButtonTasks[t.Task] || seen[t.Task] {
				return fmt.Errorf("days[%d].tasks[%d].task is unknown or repeated", i, j)
			}
			seen[t.Task] = true
			if t.Attempts < 0 || t.Attempts > maxAIButtonCount {
				return fmt.Errorf("days[%d].tasks[%d].attempts is out of range", i, j)
			}
			for reason, n := range t.Failures {
				if !aiButtonReasons[reason] {
					return fmt.Errorf("days[%d].tasks[%d].failures has an unknown reason", i, j)
				}
				if n < 0 || n > maxAIButtonCount {
					return fmt.Errorf("days[%d].tasks[%d].failures is out of range", i, j)
				}
			}
		}
	}
	return nil
}
