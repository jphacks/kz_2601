package calls

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/livekit"

	"github.com/jphacks/kz_2601/backend/internal/coins"
	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

// Errors.
var (
	// ErrDisabled: CALLS_ENABLED is off.
	ErrDisabled = errors.New("calls are off")
	// ErrSessionClosed: a renewal of a session that ended or is not the caller's.
	ErrSessionClosed = errors.New("call session is closed")
)

// Log events.
const (
	EventIssued     = "calls.token_issued"
	EventSettled    = "calls.settled"
	EventUncovered  = "calls.uncovered_minutes" // minutes past the reserved blocks (not charged)
	EventNoJoinTime = "calls.no_join_time"      // a leave without a join time (not charged)
)

// MaxNameRunes bounds the display name shown on the other tiles.
const MaxNameRunes = 40

// Coins is the part of coins.Service calls uses (nil = calls are free).
type Coins interface {
	Reserve(ctx context.Context, r coins.ReserveRequest) (*coins.Hold, error)
	CommitCoins(ctx context.Context, h *coins.Hold, n int64) (coins.Charge, error)
	Extend(ctx context.Context, h *coins.Hold, expireAt time.Time) error
}

// Service issues room tokens and settles the minutes.
type Service struct {
	Config Config
	Store  Store
	Coins  Coins
	Now    func() time.Time
	Log    *slog.Logger
	// Rooms takes people out of a room (nil = nobody is taken out).
	Rooms Rooms
	// NewID makes session ids (tests); nil = 16 random bytes.
	NewID func() string
	// AfterFunc runs f after d and returns its stop (tests); nil = time.AfterFunc.
	AfterFunc func(d time.Duration, f func()) (stop func() bool)

	timersMu sync.Mutex
	timers   map[string]func() bool // session id -> stop of its PaidUntil timer
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *Service) log() *slog.Logger {
	if s.Log != nil {
		return s.Log
	}
	return slog.Default()
}

// Enabled reports CALLS_ENABLED.
func (s *Service) Enabled() bool { return s != nil && s.Config.Enabled }

