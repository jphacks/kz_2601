package appconfig

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/store"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Runner outbox (#341): a record write whose serverRules trigger matches
// queues one outbox event in the same transaction (store.Outbox). Off unless
// RUNNER_OUTBOX=1; writes no rule matches cost what they did before.
//
// The rules come from the creator's published DSL for shared / public
// entities (a participant's copy never counts, #337) and from the caller's
// own DSL for private ones.

const (
	runnerOutboxEnv = "RUNNER_OUTBOX"
	outboxLocalEnv  = "local" // APP_ENV unset
)

var outboxEnvRe = regexp.MustCompile(`^[a-z0-9_-]{1,32}$`)

// outboxConfig is RUNNER_OUTBOX + the APP_ENV the events are filed under.
type outboxConfig struct {
	on  bool
	env string
}

func init() {
	registerRoutes(func(h *Handler, _ *http.ServeMux) { h.outbox = outboxFromEnv(os.Getenv) })
}

// outboxFromEnv reads RUNNER_OUTBOX (1 / true / on) and APP_ENV.
func outboxFromEnv(getenv func(string) string) outboxConfig {
	switch strings.ToLower(strings.TrimSpace(getenv(runnerOutboxEnv))) {
	case "1", "true", "on":
	default:
		return outboxConfig{}
	}
	env, ok := runnerEnv(getenv)
	if !ok {
		log.Printf("appconfig: %s=1 but APP_ENV %q cannot name an outbox; the outbox stays off", runnerOutboxEnv, env)
		return outboxConfig{}
	}
	return outboxConfig{on: true, env: env}
}

// runnerEnv is the {env} of runner/{env}/… for APP_ENV; ok=false when
// APP_ENV cannot name one.
func runnerEnv(getenv func(string) string) (env string, ok bool) {
	env = strings.ToLower(strings.TrimSpace(getenv("APP_ENV")))
	switch env {
	case "":
		env = outboxLocalEnv
	case "staging":
		env = "stg" // fsenv counts it as stg too
	}
	return env, outboxEnvRe.MatchString(env)
}

// outboxPlan is what a write to one entity needs to build its events.
type outboxPlan struct {
	env     string
	rules   *dsl.App // the DSL whose serverRules count
	scope   dsl.EntityScope
	appID   string
	entity  string
	uid     string // private: whose records
	version int
	kind    string // dsl.TriggerCreated / TriggerUpdated
	heavy   map[string]bool
	actor   string
	anon    bool
	tz      string // the creator's zone when the writer is the creator ("" = the runner looks it up)
}

// outboxPlanFor returns the plan for a kind (dsl.TriggerCreated / Updated)
// write to ent, or nil when the outbox is off or no rule could match it.
// ent is the entity after withSharing (its scope decides the record set).
func (h *Handler) outboxPlanFor(r *http.Request, cur *store.App, ent *dsl.Entity, kind string) (*outboxPlan, error) {
	if !h.outbox.on {
		return nil, nil
	}
	p := &outboxPlan{env: h.outbox.env, appID: cur.ID, entity: ent.ID, kind: kind, actor: uidOf(r), anon: auth.Anonymous(r.Context())}
	if ent.IsShared() {
		a, pub, err := h.publishedDSL(r.Context(), cur.ID)
		if err != nil || a == nil {
			return nil, err
		}
		owner, err := h.appOwner(r.Context(), cur.ID)
		if err != nil || owner == "" || pub.OwnerUID != owner {
			return nil, err // only the creator's mirror counts
		}
		p.rules, p.version = a, pub.Version
		if owner == p.actor {
			p.tz = headerTimeZone(r)
		}
	} else {
		a, err := h.parsedDSL(uidOf(r), cur)
		if err != nil {
			return nil, err
		}
		p.rules, p.version, p.uid, p.tz = a, cur.CurrentVersion, uidOf(r), headerTimeZone(r)
	}
	src := p.rules.EntityByID(ent.ID)
	// the declaring DSL must put the records on the same side as the write
	if src == nil || src.IsShared() != ent.IsShared() || !p.rules.HasRecordRules(ent.ID, kind) {
		return nil, nil
	}
	p.scope = src.EffectiveScope()
	p.heavy = map[string]bool{}
	for i := range src.Fields {
		switch src.Fields[i].Type {
		case dsl.FieldPhoto, dsl.FieldVideo, dsl.FieldLocation:
			p.heavy[src.Fields[i].ID] = true
		}
	}
	if err := h.checkAppStop(r, p.env, cur.ID); err != nil { // #346
		return nil, err
	}
	return p, nil
}

