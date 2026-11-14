package loop

import (
	"context"
	"strings"
	"testing"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/option"
)

// offlineControl is a Firestore control whose client never connects (the *Ref
// helpers only build references).
func offlineControl(t *testing.T) (*Firestore, string) {
	t.Helper()
	c, err := firestore.NewClient(context.Background(), "demo-kz2601", option.WithoutAuthentication())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return NewFirestore(c), strings.TrimSuffix(c.Doc("a/b").Path, "a/b")
}

// #2797: the switches document and the monthly usage documents stay where they
// are.
func TestDocPaths(t *testing.T) {
	f, prefix := offlineControl(t)
	cases := []struct {
		name string
		ref  *firestore.DocumentRef
		want string
	}{
		{"switches", f.switchesRef(), "loop_control/switches"},
		{"usage", f.usageRef("2026-11"), "loop_control/usage-2026-11"},
		{"usage of another month", f.usageRef("2027-01"), "loop_control/usage-2027-01"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := strings.TrimPrefix(c.ref.Path, prefix); got != c.want {
				t.Errorf("path = %s, want %s", got, c.want)
			}
		})
	}
}