func (s *Service) newID() string {
	if s.NewID != nil {
		return s.NewID()
	}
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// RoomName is the LiveKit room of an app (and a callButton's room key).
func RoomName(appID, room string) string {
	if room == "" {
		return "app:" + appID
	}
	return "app:" + appID + ":" + room
}

// JoinRequest is one token request. The caller checked the membership.
type JoinRequest struct {
	UID       string
	Anonymous bool
	AppID     string
	// Room is the callButton's room key ("" = the app's room).
	Room string
	// Video: the callButton allows video and the caller asked for it.
	Video bool
	// Name is shown on the other tiles (trimmed to MaxNameRunes).
	Name string
	// SessionID renews the block of a session still in the room.
	SessionID string
}

// Join is the response of POST /v1/apps/{appId}/calls/token.
type Join struct {
	URL       string    `json:"url"`
	Token     string    `json:"token"`
	Room      string    `json:"room"`
	Identity  string    `json:"identity"`
	SessionID string    `json:"sessionId"`
	ExpiresAt time.Time `json:"expiresAt"`
	Video     bool      `json:"video"`
	// BlockMinutes / ReservedUntil: when the app asks again (renewal).
	BlockMinutes  int       `json:"blockMinutes"`
	ReservedUntil time.Time `json:"reservedUntil"`
	// CoinsReserved is this block's hold (0 = not counted).
	CoinsReserved int64 `json:"coinsReserved"`
	PerMinute     int64 `json:"coinsPerMinute"`
}

// Join reserves a block and issues a token. Errors: ErrDisabled,
// ErrSessionClosed, *coins.InsufficientError (402).
func (s *Service) Join(ctx context.Context, r JoinRequest) (Join, error) {
	if !s.Enabled() {
		return Join{}, ErrDisabled
	}
	now := s.now()
	room := RoomName(r.AppID, r.Room)
	sess := Session{ID: r.SessionID, UID: r.UID, AppID: r.AppID, Room: room, Video: r.Video, CreatedAt: now, DeleteAt: now.Add(Retention)}
	renew := r.SessionID != ""
	if renew {
		got, err := s.Store.Get(ctx, r.SessionID)
		if errors.Is(err, ErrNotFound) {
			return Join{}, ErrSessionClosed
		}
		if err != nil {
			return Join{}, err
		}
		if got.UID != r.UID || got.Room != room || got.Settled {
			return Join{}, ErrSessionClosed
		}
		if !got.PaidUntil.IsZero() && !now.Before(got.PaidUntil) {
			// the holds already ran out: the stay ends there
			if err := s.Expire(ctx, got.ID); err != nil {
				s.log().WarnContext(ctx, "calls: expire failed", slog.String("uid_hash", httpx.UIDHash(got.UID)), slog.Any("error", err))
			}
			return Join{}, ErrSessionClosed
		}
		sess = got
		// the earlier blocks are settled only when the caller leaves: keep
		// their holds as long as the new block's, or they expire mid-call
		// and their minutes are never charged
		s.extend(ctx, sess, now.Add(s.Config.BlockDuration()+s.Config.HoldGrace))
	} else {
		sess.ID = s.newID()
	}

	block, err := s.reserve(ctx, r, sess.ID, len(sess.Blocks))
	if err != nil {
		return Join{}, err
	}
	if renew {
		_, err = s.Store.Update(ctx, sess.ID, func(cur *Session) error {
			if cur.Settled {
				return ErrSessionClosed
			}
			if block != nil {
				cur.Blocks = append(cur.Blocks, *block)
			}
			cur.PaidUntil = s.paidUntil(*cur, cur.JoinedAt)
			sess = *cur
			return nil
		})
	} else {
		if block != nil {
			sess.Blocks = []Block{*block}
		}
		err = s.Store.Create(ctx, sess)
	}
	if err != nil {
		// the hold expires on its own (returned to the payer)
		return Join{}, err
	}

	if !sess.PaidUntil.IsZero() {
		s.arm(sess.ID, sess.PaidUntil)
	}
	ttl := s.tokenTTL(sess, now)
	token, err := s.token(sess, r.Name, ttl)
	if err != nil {
		return Join{}, err
	}
	at := now.Truncate(time.Second) // whole seconds: iOS reads plain RFC 3339
	out := Join{URL: s.Config.URL, Token: token, Room: room, Identity: sess.ID, SessionID: sess.ID,
		ExpiresAt: at.Add(ttl).Truncate(time.Second), Video: sess.Video, BlockMinutes: s.Config.BlockMinutes,
		ReservedUntil: at.Add(s.Config.BlockDuration()), PerMinute: s.Config.PerMinute}
	if block != nil && !block.DryRun {
		out.CoinsReserved = block.Amount
	}
	s.log().InfoContext(ctx, EventIssued, slog.String("event", EventIssued), slog.String("uid_hash", httpx.UIDHash(r.UID)),
		slog.String("appId", r.AppID), slog.Bool("renew", renew), slog.Bool("video", sess.Video), slog.Int64("coins", out.CoinsReserved))
	return out, nil
}

// extend keeps the holds of sess's blocks until expireAt (a failure is
// logged: that block may then go back uncharged).
func (s *Service) extend(ctx context.Context, sess Session, expireAt time.Time) {
	if s.Coins == nil {
		return
	}
	for _, b := range sess.Blocks {
		h := &coins.Hold{ID: b.HoldID, UID: sess.UID, Op: coins.OpCall, Amount: b.Amount, DryRun: b.DryRun}
		if err := s.Coins.Extend(ctx, h, expireAt); err != nil {
			s.log().WarnContext(ctx, "calls: extend block hold failed", slog.String("uid_hash", httpx.UIDHash(sess.UID)), slog.Any("error", err))
		}
	}
}

// reserve holds block n of a session (nil when calls are not charged or
// COIN_BILLING_MODE is off).
func (s *Service) reserve(ctx context.Context, r JoinRequest, sessionID string, n int) (*Block, error) {
	amount := s.Config.BlockCoins()
	if s.Coins == nil || amount <= 0 {
		return nil, nil
	}
	h, err := s.Coins.Reserve(ctx, coins.ReserveRequest{
		UID: r.UID, Anonymous: r.Anonymous, Op: coins.OpCall, AppID: r.AppID,
		RequestID: "call/" + sessionID + "/" + strconv.Itoa(n),
		Max:       amount, TTL: s.Config.BlockDuration() + s.Config.HoldGrace,
	})
	if err != nil || h == nil {
		return nil, err
	}
	return &Block{HoldID: h.ID, Amount: h.Amount, DryRun: h.DryRun}, nil
}

// token is the LiveKit access token of sess: join its room, publish the
// microphone (and the camera when video), subscribe, no data channel.
func (s *Service) token(sess Session, name string, ttl time.Duration) (string, error) {
	yes, no := true, false
	sources := []livekit.TrackSource{livekit.TrackSource_MICROPHONE}
	if sess.Video {
		sources = append(sources, livekit.TrackSource_CAMERA)
	}
	grant := &auth.VideoGrant{
		RoomJoin: true, Room: sess.Room,
		CanPublish: &yes, CanSubscribe: &yes, CanPublishData: &no, CanUpdateOwnMetadata: &no,
	}
	grant.SetCanPublishSources(sources)
	at := auth.NewAccessToken(s.Config.APIKey, s.Config.APISecret).
		SetVideoGrant(grant).
		SetIdentity(sess.ID).
		SetName(cleanName(name)).
		SetValidFor(ttl).
		SetRoomConfig(&livekit.RoomConfiguration{
			MaxParticipants: uint32(s.Config.MaxParticipants),
			EmptyTimeout:    60,
		})
	return at.ToJWT()
}

// cleanName trims the display name (control characters out, MaxNameRunes).
func cleanName(n string) string {
	n = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, strings.TrimSpace(n))
	if utf8.RuneCountInString(n) > MaxNameRunes {
		n = string([]rune(n)[:MaxNameRunes])
	}
	return n
}

