package appstore

import (
	"regexp"
	"testing"
)

// UUIDv8 with the RFC 9562 variant.
var uuidV8Re2593 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-8[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestAccountToken2593(t *testing.T) {
	a := AccountToken("user-a")
	if again := AccountToken("user-a"); again != a {
		t.Errorf("same uid gave %q then %q", a, again)
	}
	// Pinned so iOS / Android (same SHA-256 recipe) stay in step.
	if want := "a9dc39fa-e99f-88ff-a373-5a8fe6617ccb"; a != want {
		t.Errorf("AccountToken(user-a) = %q, want %q", a, want)
	}
	if b := AccountToken("user-b"); b == a {
		t.Errorf("different uids gave the same token %q", a)
	}
	for _, uid := range []string{"user-a", "user-b", "", "日本語のuid"} {
		if tok := AccountToken(uid); !uuidV8Re2593.MatchString(tok) {
			t.Errorf("AccountToken(%q) = %q, not a UUIDv8", uid, tok)
		}
	}
}
