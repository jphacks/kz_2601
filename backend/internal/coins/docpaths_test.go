package coins

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

// #2797: the coin documents stay where they are.
func TestDocPaths(t *testing.T) {
	f, prefix := offlineStore(t)
	cases := []struct {
		name string
		ref  *firestore.DocumentRef
		want string
	}{
		{"wallet", f.walletRef("u1"), "users/u1/coins/wallet"},
		{"wallet of a uid with a dash", f.walletRef("u-2"), "users/u-2/coins/wallet"},
		{"hold", f.holdRef("h1"), "coinReservations/h1"},
		{"purchase", f.purchaseRef("t1"), "coin_transactions/t1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := strings.TrimPrefix(c.ref.Path, prefix); got != c.want {
				t.Errorf("path = %s, want %s", got, c.want)
			}
		})
	}
}
