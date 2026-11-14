package dsl

import (
	"fmt"
	"strings"
	"testing"
)

// collectAdds runs check with an add that records each message.
func collectAdds(check func(add func(string, ...any))) []string {
	var got []string
	check(func(format string, a ...any) { got = append(got, fmt.Sprintf(format, a...)) })
	return got
}

func TestCheckDeleteBy(t *testing.T) {
	for _, tc := range []struct {
		name string
		e    Entity
		want string // "" = no problem
	}{
		{"unset", Entity{ID: "posts"}, ""},
		{"unset on private", Entity{ID: "posts", Scope: ScopePrivate}, ""},
		{"author on shared", Entity{ID: "posts", Scope: ScopeShared, DeleteBy: DeleteByAuthor}, ""},
		{"admins on public", Entity{ID: "posts", Scope: ScopePublic, DeleteBy: DeleteByAdmins}, ""},
		{"authorOrAdmins on shared", Entity{ID: "posts", Scope: ScopeShared, DeleteBy: DeleteByAuthorOrAdmins}, ""},
		{"unknown value", Entity{ID: "posts", Scope: ScopeShared, DeleteBy: "everyone"},
			`entity "posts" deleteBy "everyone" must be author, admins or authorOrAdmins`},
		{"private default scope", Entity{ID: "notes", DeleteBy: DeleteByAdmins},
			`entity "notes" deleteBy needs scope shared or public`},
		{"explicit private", Entity{ID: "notes", Scope: ScopePrivate, DeleteBy: DeleteByAuthor},
			`entity "notes" deleteBy needs scope shared or public`},
		{"match entity", Entity{ID: "cards", Scope: ScopeShared, Match: MatchProfile, DeleteBy: DeleteByAdmins},
			`entity "cards" deleteBy: a match "profile" entity is deleted by its author only`},
		{"single entity", Entity{ID: "settings", Scope: ScopeShared, Single: true, DeleteBy: DeleteByAuthorOrAdmins},
			`entity "settings" deleteBy: a single entity has no delete`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := collectAdds(func(add func(string, ...any)) { checkDeleteBy(&tc.e, add) })
			if tc.want == "" {
				if len(got) != 0 {
					t.Fatalf("want no problem, got %q", got)
				}
				return
			}
			if len(got) != 1 || !strings.HasPrefix(got[0], tc.want) {
				t.Fatalf("got %q, want one starting with %q", got, tc.want)
			}
		})
	}
}

func TestCheckNotification(t *testing.T) {
	const where = "app.notifications[2]"
	ok := Notification{ID: "remind", Title: "水やり", Repeat: "weekly", Weekday: "mon", Time: "07:30"}
	for _, tc := range []struct {
		name string
		mod  func(n *Notification)
		want []string
	}{
		{"valid", func(*Notification) {}, nil},
		{"valid once without time", func(n *Notification) { n.Repeat, n.Weekday, n.Time = "once", "", "" }, nil},
		{"valid monthly with entity", func(n *Notification) { n.Repeat, n.Weekday, n.Entity = "monthly", "", "subs" }, nil},
		{"bad id", func(n *Notification) { n.ID = "Remind!" },
			[]string{where + `: notification id "Remind!" is not a valid id`}},
		{"empty id", func(n *Notification) { n.ID = "" },
			[]string{where + `: notification id "" is not a valid id`}},
		{"empty title", func(n *Notification) { n.Title = "" },
			[]string{where + ": notification title is empty"}},
		{"weekly without weekday", func(n *Notification) { n.Weekday = "" },
			[]string{where + ": weekly notification needs weekday"}},
		{"monthly without entity", func(n *Notification) { n.Repeat, n.Weekday = "monthly", "" },
			[]string{where + ": repeat monthly needs entity + field"}},
		{"yearly without entity", func(n *Notification) { n.Repeat, n.Weekday = "yearly", "" },
			[]string{where + ": repeat yearly needs entity + field"}},
		{"time not HH:MM", func(n *Notification) { n.Time = "7:30" },
			[]string{where + `: notification time "7:30" must be HH:MM`}},
		{"time out of range", func(n *Notification) { n.Time = "24:00" },
			[]string{where + `: notification time "24:00" must be HH:MM`}},
		{"every problem in order", func(n *Notification) { n.ID, n.Title, n.Weekday, n.Time = "", "", "", "x" },
			[]string{
				where + `: notification id "" is not a valid id`,
				where + ": notification title is empty",
				where + ": weekly notification needs weekday",
				where + `: notification time "x" must be HH:MM`,
			}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n := ok
			tc.mod(&n)
			got := collectAdds(func(add func(string, ...any)) { checkNotification(where, &n, add) })
			if len(got) != len(tc.want) {
				t.Fatalf("got %q, want %d problems %q", got, len(tc.want), tc.want)
			}
			for i, w := range tc.want {
				if !strings.HasPrefix(got[i], w) {
					t.Errorf("[%d] = %q, want prefix %q", i, got[i], w)
				}
			}
		})
	}
}
