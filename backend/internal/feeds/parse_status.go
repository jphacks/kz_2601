package feeds

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"
)

// ---- status (Atlassian Statuspage /api/v2/summary.json) ---------------------

type statusSummary struct {
	Page struct {
		UpdatedAt string `json:"updated_at"`
	} `json:"page"`
	Status struct {
		Indicator   string `json:"indicator"`
		Description string `json:"description"`
	} `json:"status"`
	Components []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
		Group  bool   `json:"group"`
	} `json:"components"`
	Incidents             []statusEvent `json:"incidents"`
	ScheduledMaintenances []statusEvent `json:"scheduled_maintenances"`
}

// statusEvent is an incident or a scheduled maintenance. The update bodies
// (incident_updates) are not read: only the name, state and times are shown.
type statusEvent struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	Impact       string `json:"impact"`
	CreatedAt    string `json:"created_at"`
	StartedAt    string `json:"started_at"`
	ScheduledFor string `json:"scheduled_for"`
}

// statusID is the shape of a Statuspage incident id; the link is built from
// the allowlisted site and this id, never taken from the response.
var statusID = regexp.MustCompile(`^[a-z0-9]{1,32}$`)

var (
	statusIndicator = map[string]string{"none": "すべて正常", "minor": "一部で低下", "major": "大きな障害", "critical": "重大な障害", "maintenance": "メンテナンス中"}
	statusImpact    = map[string]string{"none": "なし", "minor": "小", "major": "大", "critical": "重大", "maintenance": "メンテナンス"}
	statusIncident  = map[string]string{"investigating": "調査中", "identified": "原因を特定", "monitoring": "経過を観察中"}
	statusMaint     = map[string]string{"scheduled": "予定", "in_progress": "実施中", "verifying": "確認中"}
	statusComponent = map[string]string{"degraded_performance": "性能の低下", "partial_outage": "一部停止", "major_outage": "停止", "under_maintenance": "メンテナンス中"}
	// statusIcon uses names in docs/dsl/icon-map.json (Android draws those).
	statusIcon = map[string]string{
		"none": "checkmark.circle", "minor": "exclamationmark.circle", "major": "exclamationmark.triangle", "critical": "xmark.circle",
		"maintenance":          "wrench",
		"degraded_performance": "exclamationmark.circle", "partial_outage": "exclamationmark.triangle", "major_outage": "xmark.circle",
		"under_maintenance": "wrench",
	}
)

// parseStatus turns a summary into rows: the overall state first, then open
// incidents, maintenance in progress, components that are not operational,
// and upcoming maintenance. name and site come from the allowlist.
func parseStatus(body []byte, name, site string) ([]Item, error) {
	var s statusSummary
	if err := json.Unmarshal(body, &s); err != nil {
		return nil, fmt.Errorf("status: %w", err)
	}
	if s.Status.Indicator == "" {
		return nil, errors.New("status: no status.indicator")
	}
	out := []Item{{
		Title:    name + ": " + statusLabel(statusIndicator, s.Status.Indicator),
		Subtitle: clean(s.Status.Description, maxSubtitleRunes),
		Icon:     statusIcon[s.Status.Indicator],
		Date:     statusTime(s.Page.UpdatedAt),
		URL:      httpsURL(site),
	}}
	for _, e := range s.Incidents {
		st, ok := statusIncident[e.Status]
		if !ok { // resolved / postmortem
			continue
		}
		out = appendEvent(out, e, site, "障害・"+st+"・影響 "+statusLabel(statusImpact, e.Impact), statusIcon[e.Impact], e.StartedAt, e.CreatedAt)
	}
	var upcoming []Item
	for _, e := range s.ScheduledMaintenances {
		st, ok := statusMaint[e.Status]
		if !ok { // completed / cancelled
			continue
		}
		if e.Status == "scheduled" {
			upcoming = appendEvent(upcoming, e, site, "メンテナンス・"+st, statusIcon["maintenance"], e.ScheduledFor, e.CreatedAt)
			continue
		}
		out = appendEvent(out, e, site, "メンテナンス・"+st, statusIcon["maintenance"], e.ScheduledFor, e.StartedAt)
	}
	for _, c := range s.Components {
		st, ok := statusComponent[c.Status]
		if !ok || c.Group { // operational, or a group that repeats its members
			continue
		}
		if t := clean(c.Name, maxTitleRunes); t != "" {
			out = append(out, Item{Title: t, Subtitle: st, Icon: statusIcon[c.Status]})
		}
	}
	return append(out, upcoming...), nil
}

func appendEvent(out []Item, e statusEvent, site, subtitle, icon string, times ...string) []Item {
	title := clean(e.Name, maxTitleRunes)
	if title == "" {
		return out
	}
	it := Item{Title: title, Subtitle: clean(subtitle, maxSubtitleRunes), Icon: icon}
	for _, t := range times {
		if it.Date = statusTime(t); it.Date != "" {
			break
		}
	}
	if statusID.MatchString(e.ID) {
		it.URL = httpsURL(site + "/incidents/" + e.ID)
	}
	return append(out, it)
}

// statusLabel is the Japanese name of a known value, or the value itself, cleaned.
func statusLabel(m map[string]string, v string) string {
	if s, ok := m[v]; ok {
		return s
	}
	return clean(v, 40)
}

// statusTime is an RFC 3339 time in JST, "" when it does not parse.
func statusTime(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return ""
	}
	return t.In(jst).Format(time.RFC3339)
}
