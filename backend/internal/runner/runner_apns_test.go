package runner_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/runner/notify/apns"
	"github.com/jphacks/kz_2601/backend/internal/runner/quota"
)

// fakeAPNs answers 400 BadDeviceToken for bad and 200 otherwise, recording
// the device tokens it was sent.
func fakeAPNs(t *testing.T, bad string) (*apns.Client, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var got []string
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := strings.TrimPrefix(r.URL.Path, "/3/device/")
		mu.Lock()
		got = append(got, tok)
		mu.Unlock()
		if tok == bad {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"reason":"BadDeviceToken"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader) // no real .p8
	if err != nil {
		t.Fatal(err)
	}
	c, err := apns.New(apns.Config{KeyID: "KEY123", TeamID: "TEAM456", Topic: "jp.example.app", Key: key,
		Host: srv.URL, HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	return c, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), got...)
	}
}

// #269: the runner sends through the APNs client to the recipients' stored
// production tokens and deletes a token APNs calls BadDeviceToken.
func TestRunnerSendsThroughAPNs(t *testing.T) {
	sender, sent := fakeAPNs(t, hexToken(11))
	stores(t, options{pro: fakePro{}, age: oldAccounts{}, storeDevices: true, sender: sender}, func(t *testing.T, h *harness) {
		owner, mom, dad := "uid-a-owner"+h.sfx, "uid-a-mom"+h.sfx, "uid-a-dad"+h.sfx
		h.pro[owner] = true
		id := "RUN-APNS" + h.sfx
		app := h.circle(id, owner, circleRules(t), mom, dad)
		h.registerDevice(owner, "inst-owner", 11, "production")
		h.registerDevice(mom, "inst-mom", 12, "production")
		h.registerDevice(mom, "inst-mom-dev", 13, "sandbox")
		h.consent(mom, id)
		before := len(sent())

		h.post(dad, app+"/entities/messages/records", `{"data":{"body":"こんにちは"}}`)
		h.tick()

		got := sent()[before:]
		slices.Sort(got)
		if !slices.Equal(got, []string{hexToken(11), hexToken(12)}) {
			t.Fatalf("sent to %v", got)
		}
		if ev := h.oneEvent(id); ev.Outcome != quota.OutcomeOK {
			t.Fatalf("event: %+v", ev)
		}
		if ds, err := h.st.ListDevices(t.Context(), owner); err != nil || len(ds) != 0 {
			t.Fatalf("BadDeviceToken kept: %+v %v", ds, err)
		}
		if ds, _ := h.st.ListDevices(t.Context(), mom); len(ds) != 2 {
			t.Fatalf("mom's devices: %+v", ds)
		}
		if strings.Contains(h.logs.String(), hexToken(12)) {
			t.Fatal("a device token was logged")
		}
	})
}
