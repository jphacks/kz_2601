package entitlement

import (
	"context"
	"errors"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// notificationsCollection holds NotificationRecord by notificationUUID (#315).
const notificationsCollection = "appstore_notifications"

// NotificationSeen implements NotifyStore.
func (f *Firestore) NotificationSeen(ctx context.Context, uuid string) (bool, error) {
	_, err := f.c.Collection(notificationsCollection).Doc(uuid).Get(ctx)
	if notFound(err) {
		return false, nil
	}
	return err == nil, err
}

// RecordNotification implements NotifyStore (create-only: the first record wins).
func (f *Firestore) RecordNotification(ctx context.Context, rec NotificationRecord) error {
	_, err := f.c.Collection(notificationsCollection).Doc(rec.UUID).Create(ctx, map[string]any{
		"notificationType":      rec.Type,
		"subtype":               rec.Subtype,
		"environment":           rec.Environment,
		"originalTransactionId": rec.OriginalTransactionID,
		"outcome":               rec.Outcome,
		"signedAt":              rec.SignedAt,
		"processedAt":           rec.ProcessedAt,
		"expireAt":              rec.ExpireAt,
	})
	if status.Code(err) == codes.AlreadyExists {
		return nil
	}
	return err
}

// Bindings implements NotifyStore.
func (f *Firestore) Bindings(ctx context.Context, after string, limit int) ([]Binding, error) {
	q := f.c.Collection(bindingsCollection).OrderBy(firestore.DocumentID, firestore.Asc).Limit(limit)
	if after != "" {
		q = q.StartAfter(after)
	}
	it := q.Documents(ctx)
	defer it.Stop()
	var out []Binding
	for {
		s, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		uid, _ := s.Data()["uid"].(string)
		out = append(out, Binding{OriginalTransactionID: s.Ref.ID, UID: uid})
	}
}
