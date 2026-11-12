package extapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const sbMeter = "C271111EC0AB"

// 偽の値(試験だけ)。
var (
	sbTestToken  = strings.Repeat("0123456789abcdef", 6)
	sbTestSecret = strings.Repeat("fedcba9876543210", 2)
)

var sbNow = time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)

// sbFake is a fake SwitchBot API v1.1: it checks the signature and answers.
type sbFake struct {
	status atomic.Int64 // HTTP status
	code   atomic.Int64 // the answer's statusCode
	calls  atomic.Int64
	path   atomic.Value // string
	body   atomic.Value // string: the status body
}

func newSBFake(t *testing.T) (*sbFake, *httptest.Server) {
	t.Helper()
	f := &sbFake{}
	f.status.Store(200)
	f.code.Store(100)
	f.body.Store(`{"deviceId":"C271111EC0AB","deviceType":"Meter","hubDeviceId":"E4E4E4E4E4E4","temperature":24.46,"humidity":52,"battery":87,"version":"V2.6"}`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		f.path.Store(r.URL.Path)
		tok, ts, nonce, sign := r.Header.Get("Authorization"), r.Header.Get("t"), r.Header.Get("nonce"), r.Header.Get("sign")
		m := hmac.New(sha256.New, []byte(sbTestSecret))
		m.Write([]byte(tok + ts + nonce))
		if r.Method != http.MethodGet || r.URL.RawQuery != "" || tok != sbTestToken || ts != strconv.FormatInt(sbNow.UnixMilli(), 10) || len(nonce) != 32 ||
			sign != base64.StdEncoding.EncodeToString(m.Sum(nil)) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"Unauthorized"}`))
			return
		}
		if s := f.status.Load(); s != 200 {
			w.WriteHeader(int(s))
			return
		}
		body := `{}`
		switch {
		case r.URL.Path == "/v1.1/devices":
			body = `{"deviceList":[
				{"deviceId":"C271111EC0AB","deviceName":"寝室の温湿度計","deviceType":"Meter","enableCloudService":true,"hubDeviceId":"E4E4E4E4E4E4"},
				{"deviceId":"6055F92FCFD2","deviceName":"リビングのプラグ","deviceType":"Plug Mini (JP)","enableCloudService":true,"hubDeviceId":""},
				{"deviceId":"bad/id","deviceName":"x","deviceType":"Bot"}],
				"infraredRemoteList":[{"deviceId":"01-202008110034-12345678","deviceName":"エアコン","remoteType":"Air Conditioner","hubDeviceId":"E4E4E4E4E4E4"}]}`
		case strings.HasSuffix(r.URL.Path, "/status"):
			body = f.body.Load().(string)
		}
		_, _ = w.Write([]byte(`{"statusCode":` + jsonInt(f.code.Load()) + `,"message":"success","body":` + body + `}`))
	}))
	t.Cleanup(srv.Close)
	return f, srv
}

func jsonInt(n int64) string { b, _ := json.Marshal(n); return string(b) }

func sbKeys(k switchBotKey, err error) switchBotKeys {
	return func(context.Context, Input) (switchBotKey, error) { return k, err }
}

func newSBOps(t *testing.T, srv *httptest.Server) map[string]Operation {
	t.Helper()
	ops, err := newSwitchBot(sbKeys(switchBotKey{Token: sbTestToken, Secret: sbTestSecret}, nil), srv.URL, srv.Client(), func() time.Time { return sbNow })
	if err != nil {
		t.Fatal(err)
	}
	return ops
}

func TestSwitchBotDevices(t *testing.T) {
	f, srv := newSBFake(t)
	out, err := newSBOps(t, srv)["devices"].Do(t.Context(), Input{})
	if err != nil {
		t.Fatal(err)
	}
	if f.path.Load() != "/v1.1/devices" {
		t.Fatalf("path %v", f.path.Load())
	}
	// physical devices only; a bad id is dropped; IR remotes are not listed (commands are a follow-up)
	want := []Row{
		{Title: "寝室の温湿度計", Subtitle: "Meter", Value: "C271111EC0AB"},
		{Title: "リビングのプラグ", Subtitle: "Plug Mini (JP)", Value: "6055F92FCFD2"},
	}
	if len(out.Rows) != len(want) || out.Rows[0] != want[0] || out.Rows[1] != want[1] {
		t.Fatalf("rows %+v", out.Rows)
	}
}

func TestSwitchBotStatus(t *testing.T) {
	f, srv := newSBFake(t)
	ops := newSBOps(t, srv)
	out, err := ops["status"].Do(t.Context(), Input{Args: map[string]string{"deviceId": " " + sbMeter + " "}})
	if err != nil {
		t.Fatal(err)
	}
	if f.path.Load() != "/v1.1/devices/"+sbMeter+"/status" {
		t.Fatalf("path %v", f.path.Load())
	}
	if out.Item == nil || *out.Item != (Row{Title: "24.5°C・湿度 52%", Subtitle: "Meter", Value: "24.5°C"}) {
		t.Fatalf("item %+v", out.Item)
	}
	wantCells := [][]string{{"temperature", "24.5°C"}, {"humidity", "52%"}, {"battery", "87%"}}
	if len(out.Rows) != 3 || len(out.Cells) != 3 {
		t.Fatalf("rows %+v cells %v", out.Rows, out.Cells)
	}
	for i, c := range wantCells {
		if out.Cells[i][0] != c[0] || out.Cells[i][1] != c[1] || out.Rows[i].Value != c[1] {
			t.Fatalf("%d: rows %+v cells %v", i, out.Rows, out.Cells)
		}
	}

	// a plug: on / off
	f.body.Store(`{"deviceId":"6055F92FCFD2","deviceType":"Plug Mini (JP)","power":"on","voltage":100.2,"weight":3.1,"electricCurrent":0.2}`)
	out, err = ops["status"].Do(t.Context(), Input{Args: map[string]string{"deviceId": "6055F92FCFD2"}})
	if err != nil || out.Item.Title != "電源 オン" || out.Item.Value != "オン" || len(out.Rows) != 1 {
		t.Fatalf("%+v %v", out, err)
	}

	// readings that tell whether someone is home are never returned
	for _, b := range []string{
		`{"deviceId":"A1","deviceType":"Motion Sensor","moveDetected":true,"brightness":"bright"}`,
		`{"deviceId":"A1","deviceType":"Contact Sensor","openState":"open","moveDetected":false}`,
		`{"deviceId":"A1","deviceType":"Smart Lock","lockState":"unlocked","doorState":"opened"}`,
	} {
		f.body.Store(b)
		out, err := ops["status"].Do(t.Context(), Input{Args: map[string]string{"deviceId": "A1"}})
		if Reason(err) != ReasonBadRequest {
			t.Fatalf("%s: %+v %v", b, out, err)
		}
	}
	// battery alone is returned (the lock's battery)
	f.body.Store(`{"deviceId":"A1","deviceType":"Smart Lock","lockState":"unlocked","battery":60}`)
	out, err = ops["status"].Do(t.Context(), Input{Args: map[string]string{"deviceId": "A1"}})
	if err != nil || len(out.Rows) != 1 || out.Rows[0].Title != "電池" || strings.Contains(out.Item.Title+out.Item.Value, "unlock") {
		t.Fatalf("%+v %v", out, err)
	}
}

func TestSwitchBotErrors(t *testing.T) {
	f, srv := newSBFake(t)
	ops := newSBOps(t, srv)
	args := map[string]string{"deviceId": sbMeter}
	for code, want := range map[int64]string{
		152: ReasonBadRequest, // no such device
		151: ReasonBadRequest,
		160: ReasonBadRequest,
		161: ReasonUpstream, // offline
		171: ReasonUpstream,
		190: ReasonUpstream,
		999: ReasonUpstream,
	} {
		f.code.Store(code)
		if _, err := ops["status"].Do(t.Context(), Input{Args: args}); Reason(err) != want {
			t.Fatalf("statusCode %d: %s (%v)", code, Reason(err), err)
		}
	}
	f.code.Store(100)
	for status, want := range map[int64]string{
		401: ReasonNotConnected, // the token was reset
		403: ReasonNotConnected,
		429: ReasonUpstream,
		500: ReasonUpstream,
	} {
		f.status.Store(status)
		_, err := ops["status"].Do(t.Context(), Input{Args: args})
		if Reason(err) != want {
			t.Fatalf("status %d: %s (%v)", status, Reason(err), err)
		}
		if status == 429 && !upstreamLimited(err) {
			t.Fatal("429 does not pause the provider")
		}
	}
	f.status.Store(200)

	// a wrong secret: SwitchBot refuses the signature → not connected
	bad, _ := newSwitchBot(sbKeys(switchBotKey{Token: sbTestToken, Secret: "00000000000000000000"}, nil), srv.URL, srv.Client(), func() time.Time { return sbNow })
	if _, err := bad["devices"].Do(t.Context(), Input{}); Reason(err) != ReasonNotConnected {
		t.Fatalf("wrong secret: %v", err)
	}

	// no key, or a stored value that is not a key: not connected, no call
	n := f.calls.Load()
	for _, k := range []switchBotKeys{
		sbKeys(switchBotKey{}, ErrNotConnected),
		sbKeys(switchBotKey{Token: "short", Secret: sbTestSecret}, nil),
		sbKeys(switchBotKey{Token: sbTestToken, Secret: "has space here"}, nil),
		sbKeys(switchBotKey{Token: sbTestToken + "\r\nX: y", Secret: sbTestSecret}, nil),
	} {
		o, _ := newSwitchBot(k, srv.URL, srv.Client(), nil)
		if _, err := o["devices"].Do(t.Context(), Input{}); Reason(err) != ReasonNotConnected {
			t.Fatalf("%v", err)
		}
	}
	// a bad device id never reaches the API
	for _, id := range []string{"", "../devices", "a/b", "C2711?x=1", strings.Repeat("a", 65), "-abc"} {
		if _, err := ops["status"].Do(t.Context(), Input{Args: map[string]string{"deviceId": id}}); Reason(err) != ReasonBadRequest {
			t.Fatalf("%q: %v", id, err)
		}
	}
	if f.calls.Load() != n {
		t.Fatal("a refused call reached the API")
	}
	// the key is in no error
	f.status.Store(500)
	_, err := ops["status"].Do(t.Context(), Input{Args: args})
	if err == nil || strings.Contains(err.Error(), sbTestToken) || strings.Contains(err.Error(), sbTestSecret) {
		t.Fatalf("%v", err)
	}
	if s := (switchBotKey{Token: sbTestToken, Secret: sbTestSecret}); strings.Contains(s.String()+s.GoString(), sbTestToken) {
		t.Fatal("the key prints")
	}
}

func TestSwitchBotDefinition(t *testing.T) {
	p, ok := Lookup(SwitchBot)
	if !ok || p.Billing != BillingFree || p.NeedsOAuth || p.Fake || p.EnvName() != "SWITCHBOT" || len(p.Hosts) != 1 || p.Hosts[0] != switchBotHost {
		t.Fatalf("%+v", p)
	}
	for _, o := range p.Ops {
		if o.Write || o.CacheTTL != 0 {
			t.Fatalf("%s: read only and never cached (the cache is not per person)", o.Name)
		}
	}
	if _, err := p.CheckArgs("command", nil); err == nil {
		t.Fatal("device commands are a follow-up")
	}
	if _, err := p.CheckArgs("devices", map[string]string{"deviceId": sbMeter}); err == nil {
		t.Fatal("devices takes no argument")
	}
	if _, err := p.CheckArgs("status", map[string]string{"deviceId": "a/b"}); err == nil {
		t.Fatal("bad device id accepted at CheckArgs")
	}
	if _, err := p.CheckArgs("status", map[string]string{"deviceId": sbMeter}); err != nil {
		t.Fatal(err)
	}
	// until the per-user key store (#1671): off even when switched on
	if _, err := p.New(Secrets{APIKey: sbTestToken}); err == nil {
		t.Fatal("New built ops without a per-user key store")
	}
	if _, err := newSwitchBot(nil, switchBotHost, http.DefaultClient, nil); err == nil {
		t.Fatal("no key source accepted")
	}
	c, err := ConfigFromEnv(func(string) string { return "" })
	if err != nil || c.Enabled[SwitchBot] {
		t.Fatalf("enabled by default: %v", err)
	}
	e := newEnv(t, opts{cfg: func(c *Config) { c.Enabled[SwitchBot] = true }})
	if e.svc.Enabled(SwitchBot) {
		t.Fatal("on without the per-user key store")
	}
}

// Through Service.Call: one `runs`, no coins; a refused key gives the run back.
func TestSwitchBotThroughService(t *testing.T) {
	f, srv := newSBFake(t)
	e := newEnv(t, opts{cfg: func(c *Config) { c.Enabled[SwitchBot] = true }})
	e.svc.ops[SwitchBot] = newSBOps(t, srv)
	res, err := e.call(e.req(SwitchBot, "status", map[string]string{"deviceId": sbMeter}))
	if err != nil || res.Output.Item == nil || res.Charge.Coins != 0 || e.runs() != 1 {
		t.Fatalf("%+v %v runs=%d", res, err, e.runs())
	}
	// never cached: the same call reaches the API again
	n := f.calls.Load()
	if _, err := e.call(e.req(SwitchBot, "status", map[string]string{"deviceId": sbMeter})); err != nil || f.calls.Load() != n+1 {
		t.Fatalf("cached? %v", err)
	}
	f.status.Store(401)
	if _, err := e.call(e.req(SwitchBot, "devices", nil)); Reason(err) != ReasonNotConnected || e.runs() != 2 {
		t.Fatalf("%v runs=%d", err, e.runs())
	}
	if strings.Contains(e.logs.String(), sbTestToken) || strings.Contains(e.logs.String(), sbTestSecret) {
		t.Fatal("the key is in a log")
	}
}
