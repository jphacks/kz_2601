package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// ErrForbidden is returned when uid does not own the document it tries to change.
var ErrForbidden = errors.New("forbidden")

// Share is shares/{token}: a frozen DSL snapshot that anyone signed in can
// import as their own app (#49). Records are never part of a share.
type Share struct {
	Token     string          `json:"token"`
	AppID     string          `json:"appId"`
	OwnerUID  string          `json:"ownerUid"`
	Name      string          `json:"name"`
	Version   int             `json:"dslVersion"`
	DSL       json.RawMessage `json:"dsl"`
	Revoked   bool            `json:"revoked"`
	CreatedAt time.Time       `json:"createdAt"`
}

// CatalogApp is public_apps/{appId}: a listing in the "探す" catalog (#50).
// Not to be confused with apps_public/{appId} (#93), which only exists so that
// shared/public entities have an app-wide record collection.
type CatalogApp struct {
	ID          string          `json:"id"` // the owner's appId
	OwnerUID    string          `json:"ownerUid"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Tags        []string        `json:"tags"`
	AuthorName  string          `json:"authorName"`
	Components  []string        `json:"components"` // distinct component types, for card icons
	Version     int             `json:"dslVersion"`
	DSL         json.RawMessage `json:"dsl,omitempty"`
	Installs    int             `json:"installs"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
	// ModerationState: unpublished by the operator (#321). Kept when the owner publishes again.
	ModerationState
}

// MaxCatalogApps bounds the in-memory search (architecture.md §3.2).
const MaxCatalogApps = 500

// Sharing is the share-link and catalog part of Store.
type Sharing interface {
	CreateShare(ctx context.Context, s Share) error
	GetShare(ctx context.Context, token string) (*Share, error)
	RevokeShare(ctx context.Context, token string) error

	// PutCatalogApp creates or updates public_apps/{c.ID}. ErrForbidden when
	// another uid owns it. Installs and CreatedAt of an existing listing are kept.
	PutCatalogApp(ctx context.Context, uid string, c CatalogApp) (*CatalogApp, error)
	GetCatalogApp(ctx context.Context, id string) (*CatalogApp, error)
	// DeleteCatalogApp: ErrNotFound when absent, ErrForbidden when not uid's.
	DeleteCatalogApp(ctx context.Context, uid, id string) error
	// ListCatalogApps returns up to limit listings, most installed first (DSL omitted).
	ListCatalogApps(ctx context.Context, limit int) ([]CatalogApp, error)
	IncrementInstalls(ctx context.Context, id string) error
}
