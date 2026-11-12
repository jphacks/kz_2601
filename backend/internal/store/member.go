package store

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Member is apps_public/{appId}/members/{uid}: someone who joined an app with
// shared-scope entities through a share link (#254). The app's creator
// (app_owners) is a member without a document.
type Member struct {
	UID         string    `json:"uid"`
	DisplayName string    `json:"displayName,omitempty"`
	Via         string    `json:"via"`                 // MemberViaShare | MemberViaLegacy
	InvitedBy   string    `json:"invitedBy,omitempty"` // uid that issued the share link
	JoinedAt    time.Time `json:"joinedAt"`
	// Admin (#272): an admin of the app's shared data. The creator is always
	// one (without a document); an admin makes other members admins.
	Admin bool `json:"admin,omitempty"`
}

// Member.Via values.
const (
	MemberViaShare  = "share"  // joined with POST /shares/{token}/import
	MemberViaLegacy = "legacy" // had the app before membership existed (migration)
)

// MaxMembers bounds one app's member list (family / club sized).
const MaxMembers = 100

// MemberGetter is the one-document read of a membership.
type MemberGetter interface {
	GetMember(ctx context.Context, appID, uid string) (*Member, error)
}

// IsMember reports whether uid has a members/{uid} document under appID,
// reading that one document instead of the whole list (#2297). The creator
// has no document: check the owner first. A uid that cannot be a document ID
// (empty, with "/", "." / "..", __x__, too long) is not a member and is not read.
func IsMember(ctx context.Context, g MemberGetter, appID, uid string) (bool, error) {
	if !memberDocID(uid) {
		return false, nil
	}
	_, err := g.GetMember(ctx, appID, uid)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

// memberDocID: uid is a valid Firestore document ID.
func memberDocID(uid string) bool {
	return uid != "" && len(uid) <= 1500 && uid != "." && uid != ".." && !strings.Contains(uid, "/") &&
		!(len(uid) >= 4 && strings.HasPrefix(uid, "__") && strings.HasSuffix(uid, "__"))
}

// Membership is the participant list of shared apps.
type Membership interface {
	// AddMember adds m under appID. An existing member keeps JoinedAt, Via and
	// InvitedBy; only a non-empty DisplayName is updated. created reports a new member.
	AddMember(ctx context.Context, appID string, m Member) (created bool, err error)
	// GetMember returns ErrNotFound when uid has not joined appID.
	GetMember(ctx context.Context, appID, uid string) (*Member, error)
	// ListMembers returns up to limit members, oldest first.
	ListMembers(ctx context.Context, appID string, limit int) ([]Member, error)
	// RemoveMember returns ErrNotFound when uid is not a member.
	RemoveMember(ctx context.Context, appID, uid string) error
	// SetMemberAdmin sets Member.Admin (#272). ErrNotFound when uid is not a member.
	SetMemberAdmin(ctx context.Context, appID, uid string, admin bool) error
}
