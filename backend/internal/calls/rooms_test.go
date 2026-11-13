package calls

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// NewLiveKitRooms always builds a client (a missing config is caught by
// Config, not here): it keeps the keys, and ws:// is called over http://.
func TestNewLiveKitRooms(t *testing.T) {
	r := NewLiveKitRooms(Config{URL: "wss://p.livekit.cloud/", APIKey: testKey, APISecret: testSecret})
	if r == nil || r.client == nil || r.key != testKey || r.secret != testSecret || r.requestTimeout != 10*time.Second {
		t.Fatalf("rooms = %+v", r)
	}

	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		gotPath = req.URL.Path
		w.Header().Set("Content-Type", "application/protobuf")
	}))
	defer srv.Close()
	// a ws:// URL with a trailing slash reaches the http server at the root
	wsURL := "ws://" + strings.TrimPrefix(srv.URL, "http://") + "/"
	r = NewLiveKitRooms(Config{URL: wsURL, APIKey: testKey, APISecret: testSecret})
	if err := r.RemoveParticipant(context.Background(), "app:a1", "s1"); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/twirp/livekit.RoomService/RemoveParticipant" {
		t.Fatalf("path %q", gotPath)
	}

	// no keys: the admin token cannot be made, so nothing is sent
	gotPath = ""
	r = NewLiveKitRooms(Config{URL: wsURL})
	if err := r.RemoveParticipant(context.Background(), "app:a1", "s1"); err == nil || gotPath != "" {
		t.Fatalf("no keys: err %v, request to %q", err, gotPath)
	}
}
