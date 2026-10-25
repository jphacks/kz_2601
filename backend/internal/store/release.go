package store

import (
	"context"
	"encoding/json"
	"time"
)

// Release request statuses (#76). Only "received" is written by the API; an
// admin moves the others by hand in the Firestore console
// (architecture.md §3.3).
const (
	ReleaseReceived  = "received"
	ReleaseReviewing = "reviewing"
	ReleasePlanned   = "planned"
	ReleaseDeclined  = "declined"
)

// ReleaseOpen reports whether a request with this status still counts as
// pending, so a new request for the same app returns it instead of adding a
// duplicate. Only "declined" closes a request.
func ReleaseOpen(status string) bool { return status != ReleaseDeclined }

// ReleaseRequest is release_requests/{id}: "please ship this as a standalone
// app", with a frozen DSL snapshot of the version the owner asked about.
type ReleaseRequest struct {
	ID        string          `json:"id"`
	AppID     string          `json:"appId"`
	OwnerUID  string          `json:"ownerUid"`
	Name      string          `json:"name"`
	Version   int             `json:"dslVersion"`
	DSL       json.RawMessage `json:"dsl,omitempty"`
	Purpose   string          `json:"purpose"`
	Contact   string          `json:"contact"`
	Status    string          `json:"status"`
	CreatedAt time.Time       `json:"createdAt"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

// MaxReleaseRequestsPerUser bounds ListReleaseRequests.
const MaxReleaseRequestsPerUser = 200

// ReleaseRequests is the release-request part of Store.
type ReleaseRequests interface {
	// CreateReleaseRequest is idempotent per (OwnerUID, AppID): when an open
	// request already exists it is returned with created=false and nothing is
	// written. ID, Status and timestamps are filled in by the store.
	CreateReleaseRequest(ctx context.Context, rr ReleaseRequest) (out *ReleaseRequest, created bool, err error)
	// ListReleaseRequests returns uid's requests, newest first (DSL omitted).
	// appID == "" lists every app.
	ListReleaseRequests(ctx context.Context, uid, appID string) ([]ReleaseRequest, error)
}
