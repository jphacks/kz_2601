package runner

import (
	"context"
	"log/slog"

	"github.com/jphacks/kz_2601/backend/internal/coins"
	"github.com/jphacks/kz_2601/backend/internal/runner/notify"
	"github.com/jphacks/kz_2601/backend/internal/runner/quota"
	"github.com/jphacks/kz_2601/backend/internal/runner/scope"
)

// Reasons of an ok run whose writes or notifications the creator's monthly
// allowance cut (#630 part 3). The creator sees the same word as for a run
// it stopped.
const (
	ReasonAllowance   = "allowance_exhausted"
	ReasonOverflowCap = "overflow_cap_reached"
	ReasonCoins       = "coins_insufficient"
)

// allowanceReason is the reason of an ok run cut by stop (quota.Block.Stop).
func allowanceReason(stop string) string {
	switch stop {
	case quota.OutcomeSkippedOverflowCap:
		return ReasonOverflowCap
	case quota.OutcomeSkippedCoins:
		return ReasonCoins
	}
	return ReasonAllowance
}

// AllowanceWarnEvent is the line written when a monthly notice is claimed
// (#630). Cloud Logging filter: jsonPayload.event="runner.allowance_warn".
const AllowanceWarnEvent = "runner.allowance_warn"

// warnAllowance pushes the creator the monthly 80% / 100% notices this run
// reached (allowance per kind, and the overflow coin cap), each once a month
// (quota.ClaimNotice). Best effort, like warnQuota; nothing is claimed under
// a notify pause.
func (r *Runner) warnAllowance(ctx context.Context, h *scope.Handle, dec quota.Decision, appID, appName, creator, notifyPaused string) {
	if creator == "" || notifyPaused != "" {
		return
	}
	for _, n := range dec.Notices() {
		first, err := r.cfg.Quota.ClaimNotice(ctx, dec, n)
		if err != nil {
			r.cfg.Logger.WarnContext(ctx, "runner: claiming the allowance notice failed", slog.String("app_id", appID), slog.Any("error", err))
			continue
		}
		if !first {
			continue
		}
		title, body := allowanceWarnText(n)
		var tokens notify.TokenReader = handleTokens{h: h, env: r.cfg.PushEnvironment}
		if r.cfg.Tokens != nil {
			tokens = r.cfg.Tokens
		}
		disp := notify.Dispatcher{Tokens: tokens, Sender: r.cfg.Sender}
		out, err := disp.Dispatch(ctx, notify.Resolution{Action: n.Level, To: []string{creator}},
			notify.Content{Key: "allowance_warn/" + dec.Month() + "/" + n.Kind, AppID: appID, AppName: appName, Title: title, Body: body})
		result := "sent"
		switch {
		case err != nil:
			result = "error"
		case len(out.Sent) == 0 && len(out.Skipped) > 0:
			result = string(out.Skipped[0].Reason)
		}
		if h != nil {
			r.dropUnregistered(ctx, h, out.Unregistered)
		}
		attrs := []slog.Attr{
			slog.String("event", AllowanceWarnEvent), slog.String("app_id", appID), slog.String("month", dec.Month()),
			slog.String("kind", n.Kind), slog.Int("level", n.Level), slog.Bool("stopped", n.Stopped), slog.String("result", result),
		}
		if err != nil {
			attrs = append(attrs, slog.Any("error", err))
		}
		r.cfg.Logger.LogAttrs(ctx, slog.LevelInfo, AllowanceWarnEvent, attrs...)
	}
}

// allowanceKindName is the creator's word of an allowance kind.
func allowanceKindName(kind string) string {
	switch kind {
	case coins.KindRecipients:
		return "通知"
	case coins.KindWrites:
		return "書き込み"
	case coins.KindReads:
		return "読み取り"
	}
	return "自動の動き"
}

// allowanceWarnText is the push of n. Japanese only, like quotaWarnText. It
// names no app: the allowance is the creator's, over all their apps.
func allowanceWarnText(n quota.Notice) (title, body string) {
	if n.Kind == quota.NoticeCap {
		if n.Level >= quota.WarnFull {
			return "今月のコインの上限に達しました",
				"枠を超えた後のコイン払いが、今月の上限に達しました。自動の動きは来月まで止まります(上限は設定で変えられます)"
		}
		return "今月のコインの上限が近づいています", "枠を超えた後のコイン払いが、今月の上限の 80% に達しました"
	}
	name := allowanceKindName(n.Kind)
	switch {
	case n.Level < quota.WarnFull:
		return "今月の枠が近づいています", "今月の" + name + "の枠の 80% を使いました"
	case n.Stopped:
		return "今月の枠を使い切りました", "今月の" + name + "の枠を使い切ったため、自動の動きは来月まで止まります"
	}
	return "今月の枠を使い切りました", "今月の" + name + "の枠を使い切りました。超えた分はコインで払っています"
}
