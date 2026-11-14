package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// appData is users/{uid}/apps/{appId} with its versions in the period.
type appData struct {
	ID        string
	Name      string
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
	DSL       json.RawMessage // current version
	Versions  []versionData
	Records   int // records in the app's entities (capped); -1 = not counted
}

// versionData is users/{uid}/apps/{appId}/versions/{n}.
type versionData struct {
	N            int
	CreatedAt    time.Time
	Kind         string // ai | manual | restore | import | system
	Mode         string // full | patch
	Transcript   string // ai: what the user asked (user content)
	Summary      string // ai: the model's summary of the change
	RestoredFrom int
	DSL          json.RawMessage
}

// feedbackData is feedback/{id} without the screenshot / DSL attachments.
type feedbackData struct {
	ID        string
	Kind      string
	AppID     string
	Message   string // user content (untrusted, #366)
	CreatedAt time.Time
}

// reader is everything the report reads from Firestore. Read calls only:
// there is no method that writes, and fsReader (the only implementation
// outside tests) keeps the client unexported so nothing else reaches it.
type reader interface {
	Apps(ctx context.Context, uid string, since time.Time) ([]appData, error)
	Versions(ctx context.Context, uid, appID string, since time.Time) ([]versionData, error)
	RecordCount(ctx context.Context, uid, appID string, entities []string, limit int) (int, error)
	Feedback(ctx context.Context, uid string, since time.Time) ([]feedbackData, error)
}

// fsReader reads with Get / Documents only (main_test.go checks the source
// of this package for write calls).
type fsReader struct{ c *firestore.Client }

func (r fsReader) app(uid string) *firestore.CollectionRef {
	return r.c.Collection("users").Doc(uid).Collection("apps")
}

// Apps lists uid's apps updated at or after since.
func (r fsReader) Apps(ctx context.Context, uid string, since time.Time) ([]appData, error) {
	it := r.app(uid).Where("updatedAt", ">=", since).
		Select("name", "status", "createdAt", "updatedAt", "dslJson").Documents(ctx)
	defer it.Stop()
	var out []appData
	for {
		snap, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("list apps: %w", err)
		}
		var d struct {
			Name      string    `firestore:"name"`
			Status    string    `firestore:"status"`
			CreatedAt time.Time `firestore:"createdAt"`
			UpdatedAt time.Time `firestore:"updatedAt"`
			DSLJSON   string    `firestore:"dslJson"`
		}
		if err := snap.DataTo(&d); err != nil {
			return nil, fmt.Errorf("decode app: %w", err)
		}
		out = append(out, appData{ID: snap.Ref.ID, Name: d.Name, Status: d.Status, CreatedAt: d.CreatedAt,
			UpdatedAt: d.UpdatedAt, DSL: json.RawMessage(d.DSLJSON), Records: -1})
	}
}

// Versions lists the versions created at or after since, oldest first.
func (r fsReader) Versions(ctx context.Context, uid, appID string, since time.Time) ([]versionData, error) {
	it := r.app(uid).Doc(appID).Collection("versions").Where("createdAt", ">=", since).
		OrderBy("createdAt", firestore.Asc).Select("version", "dslJson", "source", "createdAt").Documents(ctx)
	defer it.Stop()
	var out []versionData
	for {
		snap, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("list versions: %w", err)
		}
		var d struct {
			Version   int64          `firestore:"version"`
			DSLJSON   string         `firestore:"dslJson"`
			Source    map[string]any `firestore:"source"`
			CreatedAt time.Time      `firestore:"createdAt"`
		}
		if err := snap.DataTo(&d); err != nil {
			return nil, fmt.Errorf("decode version: %w", err)
		}
		v := versionData{N: int(d.Version), CreatedAt: d.CreatedAt, DSL: json.RawMessage(d.DSLJSON)}
		v.Kind, _ = d.Source["kind"].(string)
		v.Mode, _ = d.Source["mode"].(string)
		v.Transcript, _ = d.Source["transcript"].(string)
		v.Summary, _ = d.Source["summary"].(string)
		if n, ok := d.Source["restoredFrom"].(int64); ok {
			v.RestoredFrom = int(n)
		}
		out = append(out, v)
	}
}

// RecordCount counts record documents (ids only, no contents) up to limit.
func (r fsReader) RecordCount(ctx context.Context, uid, appID string, entities []string, limit int) (int, error) {
	n := 0
	for _, e := range entities {
		if n >= limit {
			break
		}
		it := r.app(uid).Doc(appID).Collection("entities").Doc(e).Collection("records").
			Select().Limit(limit - n).Documents(ctx)
		for {
			_, err := it.Next()
			if errors.Is(err, iterator.Done) {
				break
			}
			if err != nil {
				it.Stop()
				return 0, fmt.Errorf("count records: %w", err)
			}
			n++
		}
		it.Stop()
	}
	return n, nil
}

// Feedback lists uid's feedback created at or after since (no screenshot or DSL).
func (r fsReader) Feedback(ctx context.Context, uid string, since time.Time) ([]feedbackData, error) {
	it := r.c.Collection("feedback").Where("uid", "==", uid).
		Select("kind", "appId", "message", "createdAt").Documents(ctx)
	defer it.Stop()
	var out []feedbackData
	for {
		snap, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return out, nil
		}
		if status.Code(err) == codes.FailedPrecondition { // a missing index must not stop the report
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("list feedback: %w", err)
		}
		var d struct {
			Kind      string    `firestore:"kind"`
			AppID     string    `firestore:"appId"`
			Message   string    `firestore:"message"`
			CreatedAt time.Time `firestore:"createdAt"`
		}
		if err := snap.DataTo(&d); err != nil {
			return nil, fmt.Errorf("decode feedback: %w", err)
		}
		if d.CreatedAt.Before(since) {
			continue
		}
		out = append(out, feedbackData{ID: snap.Ref.ID, Kind: d.Kind, AppID: d.AppID, Message: d.Message, CreatedAt: d.CreatedAt})
	}
}
