package ai

import (
	"context"
	"strings"
	"sync"

	"github.com/jphacks/kz_2601/backend/internal/paid"
)

// Push wording: the static prompts say server push is impossible (chat
// threads, chatLimits). When the server-rules pass (#343) will run for the
// request, that is wrong: the pass adds the notifications after the main
// call. So, like restrictPrompt, the assembled prompt is rewritten per
// request; prompt.go / catalog.go and the default bytes stay untouched.

// chatThreadPushCannot is the chatThreadRules line the pass makes untrue.
const chatThreadPushCannot = "- 新しいメッセージの通知(プッシュ)はできない。求められたら「新しいメッセージの通知は今はできないため、開いた時にすぐ届く形で代替しました。」と申告する。"

// chatThreadPushLater replaces it when the pass runs.
const chatThreadPushLater = "- 新しい記録・メッセージが来た時の参加者・作った人への通知(プッシュ)は、この後の工程でサーバーの自動の動き(serverRules)として作られる。この設計図には通知のための部品や記録を足さず、summary に通知の「代替」や「通知はできない」を書かない。summary で料金に触れなくてよい。"

// chatLimitPushItem is the chatLimits entry the pass makes untrue.
const chatLimitPushItem = "サーバーからのプッシュ通知(代わり → 時刻指定のローカル通知)、"

// serverPushChatRule is added to the chat prompts when the pass runs.
const serverPushChatRule = "- サーバーからの通知(#343): 新しい記録・メッセージが来た時に参加者や作った人へ届く通知、決まった時刻にサーバーから届く通知は**できる**(アプリを開いていなくても届く。アプリを作る時にサーバーの自動の動きとして作る)。サーバーの自動の動きはアプリを作った人の Tailor It Pro の機能(参加した人には要らない)。text で料金に触れなくてよい(要る時は画面が案内する)。"

type serverPushKey struct{}

// withServerPush marks ctx: the server-rules pass will run for this request.
func withServerPush(ctx context.Context, on bool) context.Context {
	if !on {
		return ctx
	}
	return context.WithValue(ctx, serverPushKey{}, true)
}

func serverPushFrom(ctx context.Context) bool {
	on, _ := ctx.Value(serverPushKey{}).(bool)
	return on
}

// serverRulesOn reports whether the server-rules pass may add rules for this
// request: the flag is on, the build has shared data and the transcript is
// not 「この機能なしで作る」. applyServerRulesPass and the push wording share it.
func (s *Service) serverRulesOn(ctx context.Context, transcript string) bool {
	return s.serverRulesEnabled() && !restrictionFrom(ctx).NoSharedData() &&
		!defaultPaidCatalog.dropped(transcript, paid.ServerRules)
}

var serverPushMemo sync.Map // system -> rewritten

// serverPushPrompt rewrites the push lines when ctx says the pass runs.
// Run after restrictPrompt (its replacements match the original rules).
func serverPushPrompt(ctx context.Context, system string) string {
	if !serverPushFrom(ctx) {
		return system
	}
	if v, ok := serverPushMemo.Load(system); ok {
		return v.(string)
	}
	v, _ := serverPushMemo.LoadOrStore(system, rewriteServerPush(system))
	return v.(string)
}

func rewriteServerPush(s string) string {
	s = strings.Replace(s, chatThreadPushCannot, chatThreadPushLater, 1)
	if strings.Contains(s, chatLimitPushItem) {
		s = strings.Replace(s, chatLimitPushItem, "", 1)
		s = strings.Replace(s, "\n"+publicPIIChatRule, "\n"+publicPIIChatRule+"\n"+serverPushChatRule, 1)
	}
	return s
}
