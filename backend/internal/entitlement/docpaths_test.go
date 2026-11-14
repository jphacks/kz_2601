package entitlement

import (
	"context"
	"strings"
	"testing"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/option"
)

// offlineStore is a Firestore store whose client never connects (the *Ref
// helpers only build references).
func offlineStore(t *testing.T) (*Firestore, string) {
	t.Helper()
	c, err := firestore.NewClient(context.Background(), "demo-kz2601", option.WithoutAuthentication())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return NewFirestore(c), strings.TrimSuffix(c.Doc("a/b").Path, "a/b")
}

// #2797: the Pro document and the binding documents stay where they are, and a
// Play key goes to the Play collection with the prefix cut.
func TestDocPaths(t *testing.T) {
	f, prefix := offlineStore(t)
	cases := []struct {
		name string
		ref  *firestore.DocumentRef
		want string
	}{
		{"pro", f.proRef("u1"), "users/u1/entitlements/pro"},
		{"pro of a uid with a dash", f.proRef("u-2"), "users/u-2/entitlements/pro"},
		{"app store binding", f.bindingRef("2000000012345678"), "appstore_transactions/2000000012345678"},
		{"play binding", f.bindingRef(playKeyPrefix + "abc123"), "googleplay_purchases/abc123"},
		{"play key of a token", f.bindingRef(PlayKey("tok")), "googleplay_purchases/" + strings.TrimPrefix(PlayKey("tok"), playKeyPrefix)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := strings.TrimPrefix(c.ref.Path, prefix); got != c.want {
				t.Errorf("path = %s, want %s", got, c.want)
			}
		})
	}
}
