package notify

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/runner/engine"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
	"github.com/jphacks/kz_2601/backend/pkg/dsl/expreval"
)

// fakeDir is a Directory for one app.
type fakeDir struct {
	owner   string
	members []string
	authors map[string]string // "entity/id" -> author
	err     error
}

func (d *fakeDir) Owner(context.Context) (string, error)     { return d.owner, d.err }
func (d *fakeDir) Members(context.Context) ([]string, error) { return d.members, d.err }
func (d *fakeDir) RecordAuthor(_ context.Context, entity, id string) (string, bool, error) {
	a, ok := d.authors[entity+"/"+id]
	return a, ok, d.err
}

type fakeAccounts struct {
	deleted map[string]bool
	blocks  map[string][]string
}

func (a *fakeAccounts) Deleted(_ context.Context, uid string) (bool, error) {
	return a.deleted[uid], nil
}
func (a *fakeAccounts) Blocks(_ context.Context, uid string) ([]string, error) {
	return a.blocks[uid], nil
}

type allConsent struct{}

func (allConsent) Allows(context.Context, string) (bool, error) { return true, nil }

type consentSet map[string]bool

func (c consentSet) Allows(_ context.Context, uid string) (bool, error) { return c[uid], nil }

// ages: every account was created long ago unless listed.
type ages map[string]time.Time

func (a ages) CreatedAt(_ context.Context, uid string) (time.Time, bool, error) {
	if t, ok := a[uid]; ok {
		return t, true, nil
	}
	return time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), true, nil
}

type capLimiter struct{ max int }

func (c capLimiter) Limit(_ context.Context, uids []string) ([]string, []LimitDrop, error) {
	if len(uids) <= c.max {
		return uids, nil, nil
	}
	var d []LimitDrop
	for _, u := range uids[c.max:] {
		d = append(d, LimitDrop{UID: u, Scope: "recipient_app"})
	}
	return uids[:c.max], d, nil
}

var now = time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)

// The cast: owner o, members m1 m2, record author m1, ref target written by m2,
// outsider x (writes in public apps).
func world() (*fakeDir, *fakeAccounts) {
	return &fakeDir{owner: "o", members: []string{"m1", "m2"},
			authors: map[string]string{"events/e1": "m2", "events/eo": "o", "events/ex": "x"}},
		&fakeAccounts{deleted: map[string]bool{}, blocks: map[string][]string{}}
}

