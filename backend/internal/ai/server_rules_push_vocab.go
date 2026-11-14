package ai

import "regexp"

// serverRulesPushAsk: 「プッシュして」/ "push me" ask for a notify rule.
// RE2 has no lookahead, so the next character keeps the workout term
// out: プッシュアップ / push-up / pushups do not match.
const serverRulesPushAsk = `|プッシュ(?:$|[^ア])|(?i:\bpush(?:$|[^-\w]))`

func init() {
	serverRulesAskRe = regexp.MustCompile(serverRulesAskRe.String() + serverRulesPushAsk)
}