// emit builds the event of one write: the rules whose trigger and fields
// match, with only the field values those rules read.
func (p *outboxPlan) emit() store.EmitFunc {
	return func(prev *store.Record, next store.Record) []store.OutboxEvent {
		kind := store.OutboxRecordCreated
		var changed []string
		if p.kind == dsl.TriggerUpdated {
			if prev == nil {
				return nil
			}
			kind = store.OutboxRecordUpdated
			changed = changedFields(prev.Data, next.Data)
		}
		rules := p.rules.MatchRecordRules(p.entity, p.kind, changed)
		if len(rules) == 0 {
			return nil
		}
		ids := make([]string, 0, len(rules))
		var fields []string
		for _, r := range rules {
			ids = append(ids, r.ID)
			for _, f := range r.ReferencedFields() {
				if !p.heavy[f] && !slices.Contains(fields, f) {
					fields = append(fields, f)
				}
			}
		}
		slices.Sort(fields)
		ev := store.OutboxEvent{
			Env: p.env, Kind: kind, AppID: p.appID, Scope: string(p.scope), UID: p.uid,
			Entity: p.entity, RecordID: next.ID, Author: next.Author, Version: p.version,
			RuleIDs: ids, ChangedFields: changed, Data: pickFields(next.Data, fields),
			ActorUID: p.actor, ActorAnonymous: p.anon, TimeZone: p.tz,
		}
		if prev != nil {
			ev.Prev = pickFields(prev.Data, fields)
		}
		return []store.OutboxEvent{ev}
	}
}

// changedFields lists (sorted) the keys whose value differs between a and b.
// Values compare by their JSON form: Firestore reads numbers back as int64,
// a request decodes them as float64.
func changedFields(a, b map[string]any) []string {
	var out []string
	for k, v := range b {
		if w, ok := a[k]; !ok || !sameValue(v, w) {
			out = append(out, k)
		}
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			out = append(out, k)
		}
	}
	slices.Sort(out)
	return out
}

func sameValue(a, b any) bool {
	x, err1 := json.Marshal(a)
	y, err2 := json.Marshal(b)
	return err1 == nil && err2 == nil && bytes.Equal(x, y)
}

func pickFields(data map[string]any, fields []string) map[string]any {
	var out map[string]any
	for _, f := range fields {
		if v, ok := data[f]; ok {
			if out == nil {
				out = map[string]any{}
			}
			out[f] = v
		}
	}
	return out
}

// createRecordOutbox is store.CreateRecord, queueing the trigger when a
// rule may match.
func (h *Handler) createRecordOutbox(r *http.Request, cur *store.App, ent *dsl.Entity, rs store.RecordSet, rec store.Record) (*store.Record, bool, error) {
	p, err := h.outboxPlanFor(r, cur, ent, dsl.TriggerCreated)
	if err != nil {
		return nil, false, err
	}
	if p == nil {
		return h.store.CreateRecord(r.Context(), rs, rec)
	}
	return h.store.CreateRecordEmit(r.Context(), rs, rec, p.emit())
}

// updateRecordOutbox is store.UpdateRecord, queueing the trigger when a
// rule may match (the store compares with the record it reads in the same
// transaction).
func (h *Handler) updateRecordOutbox(r *http.Request, cur *store.App, ent *dsl.Entity, rs store.RecordSet, id string, data map[string]any) (*store.Record, error) {
	p, err := h.outboxPlanFor(r, cur, ent, dsl.TriggerUpdated)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return h.store.UpdateRecord(r.Context(), rs, id, data)
	}
	return h.store.UpdateRecordEmit(r.Context(), rs, id, data, p.emit())
}