func resolver(t *testing.T, cfg Config) *Resolver {
	t.Helper()
	if cfg.Limiter == nil {
		cfg.Limiter = Unlimited{}
	}
	r, err := NewResolver(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func trig(scope dsl.EntityScope, actor, author string) engine.Trigger {
	return engine.Trigger{AppID: "a1", Scope: scope, Kind: engine.KindCreated, Entity: "entries",
		Record: expreval.Record{ID: "r1", Author: author}, Actor: actor, At: now}
}

func plan(kind string) engine.PlannedNotify {
	n := engine.PlannedNotify{Action: 0, To: dsl.RuleRecipient{Kind: kind}, Title: "t", Body: "b"}
	if kind == dsl.RecipientAuthorOf {
		n.To.Field = "event"
		n.RefEntity, n.RefID = "events", "e1"
	}
	return n
}

// role × kind × scope. Consent and age are open here so only the scope,
// participants and the actor decide.
func TestResolveTable(t *testing.T) {
	cases := []struct {
		name    string
		scope   dsl.EntityScope
		actor   string // also the trigger record's author unless author is set
		author  string
		kind    string
		to      []string
		refused Reason
		skipped []Skip
	}{
		{"private owner→owner (actor is owner)", dsl.ScopePrivate, "o", "", dsl.RecipientOwner, nil, "", []Skip{{UID: "o", Reason: ReasonActor}}},
		{"private scheduled→owner", dsl.ScopePrivate, "", "", dsl.RecipientOwner, []string{"o"}, "", nil},
		{"\"\" scope is private", "", "", "", dsl.RecipientOwner, []string{"o"}, "", nil},
		{"private author refused", dsl.ScopePrivate, "o", "", dsl.RecipientAuthor, nil, ReasonScope, nil},
		{"private authorOf refused", dsl.ScopePrivate, "o", "", dsl.RecipientAuthorOf, nil, ReasonScope, nil},
		{"private members refused", dsl.ScopePrivate, "o", "", dsl.RecipientMembers, nil, ReasonScope, nil},

		{"shared member→owner", dsl.ScopeShared, "m1", "", dsl.RecipientOwner, []string{"o"}, "", nil},
		{"shared owner→owner", dsl.ScopeShared, "o", "", dsl.RecipientOwner, nil, "", []Skip{{UID: "o", Reason: ReasonActor}}},
		{"shared member→author (self)", dsl.ScopeShared, "m1", "", dsl.RecipientAuthor, nil, "", []Skip{{UID: "m1", Reason: ReasonActor}}},
		{"shared member→authorOf", dsl.ScopeShared, "m1", "", dsl.RecipientAuthorOf, []string{"m2"}, "", nil},
		{"shared member→members", dsl.ScopeShared, "m1", "", dsl.RecipientMembers, []string{"o", "m2"}, "", []Skip{{UID: "m1", Reason: ReasonActor}}},
		{"shared owner→members", dsl.ScopeShared, "o", "", dsl.RecipientMembers, []string{"m1", "m2"}, "", []Skip{{UID: "o", Reason: ReasonActor}}},
		{"shared scheduled→members", dsl.ScopeShared, "", "", dsl.RecipientMembers, []string{"o", "m1", "m2"}, "", nil},
		{"shared scheduled→author who left", dsl.ScopeShared, "", "x", dsl.RecipientAuthor, nil, "", []Skip{{UID: "x", Reason: ReasonNotMember}}},

		{"public outsider→owner", dsl.ScopePublic, "x", "", dsl.RecipientOwner, []string{"o"}, "", nil},
		{"public outsider→authorOf", dsl.ScopePublic, "x", "", dsl.RecipientAuthorOf, []string{"m2"}, "", nil},
		{"public owner→authorOf", dsl.ScopePublic, "o", "", dsl.RecipientAuthorOf, []string{"m2"}, "", nil},
		{"public author refused", dsl.ScopePublic, "x", "", dsl.RecipientAuthor, nil, ReasonScope, nil},
		{"public members refused", dsl.ScopePublic, "x", "", dsl.RecipientMembers, nil, ReasonScope, nil},
		{"unknown scope refused", "match", "x", "", dsl.RecipientOwner, nil, ReasonScope, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir, acc := world()
			r := resolver(t, Config{Directory: dir, Accounts: acc, Consent: allConsent{}, Age: ages{}})
			author := c.author
			if author == "" {
				author = c.actor
			}
			got, err := r.Resolve(context.Background(), trig(c.scope, c.actor, author), plan(c.kind))
			if err != nil {
				t.Fatal(err)
			}
			if got.Refused != c.refused || !slices.Equal(got.To, c.to) || !reflect.DeepEqual(got.Skipped, c.skipped) {
				t.Errorf("got refused=%q to=%v skipped=%v; want %q %v %v", got.Refused, got.To, got.Skipped, c.refused, c.to, c.skipped)
			}
		})
	}
}

func TestKindAllowedMatchesDSL(t *testing.T) {
	want := map[dsl.EntityScope][]string{
		dsl.ScopePrivate: {dsl.RecipientOwner},
		dsl.ScopeShared:  {dsl.RecipientOwner, dsl.RecipientAuthor, dsl.RecipientAuthorOf, dsl.RecipientMembers},
		dsl.ScopePublic:  {dsl.RecipientOwner, dsl.RecipientAuthorOf},
	}
	for scope, kinds := range want {
		for _, k := range []string{dsl.RecipientOwner, dsl.RecipientAuthor, dsl.RecipientAuthorOf, dsl.RecipientMembers, "everyone"} {
			if got := KindAllowed(scope, k); got != slices.Contains(kinds, k) {
				t.Errorf("KindAllowed(%s, %s) = %v", scope, k, got)
			}
		}
	}
}

func TestResolveExcludes(t *testing.T) {
	ctx := context.Background()
	t.Run("deleted", func(t *testing.T) {
		dir, acc := world()
		acc.deleted["m2"] = true
		r := resolver(t, Config{Directory: dir, Accounts: acc, Consent: allConsent{}, Age: ages{}})
		got, _ := r.Resolve(ctx, trig(dsl.ScopeShared, "m1", "m1"), plan(dsl.RecipientMembers))
		if !slices.Equal(got.To, []string{"o"}) || !slices.Contains(got.Skipped, Skip{UID: "m2", Reason: ReasonDeleted}) {
			t.Errorf("got %+v", got)
		}
	})
	t.Run("blocked by the actor", func(t *testing.T) {
		dir, acc := world()
		acc.blocks["m1"] = []string{"m2"}
		r := resolver(t, Config{Directory: dir, Accounts: acc, Consent: allConsent{}, Age: ages{}})
		got, _ := r.Resolve(ctx, trig(dsl.ScopeShared, "m1", "m1"), plan(dsl.RecipientAuthorOf))
		if len(got.To) != 0 || !reflect.DeepEqual(got.Skipped, []Skip{{UID: "m2", Reason: ReasonBlocked}}) {
			t.Errorf("got %+v", got)
		}
	})
	t.Run("recipient blocked the actor", func(t *testing.T) {
		dir, acc := world()
		acc.blocks["o"] = []string{"x"}
		r := resolver(t, Config{Directory: dir, Accounts: acc, Consent: allConsent{}, Age: ages{}})
		got, _ := r.Resolve(ctx, trig(dsl.ScopePublic, "x", "x"), plan(dsl.RecipientOwner))
		if len(got.To) != 0 || !reflect.DeepEqual(got.Skipped, []Skip{{UID: "o", Reason: ReasonBlocked}}) {
			t.Errorf("got %+v", got)
		}
	})
	t.Run("left the app", func(t *testing.T) {
		dir, acc := world()
		dir.members = []string{"m1"} // m2 left; the ref target is still m2's
		r := resolver(t, Config{Directory: dir, Accounts: acc, Consent: allConsent{}, Age: ages{}})
		got, _ := r.Resolve(ctx, trig(dsl.ScopeShared, "m1", "m1"), plan(dsl.RecipientAuthorOf))
		if len(got.To) != 0 || !reflect.DeepEqual(got.Skipped, []Skip{{UID: "m2", Reason: ReasonNotMember}}) {
			t.Errorf("got %+v", got)
		}
	})
	t.Run("public authorOf needs no membership", func(t *testing.T) {
		dir, acc := world()
		n := plan(dsl.RecipientAuthorOf)
		n.RefID = "ex"
		r := resolver(t, Config{Directory: dir, Accounts: acc, Consent: allConsent{}, Age: ages{}})
		got, _ := r.Resolve(ctx, trig(dsl.ScopePublic, "m1", "m1"), n)
		if !slices.Equal(got.To, []string{"x"}) {
			t.Errorf("got %+v", got)
		}
	})
	t.Run("nobody", func(t *testing.T) {
		dir, acc := world()
		r := resolver(t, Config{Directory: dir, Accounts: acc, Consent: allConsent{}, Age: ages{}})
		for _, n := range []engine.PlannedNotify{
			{To: dsl.RuleRecipient{Kind: dsl.RecipientAuthorOf, Field: "event"}},                                     // empty ref
			{To: dsl.RuleRecipient{Kind: dsl.RecipientAuthorOf, Field: "event"}, RefEntity: "events", RefID: "gone"}, // record gone
		} {
			got, _ := r.Resolve(ctx, trig(dsl.ScopeShared, "m1", "m1"), n)
			if got.Refused != ReasonNoRecipient {
				t.Errorf("got %+v", got)
			}
		}
		dir.owner = ""
		got, _ := r.Resolve(ctx, trig(dsl.ScopeShared, "m1", "m1"), plan(dsl.RecipientOwner))
		if got.Refused != ReasonNoRecipient {
			t.Errorf("no owner: got %+v", got)
		}
	})
	t.Run("limit", func(t *testing.T) {
		dir, acc := world()
		r := resolver(t, Config{Directory: dir, Accounts: acc, Consent: allConsent{}, Age: ages{}, Limiter: capLimiter{max: 1}})
		got, _ := r.Resolve(ctx, trig(dsl.ScopeShared, "", ""), plan(dsl.RecipientMembers))
		want := []Skip{{UID: "m1", Reason: ReasonRecipientLimit, Limit: "recipient_app"}, {UID: "m2", Reason: ReasonRecipientLimit, Limit: "recipient_app"}}
		if !slices.Equal(got.To, []string{"o"}) || !reflect.DeepEqual(got.Skipped, want) {
			t.Errorf("got %+v", got)
		}
	})
	t.Run("read error", func(t *testing.T) {
		dir, acc := world()
		dir.err = errors.New("boom")
		r := resolver(t, Config{Directory: dir, Accounts: acc})
		if _, err := r.Resolve(ctx, trig(dsl.ScopeShared, "m1", "m1"), plan(dsl.RecipientOwner)); err == nil {
			t.Error("want error")
		}
	})
}

// Defaults: no consent store → others are not notified, the owner is.
func TestResolveConsentDefault(t *testing.T) {
	dir, acc := world()
	r := resolver(t, Config{Directory: dir, Accounts: acc, Age: ages{}})
	got, err := r.Resolve(context.Background(), trig(dsl.ScopeShared, "", ""), plan(dsl.RecipientMembers))
	if err != nil {
		t.Fatal(err)
	}
	want := []Skip{{UID: "m1", Reason: ReasonNoConsent}, {UID: "m2", Reason: ReasonNoConsent}}
	if !slices.Equal(got.To, []string{"o"}) || !reflect.DeepEqual(got.Skipped, want) {
		t.Errorf("got %+v", got)
	}

	// with a consent store, only those who agreed
	r = resolver(t, Config{Directory: dir, Accounts: acc, Age: ages{}, Consent: consentSet{"m2": true}})
	got, _ = r.Resolve(context.Background(), trig(dsl.ScopeShared, "", ""), plan(dsl.RecipientMembers))
	if !slices.Equal(got.To, []string{"o", "m2"}) {
		t.Errorf("got %+v", got)
	}
}

// Defaults: unknown account age → the actor counts as new.
func TestResolveNewAccount(t *testing.T) {
	ctx := context.Background()
	dir, acc := world()

	r := resolver(t, Config{Directory: dir, Accounts: acc, Consent: allConsent{}})
	got, _ := r.Resolve(ctx, trig(dsl.ScopeShared, "m1", "m1"), plan(dsl.RecipientOwner))
	if len(got.To) != 0 || !reflect.DeepEqual(got.Skipped, []Skip{{UID: "o", Reason: ReasonNewAccount}}) {
		t.Errorf("unknown age: got %+v", got)
	}
	// scheduled: no actor, no age check
	got, _ = r.Resolve(ctx, trig(dsl.ScopeShared, "", ""), plan(dsl.RecipientOwner))
	if !slices.Equal(got.To, []string{"o"}) {
		t.Errorf("scheduled: got %+v", got)
	}

	for _, c := range []struct {
		age  time.Duration
		sent bool
	}{{time.Hour, false}, {NewAccountWindow - time.Second, false}, {NewAccountWindow, true}, {48 * time.Hour, true}} {
		r := resolver(t, Config{Directory: dir, Accounts: acc, Consent: allConsent{}, Age: ages{"m1": now.Add(-c.age)}})
		got, _ := r.Resolve(ctx, trig(dsl.ScopeShared, "m1", "m1"), plan(dsl.RecipientOwner))
		if (len(got.To) == 1) != c.sent {
			t.Errorf("age %v: got %+v", c.age, got)
		}
	}
}

func TestNewResolverRequires(t *testing.T) {
	if _, err := NewResolver(Config{}); err == nil {
		t.Error("want error")
	}
}
