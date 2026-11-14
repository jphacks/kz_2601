package dsl

import (
	"slices"
	"testing"
)

func TestRecipientAllowed(t *testing.T) {
	kinds := []string{RecipientOwner, RecipientAuthor, RecipientAuthorOf, RecipientMembers, RecipientRecipients}
	want := map[EntityScope][]string{
		ScopePrivate: {RecipientOwner},
		ScopeShared:  kinds,
		ScopePublic:  {RecipientOwner, RecipientAuthorOf},
		"":           nil,
		"secret":     nil,
	}
	for scope, allowed := range want {
		for _, k := range append(slices.Clone(kinds), "", "everyone") {
			if got := RecipientAllowed(scope, k); got != slices.Contains(allowed, k) {
				t.Errorf("RecipientAllowed(%q, %q) = %v", scope, k, got)
			}
		}
	}
}

func TestNotifyPlaceholderNames(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"予定があります", nil},
		{"{title} が来ました", []string{"title"}},
		{"{title} を {author} が {title}", []string{"title", "author"}},
		{"{a}{b}{a}", []string{"a", "b"}},
		{"{}", []string{""}},
		{"{{name}}", []string{"name"}},
		{"{a{b}c}", []string{"b"}},
		{"{開いたまま", nil},
		{"閉じただけ}", nil},
		{"{金額 円}", []string{"金額 円"}},
	}
	for _, c := range cases {
		if got := NotifyPlaceholderNames(c.in); !slices.Equal(got, c.want) {
			t.Errorf("NotifyPlaceholderNames(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
