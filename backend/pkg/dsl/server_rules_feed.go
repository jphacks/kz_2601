package dsl

import (
	"fmt"
	"slices"
	"strings"
)

// saveFeed of a server rule: a time-triggered rule saves one snapshot of a
// key-less public feed (the feed leaf's weather / pressure / warnings) as a
// new record, so an app keeps a history of what was otherwise only a
// forecast (e.g. 体調と気圧の関係). Closed and bounded:
//
//   - providers weather / pressure / warnings, a fixed region code of
//     WeatherRegions (never a value of a record: nothing about the user or
//     the app's data is sent to the source);
//   - only on a time trigger without each (one fetch per run), at most
//     MaxSaveFeedActions per app;
//   - the record goes to an entity on the rule's side like createRecord
//     (private: the owner's own, shared / public: the app's), never single
//     or addressed; fields maps the snapshot's parts to stored fields.
//
// The runner fetches through internal/feeds (same allowlist, caps, cache
// and kill switch as the feed leaf).

// RuleSaveFeed saves one feed snapshot into a new record of Entity.
type RuleSaveFeed struct {
	Provider string         `json:"provider"` // weather|pressure|warnings
	Region   string         `json:"region"`   // WeatherRegions code
	Entity   string         `json:"entity"`
	Fields   SaveFeedFields `json:"fields"`
}

// SaveFeedFields names the target fields of the snapshot parts ("" = not saved).
type SaveFeedFields struct {
	Date    string `json:"date,omitempty"`    // date / datetime: the run's day
	Summary string `json:"summary,omitempty"` // string / text
	Min     string `json:"min,omitempty"`     // number / integer (°C, hPa)
	Max     string `json:"max,omitempty"`     // number / integer (°C, hPa)
	Drop    string `json:"drop,omitempty"`    // boolean (pressure)
}

// MaxSaveFeedActions caps the saveFeed actions of an app.
const MaxSaveFeedActions = 3

// SaveFeedProviders are the providers saveFeed reads. Validate checks them
// (not a schema enum: the server reads the feed, so saveFeed does not follow
// the feed component's per-client gates on provider values).
var SaveFeedProviders = []string{FeedWeather, FeedPressure, FeedWarnings}

// saveFeedParts lists what each provider fills.
var saveFeedParts = map[string][]string{
	FeedWeather:  {"date", "summary", "min", "max"},
	FeedPressure: {"date", "summary", "min", "max", "drop"},
	FeedWarnings: {"date", "summary"},
}

func (f SaveFeedFields) parts() [][2]string {
	return [][2]string{{"date", f.Date}, {"summary", f.Summary}, {"min", f.Min}, {"max", f.Max}, {"drop", f.Drop}}
}

var saveFeedTypes = map[string][]FieldType{
	"date":    {FieldDate, FieldDatetime},
	"summary": {FieldString, FieldText},
	"min":     {FieldNumber, FieldInteger},
	"max":     {FieldNumber, FieldInteger},
	"drop":    {FieldBoolean},
}

// checkRuleSaveFeed validates one saveFeed action of rule rc.r.
func checkRuleSaveFeed(where string, rc *ruleCheck, s *RuleSaveFeed) {
	add := rc.add
	if !rc.r.IsScheduled() || rc.r.On.Each != "" {
		add("%s: saveFeed needs a time trigger without each (every + time; one snapshot per run)", where)
	}
	parts, ok := saveFeedParts[s.Provider]
	if !ok {
		add("%s: provider %q must be %s", where, s.Provider, strings.Join(SaveFeedProviders, ", "))
	}
	if _, ok := WeatherRegions[s.Region]; !ok {
		add("%s: region %q is not a forecast office code (e.g. 130000 東京)", where, s.Region)
	}
	target := rc.entities[s.Entity]
	if target == nil {
		add("%s: entity %q does not exist (entities: %s)", where, s.Entity, entityIDs(rc.entities))
		return
	}
	if target.Single {
		add("%s: entity %q is single (always one record); a rule cannot add records to it", where, target.ID)
	}
	checkRuleTargetScope(where, target, rc.scope, add)
	used := map[string]bool{}
	for _, p := range s.Fields.parts() {
		part, id := p[0], p[1]
		if id == "" {
			continue
		}
		w := where + ".fields." + part
		if ok && !slices.Contains(parts, part) {
			add("%s: provider %s has no %s (it fills %s)", w, s.Provider, part, strings.Join(parts, ", "))
		}
		if used[id] {
			add("%s: field %q is written twice", w, id)
		}
		used[id] = true
		f := target.FieldByID(id)
		switch {
		case f == nil:
			add("%s: %q is not a field of entity %q (fields: %s)", w, id, target.ID, fieldIDs(target))
		case f.Computed != nil:
			add("%s: field %q is computed (read-only)", w, id)
		case !slices.Contains(saveFeedTypes[part], f.Type):
			add("%s: field %q is %s; %s needs a %s field", w, id, f.Type, part, saveFeedTypeNames(part))
		}
	}
	if s.Fields.Summary == "" && s.Fields.Min == "" && s.Fields.Max == "" && s.Fields.Drop == "" {
		add("%s: fields needs at least one of summary, min, max, drop", where)
	}
	for _, f := range target.Fields {
		// min / max / drop may be missing from a snapshot; required fields
		// must be the always-present date or summary
		if f.Required && f.Computed == nil && f.ID != s.Fields.Date && f.ID != s.Fields.Summary {
			add("%s: required field %q of entity %q is not filled by saveFeed (map it to date or summary, or make it optional)", where, f.ID, target.ID)
		}
	}
}

func saveFeedTypeNames(part string) string {
	ts := saveFeedTypes[part]
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = string(t)
	}
	return strings.Join(out, " / ")
}

// checkSaveFeedCount caps the saveFeed actions of an app.
func checkSaveFeedCount(a *App, add func(string, ...any)) {
	n := 0
	for i := range a.ServerRules {
		for _, act := range a.ServerRules[i].Do {
			if act.SaveFeed != nil {
				n++
			}
		}
	}
	if n > MaxSaveFeedActions {
		add("serverRules: too many saveFeed actions (%d > %d)", n, MaxSaveFeedActions)
	}
}

// describeSaveFeed is the creator's sentence for one saveFeed.
func describeSaveFeed(s *RuleSaveFeed, ename func(string) string) string {
	what := map[string]string{FeedWeather: "天気", FeedPressure: "気圧", FeedWarnings: "警報・注意報"}[s.Provider]
	return fmt.Sprintf("%sの%sを「%s」に記録する", WeatherRegions[s.Region], what, ename(s.Entity))
}
