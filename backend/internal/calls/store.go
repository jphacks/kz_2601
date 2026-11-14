package calls

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrNotFound: no such session.
var ErrNotFound = errors.New("call session not found")

// Block is one coin hold of a session (a block of minutes).
type Block struct {
	HoldID string `firestore:"holdId"`
	Amount int64  `firestore:"amount"`
	// DryRun: COIN_BILLING_MODE=log (nothing was taken; settling only logs).
	DryRun bool `firestore:"dryRun,omitempty"`
}

// Session is callSessions/{id}: one token holder's stay in a room. The id
// is the LiveKit participant identity.
type Session struct {
	ID        string    `firestore:"-"`
	UID       string    `firestore:"uid"`
	AppID     string    `firestore:"appId"`
	Room      string    `firestore:"room"`
	Video     bool      `firestore:"video"`
	Blocks    []Block   `firestore:"blocks"`
	CreatedAt time.Time `firestore:"createdAt"`
	JoinedAt  time.Time `firestore:"joinedAt,omitempty"`
	EndedAt   time.Time `firestore:"endedAt,omitempty"`
	// PaidUntil is when the holds run out (from the join; zero = not
	// bounded or not joined yet). The server takes the participant out then.
	PaidUntil time.Time `firestore:"paidUntil,omitempty"`
	// Settled is set once with Minutes and Charges (hold id -> coins); a
	// repeated webhook commits the same charges again (idempotent).
	Settled bool             `firestore:"settled"`
	Minutes int64            `firestore:"minutes"`
	Charges map[string]int64 `firestore:"charges,omitempty"`
	// DeleteAt is for a Firestore TTL policy (kept a while for support).
	DeleteAt time.Time `firestore:"deleteAt"`
}

// Retention is how long a session document is kept (DeleteAt).
const Retention = 30 * 24 * time.Hour

// Store persists sessions.
type Store interface {
	Create(ctx context.Context, s Session) error
	Get(ctx context.Context, id string) (Session, error)
	// Update runs fn on the session in one transaction and writes it back
	// (fn may run again on contention). ErrNotFound when absent.
	Update(ctx context.Context, id string, fn func(*Session) error) (Session, error)
	// Open lists the unsettled sessions of a room (room_finished).
	Open(ctx context.Context, room string) ([]Session, error)
}

// Memory is the Store for tests and local runs.
type Memory struct {
	mu sync.Mutex
	m  map[string]Session
}

// NewMemory returns an empty store.
func NewMemory() *Memory { return &Memory{m: map[string]Session{}} }

func clone(s Session) Session {
	s.Blocks = slices.Clone(s.Blocks)
	if s.Charges != nil {
		c := make(map[string]int64, len(s.Charges))
		for k, v := range s.Charges {
			c[k] = v
		}
		s.Charges = c
	}
	return s
}

// Create implements Store.
func (m *Memory) Create(_ context.Context, s Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.m[s.ID]; ok {
		return errors.New("call session exists")
	}
	m.m[s.ID] = clone(s)
	return nil
}

// Get implements Store.
func (m *Memory) Get(_ context.Context, id string) (Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.m[id]
	if !ok {
		return Session{}, ErrNotFound
	}
	return clone(s), nil
}

// Update implements Store.
func (m *Memory) Update(_ context.Context, id string, fn func(*Session) error) (Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.m[id]
	if !ok {
		return Session{}, ErrNotFound
	}
	s = clone(s)
	if err := fn(&s); err != nil {
		return Session{}, err
	}
	m.m[id] = clone(s)
	return s, nil
}

// Open implements Store.
func (m *Memory) Open(_ context.Context, room string) ([]Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Session
	for _, s := range m.m {
		if s.Room == room && !s.Settled {
			out = append(out, clone(s))
		}
	}
	slices.SortFunc(out, func(a, b Session) int { return a.CreatedAt.Compare(b.CreatedAt) })
	return out, nil
}

const sessionsCollection = "callSessions"

// Firestore is the production Store (same database as appconfig).
type Firestore struct{ c *firestore.Client }

// NewFirestore uses client (honours FIRESTORE_EMULATOR_HOST).
func NewFirestore(c *firestore.Client) *Firestore { return &Firestore{c: c} }

func (f *Firestore) ref(id string) *firestore.DocumentRef {
	return f.c.Collection(sessionsCollection).Doc(id)
}

// Create implements Store.
func (f *Firestore) Create(ctx context.Context, s Session) error {
	_, err := f.ref(s.ID).Create(ctx, s)
	return err
}

// Get implements Store.
func (f *Firestore) Get(ctx context.Context, id string) (Session, error) {
	snap, err := f.ref(id).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, err
	}
	var s Session
	if err := snap.DataTo(&s); err != nil {
		return Session{}, err
	}
	s.ID = id
	return s, nil
}

// Update implements Store.
func (f *Firestore) Update(ctx context.Context, id string, fn func(*Session) error) (Session, error) {
	var out Session
	err := f.c.RunTransaction(ctx, func(_ context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(f.ref(id))
		if status.Code(err) == codes.NotFound {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		var s Session
		if err := snap.DataTo(&s); err != nil {
			return err
		}
		s.ID = id
		if err := fn(&s); err != nil {
			return err
		}
		out = s
		return tx.Set(f.ref(id), s)
	})
	return out, err
}

// Open implements Store (two equality filters: no composite index).
func (f *Firestore) Open(ctx context.Context, room string) ([]Session, error) {
	it := f.c.Collection(sessionsCollection).Where("room", "==", room).Where("settled", "==", false).Limit(200).Documents(ctx)
	defer it.Stop()
	var out []Session
	for {
		snap, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		var s Session
		if err := snap.DataTo(&s); err != nil {
			return nil, err
		}
		s.ID = snap.Ref.ID
		out = append(out, s)
	}
}
