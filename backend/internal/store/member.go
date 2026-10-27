package store

import (
	"context"
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
}

// Member.Via values.
const (
	MemberViaShare  = "share"  // joined with POST /shares/{token}/import
	MemberViaLegacy = "legacy" // had the app before membership existed (migration)
)

// MaxMembers bounds one app's member list (family / club sized).
const MaxMembers = 100

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
}
