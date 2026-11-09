package runner_test

import (
	"fmt"
	"math/rand/v2"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/runner/notify"
	"github.com/jphacks/kz_2601/backend/internal/runner/quota"
)

// tokenSalt keeps this package's tokens apart from the other packages' that
// share the emulator in parallel: the token index (device_tokens) is global,
// so registering the same token elsewhere would drop our device.
var tokenSalt = rand.Uint64()

func hexToken(n int) string { return fmt.Sprintf("%016x%048x", tokenSalt, n) }

func (h *harness) registerDevice(uid, inst string, tok int, env string) {
	h.t.Helper()
	body := fmt.Sprintf(`{"token":%q,"environment":%q}`, hexToken(tok), env)
	if code, out := h.do(uid, http.MethodPut, "/v1/me/devices/"+inst+h.sfx, body, nil); code != http.StatusOK {
		h.t.Fatalf("register %s: %d %v", uid, code, out)
	}
}

func (h *harness) consent(uid, appID string) {
	h.t.Helper()
	if code, out := h.do(uid, http.MethodPut, "/v1/me/notify-consent/"+appID, `{"allowed":true}`, nil); code != http.StatusOK {
		h.t.Fatalf("consent %s: %d %v", uid, code, out)
	}
}

// #269 / #339: the runner reads the devices registered through appconfig and
// the stored consents; a sandbox token is not sent to production; a token
// APNs rejects as unregistered (410) is deleted.
func TestRunnerStoredDevicesAndConsent(t *testing.T) {
	stores(t, options{pro: fakePro{}, age: oldAccounts{}, storeDevices: true}, func(t *testing.T, h *harness) {
		owner, mom, dad, kid, aunt := "uid-d-owner"+h.sfx, "uid-d-mom"+h.sfx, "uid-d-dad"+h.sfx, "uid-d-kid"+h.sfx, "uid-d-aunt"+h.sfx
		h.pro[owner] = true
		id := "RUN-DEVICES" + h.sfx
		app := h.circle(id, owner, circleRules(t), mom, dad, kid, aunt)
		h.registerDevice(owner, "inst-owner", 1, "production")
		h.registerDevice(mom, "inst-mom", 2, "production")
		h.registerDevice(dad, "inst-dad", 3, "production")
		h.registerDevice(kid, "inst-kid", 4, "sandbox")
		h.registerDevice(aunt, "inst-aunt", 5, "production")
		h.consent(mom, id)
		h.consent(kid, id)
		h.sender.Results = map[string]notify.Result{hexToken(1): {Status: 410, Reason: "Unregistered", Unregistered: true}}

		h.post(dad, app+"/entities/messages/records", `{"data":{"body":"こんにちは"}}`)
		h.tick()

		var toks []string
		for _, p := range h.sender.Pushes() {
			toks = append(toks, p.Token)
		}
		slices.Sort(toks)
		if !slices.Equal(toks, []string{hexToken(1), hexToken(2)}) {
			t.Fatalf("pushes: %v", toks)
		}
		if ev := h.oneEvent(id); ev.Outcome != quota.OutcomeOK {
			t.Fatalf("event: %+v", ev)
		}
		out := h.logs.String()
		for _, want := range []string{`"no_consent":1`, `"no_device":1`, `"unregistered":1`} {
			if !strings.Contains(out, want) {
				t.Fatalf("%s not counted: %s", want, out)
			}
		}
		if strings.Contains(out, hexToken(1)) || strings.Contains(out, hexToken(2)) {
			t.Fatal("a device token was logged")
		}
		ds, err := h.st.ListDevices(t.Context(), owner)
		if err != nil || len(ds) != 0 {
			t.Fatalf("unregistered token kept: %+v %v", ds, err)
		}
		if ds, _ := h.st.ListDevices(t.Context(), mom); len(ds) != 1 {
			t.Fatalf("mom's device: %+v", ds)
		}
	})
}
