package calls

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/livekit"
	"github.com/twitchtv/twirp"
)

// Rooms is the part of the LiveKit RoomService calls uses (nil = nobody is
// removed from a room by the server).
type Rooms interface {
	// RemoveParticipant takes identity out of room. Someone already gone
	// (or a room already closed) is not an error.
	RemoveParticipant(ctx context.Context, room, identity string) error
}

// LiveKitRooms is the RoomService of the LiveKit project (the same keys
// that sign the tokens).
type LiveKitRooms struct {
	client         livekit.RoomService
	key, secret    string
	requestTimeout time.Duration
}

// NewLiveKitRooms talks to cfg.URL (wss:// is called over https://).
func NewLiveKitRooms(cfg Config) *LiveKitRooms {
	base := cfg.URL
	switch {
	case strings.HasPrefix(base, "wss://"):
		base = "https://" + strings.TrimPrefix(base, "wss://")
	case strings.HasPrefix(base, "ws://"):
		base = "http://" + strings.TrimPrefix(base, "ws://")
	}
	return &LiveKitRooms{
		client: livekit.NewRoomServiceProtobufClient(strings.TrimRight(base, "/"), &http.Client{Timeout: 10 * time.Second}),
		key:    cfg.APIKey, secret: cfg.APISecret, requestTimeout: 10 * time.Second,
	}
}

// RemoveParticipant implements Rooms with a short-lived admin token of that room only.
func (r *LiveKitRooms) RemoveParticipant(ctx context.Context, room, identity string) error {
	tok, err := auth.NewAccessToken(r.key, r.secret).
		SetVideoGrant(&auth.VideoGrant{RoomAdmin: true, Room: room}).
		SetValidFor(time.Minute).
		ToJWT()
	if err != nil {
		return err
	}
	h := http.Header{}
	h.Set("Authorization", "Bearer "+tok)
	ctx, cancel := context.WithTimeout(ctx, r.requestTimeout)
	defer cancel()
	if ctx, err = twirp.WithHTTPRequestHeaders(ctx, h); err != nil {
		return err
	}
	_, err = r.client.RemoveParticipant(ctx, &livekit.RoomParticipantIdentity{Room: room, Identity: identity})
	var te twirp.Error
	if errors.As(err, &te) && te.Code() == twirp.NotFound {
		return nil
	}
	return err
}
