package ai

import (
	"context"
	"errors"
	"log/slog"
	"regexp"

	"github.com/jphacks/kz_2601/backend/internal/auth"
	"github.com/jphacks/kz_2601/backend/internal/paid"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// The creator of a shared app pays for its online AI (docs/00_decisions.md:
// 共有のアプリの費用は作成者の Pro で持つ). POST /v1/text with the appId of a
// shared app the caller participates in is covered by the creator's Pro.
// Membership is read on the server (app_owners + apps_public/{appId}/members).

// AppMembers is the read-only membership lookup (store.Firestore / store.Memory).
type AppMembers interface {
	AppOwner(ctx context.Context, appID string) (string, error)
	GetMember(ctx context.Context, appID, uid string) (*store.Member, error)
}

// reasonCreatorNeedsPro is details.reason when the caller is a participant
// and the creator has no Pro (same value as appconfig).
const reasonCreatorNeedsPro = "creator_needs_pro"

// textAppIDRe is appconfig's appId rule.
var textAppIDRe = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`)

func validTextAppID(s string) bool {
	return s == "" || (textAppIDRe.MatchString(s) && s != "." && s != "..")
}

// sharedCreator returns appID's creator when uid is a member of it (and not
// the creator), "" otherwise. A failed lookup counts as "not a member".
func (s *Service) sharedCreator(ctx context.Context, appID, uid string) string {
	if s.Members == nil || appID == "" || uid == "" {
		return ""
	}
	owner, err := s.Members.AppOwner(ctx, appID)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			s.logger().WarnContext(ctx, "ai: app owner lookup failed", slog.Any("error", err))
		}
		return ""
	}
	if owner == "" || owner == uid {
		return ""
	}
	if _, err := s.Members.GetMember(ctx, appID, uid); err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			s.logger().WarnContext(ctx, "ai: member lookup failed", slog.Any("error", err))
		}
		return ""
	}
	return owner
}

// gateText applies the onlineAI gate (#316) and returns ctx marked with the
// payer. The caller's own Pro comes first; a participant of a shared app is
// covered by the creator's.
func (s *Service) gateText(ctx context.Context, req TextRequest) (context.Context, error) {
	if !validTextAppID(req.AppID) {
		return ctx, badText("invalid appId")
	}
	uid := auth.UID(ctx)
	if s.Pro == nil || !s.ProMode.Checks() || s.Pro.IsPro(ctx, uid) {
		return ctx, nil
	}
	if owner := s.sharedCreator(ctx, req.AppID, uid); owner != "" {
		if s.Pro.IsPro(ctx, owner) || !s.ProMode.Stop(ctx, s.logger(), "ai.text", owner, []string{paid.OnlineAI}) {
			return withPayer(ctx, owner), nil
		}
		e := onlineAIRequired(req.Locale)
		e.Details["reason"] = reasonCreatorNeedsPro
		return ctx, e
	}
	if !s.ProMode.Stop(ctx, s.logger(), "ai.text", uid, []string{paid.OnlineAI}) {
		return ctx, nil
	}
	return ctx, onlineAIRequired(req.Locale)
}

// onlineAIRequired is the 402 of POST /v1/text without a Pro that covers it.
func onlineAIRequired(loc Locale) *Error {
	f := PaidFeature{ID: paid.OnlineAI}
	if d, ok := paid.Default.ByID(paid.OnlineAI); ok {
		f = d.Public(string(loc))
	}
	return ProRequired([]PaidFeature{f})
}

type payerKey struct{}

// withPayer marks the Claude calls of ctx as covered by another user's Pro
// (ai.usage payer_uid_hash). The caller's daily limits stay the caller's.
func withPayer(ctx context.Context, uid string) context.Context {
	return context.WithValue(ctx, payerKey{}, uid)
}

// payerOf is the uid whose Pro covers ctx when it is not the caller, "" otherwise.
func payerOf(ctx context.Context) string {
	p, _ := ctx.Value(payerKey{}).(string)
	if p == "" || p == auth.UID(ctx) {
		return ""
	}
	return p
}
