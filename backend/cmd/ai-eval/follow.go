package main

import (
	"fmt"
	"regexp"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Follows (#270) expect keys, checked by checkFollows:
//   - followFilter (bool): a component shows only people I follow ({"myFollows": true}).
//   - followButton (bool): a follow / unfollow action exists.
//   - followEntity (bool): a hand-made follow entity (follows / フォロー) exists;
//     false = the relation is left to appconfig as followRules asks.
//
// Cases: internal/ai/eval/follow_cases.json (-cases) and
// follow_chat_cases.json (-chat -cases); m0disc_cases.json d_sns_follow.
func init() {
	expectKeys["followFilter"] = expectBool
	expectKeys["followButton"] = expectBool
	expectKeys["followEntity"] = expectBool
}

var followEntityRe = regexp.MustCompile(`(?i)follow|フォロー`)

// followEntities lists entities that look like a hand-made follow list.
func followEntities(a *dsl.App) []string {
	var out []string
	for _, e := range a.Entities {
		if followEntityRe.MatchString(e.ID) || followEntityRe.MatchString(e.Name) {
			out = append(out, e.ID)
		}
	}
	return out
}

func checkFollows(exp map[string]any, a *dsl.App) []string {
	var miss []string
	filters, actions := dsl.FollowUses(a)
	for _, c := range []struct {
		key string
		got []string
	}{{"followFilter", filters}, {"followButton", actions}, {"followEntity", followEntities(a)}} {
		if want, ok := exp[c.key].(bool); ok && (len(c.got) > 0) != want {
			miss = append(miss, fmt.Sprintf("%s=%v", c.key, c.got))
		}
	}
	return miss
}
