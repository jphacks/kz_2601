package calls

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

// A stay in a room lasts as long as its coin holds pay for: from the join,
// BlockMinutes per block (Session.PaidUntil). At PaidUntil the server takes
// the participant out of the room and settles; a settled session cannot
// come back into the room.

// EventRemoved: the server took a participant out of the room.
const EventRemoved = "calls.removed"

// Reasons of EventRemoved.
const (
	removedPaidUp  = "paid_until" // the holds of the session ran out
	removedSettled = "settled"    // a join with a session that already ended
)

// retryRemove is the wait before trying a failed removal again (a few times).
const (
	retryRemove    = 30 * time.Second
	maxRemoveTries = 4
	expireTimeout  = 30 * time.Second
)

// limited reports whether sess is bounded by its holds (charged blocks; a
// dry run, COIN_BILLING_MODE=log, only logs and is not bounded).
func limited(sess Session) bool {
	if len(sess.Blocks) == 0 {
		return false
	}
	for _, b := range sess.Blocks {
		if b.DryRun {
			return false
		}
	}
	return true
}

// paidUntil is when the holds of sess run out, counting from start (zero
// when the session is not bounded or start is unknown).
func (s *Service) paidUntil(sess Session, start time.Time) time.Time {
	if !limited(sess) || start.IsZero() {
		return time.Time{}
	}
	return start.Add(time.Duration(len(sess.Blocks)) * s.Config.BlockDuration())
}

// tokenTTL is how long a token of sess may be used to join: CALL_TOKEN_TTL,
// and never past what the holds pay for (from the join, or from now when
// the caller has not joined yet).
func (s *Service) tokenTTL(sess Session, now time.Time) time.Duration {
	ttl := s.Config.TokenTTL
	start := sess.JoinedAt
	if start.IsZero() {
		start = now
	}
	if end := s.paidUntil(sess, start); !end.IsZero() {
		ttl = min(ttl, end.Sub(now))
	}
	return ttl
}

// remove takes sess's participant out of its room (no Rooms = nothing to do).
func (s *Service) remove(ctx context.Context, sess Session, reason string) error {
	if s.Rooms == nil {
		return nil
	}
	if err := s.Rooms.RemoveParticipant(ctx, sess.Room, sess.ID); err != nil {
		return err
	}
	s.log().InfoContext(ctx, EventRemoved, slog.String("event", EventRemoved), slog.String("uid_hash", httpx.UIDHash(sess.UID)),
		slog.String("appId", sess.AppID), slog.String("reason", reason))
	return nil
}

// Expire ends session id when its holds have run out: the participant is
// taken out of the room and the stay is settled up to PaidUntil. Before
// PaidUntil it only sets the timer again. Idempotent.
func (s *Service) Expire(ctx context.Context, id string) error {
	sess, err := s.Store.Get(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if sess.Settled || sess.PaidUntil.IsZero() {
		s.disarm(id)
		return nil
	}
	if s.now().Before(sess.PaidUntil) {
		s.arm(id, sess.PaidUntil)
		return nil
	}
	if err := s.remove(ctx, sess, removedPaidUp); err != nil {
		return err
	}
	s.disarm(id)
	return s.settle(ctx, id, time.Time{}, sess.PaidUntil)
}

// sweepRoom expires the sessions of room past their PaidUntil (on any
// webhook of that room, so a timer lost with an instance is caught up).
func (s *Service) sweepRoom(ctx context.Context, room string) {
	if room == "" {
		return
	}
	open, err := s.Store.Open(ctx, room)
	if err != nil {
		s.log().WarnContext(ctx, "calls: sweep room failed", slog.Any("error", err))
		return
	}
	now := s.now()
	for _, o := range open {
		if o.PaidUntil.IsZero() || now.Before(o.PaidUntil) {
			continue
		}
		if err := s.Expire(ctx, o.ID); err != nil {
			s.log().WarnContext(ctx, "calls: expire failed", slog.String("uid_hash", httpx.UIDHash(o.UID)), slog.Any("error", err))
		}
	}
}

// arm sets (or moves) the timer of session id to at.
func (s *Service) arm(id string, at time.Time) {
	s.armTry(id, at.Sub(s.now()), 1)
}

func (s *Service) armTry(id string, d time.Duration, try int) {
	after := s.AfterFunc
	if after == nil {
		after = func(d time.Duration, f func()) func() bool { return time.AfterFunc(d, f).Stop }
	}
	s.timersMu.Lock()
	defer s.timersMu.Unlock()
	if stop, ok := s.timers[id]; ok {
		stop()
	}
	if s.timers == nil {
		s.timers = map[string]func() bool{}
	}
	s.timers[id] = after(max(0, d), func() {
		ctx, cancel := context.WithTimeout(context.Background(), expireTimeout)
		defer cancel()
		if err := s.Expire(ctx, id); err != nil {
			s.log().WarnContext(ctx, "calls: expire failed", slog.Int("try", try), slog.Any("error", err))
			if try < maxRemoveTries {
				s.armTry(id, retryRemove, try+1)
				return
			}
			s.disarm(id)
		}
	})
}

// disarm drops the timer of session id.
func (s *Service) disarm(id string) {
	s.timersMu.Lock()
	defer s.timersMu.Unlock()
	if stop, ok := s.timers[id]; ok {
		stop()
		delete(s.timers, id)
	}
}
