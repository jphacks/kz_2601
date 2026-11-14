package extapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Nature Remo Cloud API v1 (#1705): the room sensors and the appliance list
// of a person's own Remo, read with that person's access token. Free: a call
// is one `runs`, no coins (Nature limits a token to 30 requests per 5
// minutes; its 429 is a retryable provider error).
//
//	devices(device?)        Rows: name / temperature (value, ℃) / humidity (subtitle)
//	appliances(appliance?)  Rows: nickname / kind
//
// Read only. Operating appliances (POST /1/signals/{id}/send,
// /1/appliances/{id}/aircon_settings) is a follow-up: it moves things in a
// home, and whether a server rule may do it without a person pressing is
// the owner's decision (same as SwitchBot, #1704).
//
// Only an allowlist of readings goes back: temperature and humidity.
// Readings that tell whether someone is home (motion `mo`, illuminance `il`
// = lights on, an air conditioner's last on / off) are never returned; nor
// are ids, serials or MAC addresses.
//
// Key: the access token is the person's own (home.nature.global), not the
// operator's, and it is not scoped (it can operate the whole home). It
// comes from the per-user key store (#1671), which is not in yet, so the
// registered provider stays off even with EXT_NATURE_REMO_ENABLED=1 (New
// refuses). The token goes in the Authorization header only, never in a
// URL, a log or an answer.
//
// Answers are never cached: the shared cache is keyed by the arguments, not
// by the person.
const (
	NatureRemo = "natureRemo"

	natureRemoHost = "https://api.nature.global"
)

// errNatureRemoNoKeyStore keeps the provider off until #1671 can hand an op
// the person's token.
var errNatureRemoNoKeyStore = errors.New("natureRemo: per-user keys (#1671) are not in yet")

func init() {
	register(Provider{ID: NatureRemo, Billing: BillingFree,
		Ops: []OpSpec{
			{Name: "devices", Args: []string{"device"}, Check: checkNatureRemoDevices},
			{Name: "appliances", Args: []string{"appliance"}, Check: checkNatureRemoAppliances},
		},
		Hosts: []string{natureRemoHost},
		New: func(Secrets) (map[string]Operation, error) {
			return nil, errNatureRemoNoKeyStore
		}})
}

// natureRemoKey is a person's Nature access token. Never log it.
type natureRemoKey struct{ Token string }

// String keeps the key out of %v.
func (natureRemoKey) String() string { return "[redacted]" }

// GoString keeps the key out of %#v.
func (natureRemoKey) GoString() string { return "[redacted]" }

// natureRemoKeys finds the key of the person a call acts for (the per-user
// key store, #1671). ErrNotConnected when the person has none.
type natureRemoKeys func(ctx context.Context, in Input) (natureRemoKey, error)

// newNatureRemo builds the ops against base (the API's /1, or a test server).
func newNatureRemo(keys natureRemoKeys, base string, hc *http.Client) (map[string]Operation, error) {
	if keys == nil {
		return nil, errNatureRemoNoKeyStore
	}
	if hc == nil {
		hc = NewHTTPClient([]string{natureRemoHost}, DefaultTimeout)
	}
	c := &natureRemo{keys: keys, base: base, hc: hc}
	return map[string]Operation{"devices": OperationFunc(c.devices), "appliances": OperationFunc(c.appliances)}, nil
}

type natureRemo struct {
	keys natureRemoKeys
	base string
	hc   *http.Client
}

// --- arguments ---

// natureRemoName reads an optional name-or-id argument: one line, at most
// 64 characters.
func natureRemoName(a map[string]string, name string) (string, error) {
	v := strings.TrimSpace(a[name])
	switch {
	case !utf8.ValidString(v) || utf8.RuneCountInString(v) > 64:
		return "", fmt.Errorf("%w: %s is over 64 characters", ErrUnknown, name)
	case strings.ContainsFunc(v, func(r rune) bool { return r < 0x20 || r == 0x7f }):
		return "", fmt.Errorf("%w: %s has control characters", ErrUnknown, name)
	}
	return v, nil
}

func checkNatureRemoDevices(a map[string]string) error {
	_, err := natureRemoName(a, "device")
	return err
}

func checkNatureRemoAppliances(a map[string]string) error {
	_, err := natureRemoName(a, "appliance")
	return err
}

// natureRemoMatches: v is the id or the exact name.
func natureRemoMatches(v, id, name string) bool {
	return v == id || v == strings.TrimSpace(name)
}

// --- answers ---

type natureRemoDevice struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	NewestEvents map[string]struct {
		Val *float64 `json:"val"`
	} `json:"newest_events"`
}

