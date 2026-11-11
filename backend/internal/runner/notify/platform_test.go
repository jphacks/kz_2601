package notify

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"testing"
)

// #918: each device goes to its platform's sender; the content is the same.
func TestPlatformSenderRoutes(t *testing.T) {
	ios, android := &FakeSender{}, &FakeSender{}
	s := &PlatformSender{IOS: ios, Android: android}
	d := &Dispatcher{Sender: s, Tokens: MemoryDevices{
		"a": {{Token: "i1", Platform: PlatformIOS}, {Token: "f1", Platform: PlatformAndroid}},
		"b": {{Token: "i2"}}, // stored before platforms: iOS
	}}
	out, err := d.Dispatch(context.Background(), Resolution{To: []string{"a", "b"}}, Content{Key: "k", AppID: "app1", AppName: "n", Title: "t", Body: "b"})
	if err != nil || !slices.Equal(out.Sent, []string{"a", "b"}) {
		t.Fatalf("got %+v %v", out, err)
	}
	tokens := func(f *FakeSender) (ts []string) {
		for _, p := range f.Pushes() {
			ts = append(ts, p.Token)
			if p.AppID != "app1" || p.Title != "t" || p.Body != "b" || p.AppName != "n" || p.CollapseID != CollapseID("k", 0) {
				t.Errorf("push %+v", p)
			}
		}
		return ts
	}
	if got := tokens(ios); !slices.Equal(got, []string{"i1", "i2"}) {
		t.Errorf("ios %v", got)
	}
	if got := tokens(android); !slices.Equal(got, []string{"f1"}) {
		t.Errorf("android %v", got)
	}
}

// Without an FCM key Android pushes are skipped (logged once), not retried,
// and iOS still goes out.
func TestPlatformSenderMissing(t *testing.T) {
	var buf bytes.Buffer
	ios := &FakeSender{}
	s := &PlatformSender{IOS: ios, Logger: slog.New(slog.NewJSONHandler(&buf, nil))}
	d := &Dispatcher{Sender: s, Tokens: MemoryDevices{
		"a": {{Token: "f1", Platform: PlatformAndroid}},
		"b": {{Token: "f2", Platform: PlatformAndroid}, {Token: "i2", Platform: PlatformIOS}},
	}}
	out, err := d.Dispatch(context.Background(), Resolution{To: []string{"a", "b"}}, Content{Key: "k"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(out.Sent, []string{"b"}) || out.Retryable || len(out.Unregistered) != 0 ||
		len(out.Skipped) != 1 || out.Skipped[0] != (Skip{UID: "a", Reason: ReasonSendFailed}) {
		t.Errorf("got %+v", out)
	}
	if n := strings.Count(buf.String(), "no sender for the platform"); n != 1 {
		t.Errorf("logged %d times: %s", n, buf.String())
	}
	if strings.Contains(buf.String(), "f1") || strings.Contains(buf.String(), "f2") {
		t.Errorf("token logged: %s", buf.String())
	}
	if _, err := s.Send(context.Background(), Push{Token: "x", Platform: "web"}); !errors.Is(err, ErrPlatformNotConfigured) {
		t.Errorf("unknown platform: %v", err)
	}
}