// Event is the part of a LiveKit webhook settlement needs.
type Event struct {
	// Kind: participant_joined | participant_left | room_finished (others are ignored).
	Kind     string
	Room     string
	Identity string
	// JoinedAt is the participant's join time (zero when unknown).
	JoinedAt time.Time
	// At is when the event happened (the leave / finish time).
	At time.Time
}

// Webhook event kinds.
const (
	KindJoined   = "participant_joined"
	KindLeft     = "participant_left"
	KindFinished = "room_finished"
)

// Handle applies one webhook event. Every path is idempotent: a repeated
// event changes nothing and commits the same charges again.
func (s *Service) Handle(ctx context.Context, e Event) error {
	switch e.Kind {
	case KindJoined:
		joined := e.JoinedAt
		if joined.IsZero() {
			joined = e.At
		}
		sess, err := s.Store.Update(ctx, e.Identity, func(cur *Session) error {
			if cur.Settled {
				return nil
			}
			if cur.JoinedAt.IsZero() && !joined.IsZero() {
				cur.JoinedAt = joined
			}
			cur.PaidUntil = s.paidUntil(*cur, cur.JoinedAt)
			return nil
		})
		if errors.Is(err, ErrNotFound) {
			return nil // not ours (or already gone)
		}
		if err != nil {
			return err
		}
		defer s.sweepRoom(ctx, e.Room)
		switch {
		case sess.Settled:
			// a token of a stay that already ended: not back into the room
			return s.remove(ctx, sess, removedSettled)
		case !sess.PaidUntil.IsZero():
			return s.Expire(ctx, sess.ID) // sets the timer (or ends it now)
		}
		return nil
	case KindLeft:
		defer s.sweepRoom(ctx, e.Room)
		return s.settle(ctx, e.Identity, e.JoinedAt, e.At)
	case KindFinished:
		open, err := s.Store.Open(ctx, e.Room)
		if err != nil {
			return err
		}
		for _, o := range open {
			if err := s.settle(ctx, o.ID, time.Time{}, e.At); err != nil {
				return err
			}
		}
	}
	return nil
}

// settle closes a session once (minutes and the charge of each block) and
// commits the charges (again, when a webhook repeats).
func (s *Service) settle(ctx context.Context, id string, joined, left time.Time) error {
	sess, err := s.Store.Update(ctx, id, func(cur *Session) error {
		if cur.Settled {
			return nil
		}
		start := cur.JoinedAt
		if !joined.IsZero() {
			start = joined
		}
		cur.Settled, cur.EndedAt = true, left
		if start.IsZero() || !left.After(start) {
			if start.IsZero() {
				s.log().WarnContext(ctx, EventNoJoinTime, slog.String("event", EventNoJoinTime), slog.String("uid_hash", httpx.UIDHash(cur.UID)))
			}
			cur.Minutes, cur.Charges = 0, map[string]int64{}
			return nil
		}
		if cur.JoinedAt.IsZero() {
			cur.JoinedAt = start
		}
		cur.Minutes = Minutes(left.Sub(start))
		var uncovered int64
		cur.Charges, uncovered = allocate(cur.Blocks, cur.Minutes*s.Config.PerMinute)
		if uncovered > 0 {
			s.log().WarnContext(ctx, EventUncovered, slog.String("event", EventUncovered), slog.String("uid_hash", httpx.UIDHash(cur.UID)),
				slog.Int64("minutes", cur.Minutes), slog.Int64("coins", uncovered))
		}
		return nil
	})
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	s.disarm(id)
	var charged int64
	for _, b := range sess.Blocks {
		h := &coins.Hold{ID: b.HoldID, UID: sess.UID, Op: coins.OpCall, AppID: sess.AppID, Amount: b.Amount, DryRun: b.DryRun}
		if s.Coins == nil {
			continue
		}
		c, err := s.Coins.CommitCoins(ctx, h, sess.Charges[b.HoldID])
		if err != nil {
			return fmt.Errorf("commit call block: %w", err)
		}
		charged += c.Coins
	}
	s.log().InfoContext(ctx, EventSettled, slog.String("event", EventSettled), slog.String("uid_hash", httpx.UIDHash(sess.UID)),
		slog.String("appId", sess.AppID), slog.Int64("minutes", sess.Minutes), slog.Int64("coins", charged))
	return nil
}

// Minutes is a stay rounded up to whole minutes (a started minute counts).
func Minutes(d time.Duration) int64 {
	if d <= 0 {
		return 0
	}
	return int64((d + time.Minute - 1) / time.Minute)
}

// allocate spreads total over the blocks in order, each up to its hold.
// uncovered is what no block could take (never charged).
func allocate(blocks []Block, total int64) (map[string]int64, int64) {
	out := make(map[string]int64, len(blocks))
	rest := max(0, total)
	for _, b := range blocks {
		n := min(b.Amount, rest)
		out[b.HoldID] = n
		rest -= n
	}
	return out, rest
}