// row: title = the name, value = the temperature (℃, a number), subtitle =
// the humidity. Nothing else of the device is read.
func (d natureRemoDevice) row() Row {
	r := Row{Title: d.Name}
	if e, ok := d.NewestEvents["te"]; ok && e.Val != nil {
		r.Value = strconv.FormatFloat(*e.Val, 'f', -1, 64)
	}
	if e, ok := d.NewestEvents["hu"]; ok && e.Val != nil {
		r.Subtitle = "湿度 " + strconv.FormatFloat(*e.Val, 'f', -1, 64) + "%"
	}
	return r
}

type natureRemoAppliance struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Nickname string `json:"nickname"`
}

var natureRemoKinds = map[string]string{"AC": "エアコン", "TV": "テレビ", "LIGHT": "照明", "IR": "赤外線"}

// --- ops ---

// devices lists the person's Remo devices with their newest temperature and
// humidity, or the one named (or with the id) device.
func (c *natureRemo) devices(ctx context.Context, in Input) (Output, error) {
	want, err := natureRemoName(in.Args, "device")
	if err != nil {
		return Output{}, err
	}
	var list []natureRemoDevice
	if err := c.get(ctx, in, "/devices", &list); err != nil {
		return Output{}, err
	}
	out := Output{Rows: []Row{}}
	for _, d := range list {
		if want == "" || natureRemoMatches(want, d.ID, d.Name) {
			out.Rows = append(out.Rows, d.row())
		}
	}
	if want != "" && len(out.Rows) == 0 {
		return Output{}, fmt.Errorf("%w: no such device", ErrUnknown)
	}
	return out, nil
}

// appliances lists the person's appliances (or the one named): nickname
// and kind only.
func (c *natureRemo) appliances(ctx context.Context, in Input) (Output, error) {
	want, err := natureRemoName(in.Args, "appliance")
	if err != nil {
		return Output{}, err
	}
	var list []natureRemoAppliance
	if err := c.get(ctx, in, "/appliances", &list); err != nil {
		return Output{}, err
	}
	out := Output{Rows: []Row{}}
	for _, a := range list {
		if want == "" || natureRemoMatches(want, a.ID, a.Nickname) {
			out.Rows = append(out.Rows, Row{Title: a.Nickname, Subtitle: natureRemoKinds[a.Type]})
		}
	}
	if want != "" && len(out.Rows) == 0 {
		return Output{}, fmt.Errorf("%w: no such appliance", ErrUnknown)
	}
	return out, nil
}

// get calls the API with the person's token. 401 / 403: the token is no
// longer taken (ErrNotConnected: the person saves it again); 429 / 5xx are
// retryable (sendJSON); the body is never kept.
func (c *natureRemo) get(ctx context.Context, in Input, path string, out any) error {
	key, err := c.keys(ctx, in)
	if err != nil {
		return err
	}
	if key.Token == "" {
		return ErrNotConnected
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+key.Token)
	req.Header.Set("Accept", "application/json")
	err = sendJSON(ctx, c.hc, req, out)
	var pe *ProviderError
	if errors.As(err, &pe) && (pe.Status == http.StatusUnauthorized || pe.Status == http.StatusForbidden) {
		return ErrNotConnected
	}
	return err
}
