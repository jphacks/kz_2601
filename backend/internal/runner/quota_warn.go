package runner

import (
	"context"
	"log/slog"
	"strconv"

	"github.com/jphacks/kz_2601/backend/internal/runner/notify"
	"github.com/jphacks/kz_2601/backend/internal/runner/quota"
	"github.com/jphacks/kz_2601/backend/internal/runner/scope"
)

// QuotaWarnEvent is the message (and `event` attr) of the line written when
// the creator's daily-limit push is claimed (#345). Cloud Logging filter:
// jsonPayload.event="runner.quota_warn".
const QuotaWarnEvent = "runner.quota_warn"

// warnQuota pushes the creator once a day per level when the run reached
// 80% or 100% of the app's daily executions (#345). Best effort: nothing
// here fails the run. Under a notify pause nothing is claimed, so the push
// can still go out the same day once the pause ends.
func (r *Runner) warnQuota(ctx context.Context, h *scope.Handle, dec quota.Decision, appID, appName, creator, notifyPaused string) {
	level := dec.Warning()
	if level == 0 || creator == "" || notifyPaused != "" {
		return
	}
	first, err := r.cfg.Quota.ClaimWarning(ctx, dec, level)
	if err != nil {
		r.cfg.Logger.WarnContext(ctx, "runner: claiming the quota warning failed", slog.String("app_id", appID), slog.Any("error", err))
		return
	}
	if !first {
		return
	}
	title, body := quotaWarnText(level, appName)
	var tokens notify.TokenReader = handleTokens{h: h, env: r.cfg.PushEnvironment}
	if r.cfg.Tokens != nil {
		tokens = r.cfg.Tokens
	}
	disp := notify.Dispatcher{Tokens: tokens, Sender: r.cfg.Sender}
	out, err := disp.Dispatch(ctx, notify.Resolution{Action: level, To: []string{creator}},
		notify.Content{Key: "quota_warn/" + appID + "/" + dec.Day, AppID: appID, AppName: appName, Title: title, Body: body})
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
		slog.String("event", QuotaWarnEvent), slog.String("app_id", appID), slog.String("day", dec.Day),
		slog.Int("level", level), slog.String("result", result),
	}
	if err != nil {
		attrs = append(attrs, slog.Any("error", err))
	}
	r.cfg.Logger.LogAttrs(ctx, slog.LevelInfo, QuotaWarnEvent, attrs...)
}

// quotaWarnText is the push of level. Japanese only: the creator's language
// is not stored.
func quotaWarnText(level int, appName string) (title, body string) {
	name := "このアプリ"
	if appName != "" {
		name = "『" + appName + "』"
	}
	if level >= quota.WarnFull {
		return "今日の自動の動きは止まっています",
			name + "の自動の動きは、今日の上限に達したため、今日は止まっています。明日また動きます"
	}
	return "今日の上限が近づいています",
		name + "の自動の動きが、今日の上限の " + strconv.Itoa(level) + "% に達しました"
}
