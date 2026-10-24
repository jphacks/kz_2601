package auth

import (
	"context"

	fbauth "firebase.google.com/go/v4/auth"
)

// UserDeleter removes the sign-in account itself (DELETE /v1/me, #119).
// Implementations must treat an already-deleted user as success.
type UserDeleter interface {
	DeleteUser(ctx context.Context, uid string) error
}

// DeleteUser implements UserDeleter with the Admin SDK. The service account
// needs Firebase Authentication Admin (roles/firebaseauth.admin); verifying
// ID tokens alone does not.
func (f *FirebaseVerifier) DeleteUser(ctx context.Context, uid string) error {
	if err := f.client.DeleteUser(ctx, uid); err != nil && !fbauth.IsUserNotFound(err) {
		return err
	}
	return nil
}
