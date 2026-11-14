package extapi

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// SwitchBot API v1.1 (#1704): a person's own devices and the readings of
// their meters and plugs, read with that person's token and secret. Free: a
// call is one `runs`, no coins (SwitchBot limits an account to 10,000 calls
// a day).
//
//	devices()             Rows: name / type / device id (physical devices only)
//	status(deviceId)      Item: "24.5°C・湿度 52%" / type / temperature;
//	                      Rows + Cells: one per reading (for the import)
//
// Read only. Device commands (POST /v1.1/devices/{id}/commands) are a
// follow-up: they move things in a home, and whether a server rule may send
// them without a person pressing is the owner's decision (#1704).
//
// status returns only an allowlist of readings (temperature, humidity, CO₂,
// battery, on/off). Readings that tell whether someone is home (motion,
// door / contact open, lock state, brightness) are never returned.
//
// Keys: the token and secret are the person's own (SwitchBot app → 開発者向
// けオプション), not the operator's. They come from the per-user key store
// (#1671), which is not in yet, so the registered provider stays off even
// with EXT_SWITCHBOT_ENABLED=1 (New refuses). Each request is signed:
// sign = base64(HMAC-SHA256(secret, token + t + nonce)). The token and
// secret go in headers only, never in a URL, a log or an answer.
//
// Answers are never cached: the shared cache is keyed by the arguments, not
// by the person.
const (
	SwitchBot = "switchbot"

	switchBotHost = "https://api.switch-bot.com"
)

// errSwitchBotNoKeyStore keeps the provider off until #1671 can hand an op
// the person's token and secret.
var errSwitchBotNoKeyStore = errors.New("switchbot: per-user keys (#1671) are not in yet")

var (
	// switchBotDeviceRe: a device id (12 hex for a BLE device; letters,
	// digits and "-" for others).
	switchBotDeviceRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,63}$`)
	switchBotTokenRe  = regexp.MustCompile(`^[A-Za-z0-9]{16,256}$`)
	switchBotSecretRe = regexp.MustCompile(`^[A-Za-z0-9]{8,128}$`)
)

func init() {
	register(Provider{ID: SwitchBot, Billing: BillingFree,
		Ops: []OpSpec{
			{Name: "devices"},
			{Name: "status", Args: []string{"deviceId"}, Check: checkSwitchBotDevice},
		},
		Hosts: []string{switchBotHost},
		New: func(Secrets) (map[string]Operation, error) {
			return nil, errSwitchBotNoKeyStore
		}})
}

func checkSwitchBotDevice(args map[string]string) error {
	if !switchBotDeviceRe.MatchString(strings.TrimSpace(args["deviceId"])) {
		return errors.New("deviceId must be a SwitchBot device id")
	}
	return nil
}

// switchBotKey is a person's SwitchBot token and secret. Never log it.
type switchBotKey struct {
	Token  string
	Secret string
}

// String keeps the key out of %v.
func (switchBotKey) String() string { return "[redacted]" }

// GoString keeps the key out of %#v.
func (switchBotKey) GoString() string { return "[redacted]" }

// switchBotKeys finds the key of the person a call acts for (the per-user
// key store, #1671). ErrNotConnected when the person has none.
type switchBotKeys func(ctx context.Context, in Input) (switchBotKey, error)

// newSwitchBot builds the ops against base (the API, or a test server).
func newSwitchBot(keys switchBotKeys, base string, hc *http.Client, now func() time.Time) (map[string]Operation, error) {
	if keys == nil {
		return nil, errSwitchBotNoKeyStore
	}
	if now == nil {
		now = time.Now
	}
	s := &switchBot{keys: keys, base: base, hc: hc, now: now}
	return map[string]Operation{
		"devices": OperationFunc(s.devices),
		"status":  OperationFunc(s.status),
	}, nil
}

type switchBot struct {
	keys switchBotKeys
	base string
	hc   *http.Client
	now  func() time.Time
}

// switchBotAnswer is the envelope of every v1.1 answer.
type switchBotAnswer struct {
	StatusCode int             `json:"statusCode"`
	Message    string          `json:"message"`
	Body       json.RawMessage `json:"body"`
}

type switchBotDevices struct {
	DeviceList []struct {
		DeviceID   string `json:"deviceId"`
		DeviceName string `json:"deviceName"`
		DeviceType string `json:"deviceType"`
	} `json:"deviceList"`
}

// switchBotStatus holds only the readings status may return.
type switchBotStatus struct {
	DeviceType  string   `json:"deviceType"`
	Temperature *float64 `json:"temperature"`
	Humidity    *float64 `json:"humidity"`
	CO2         *float64 `json:"CO2"`
	Battery     *float64 `json:"battery"`
	Power       string   `json:"power"`
}

// switchBotMaxDevices caps devices' rows.
const switchBotMaxDevices = 100

// sign returns the auth headers of one request.
func switchBotSign(k switchBotKey, now time.Time) (t, nonce, sign string, err error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", "", "", errors.New("switchbot: no nonce")
	}
	t = strconv.FormatInt(now.UnixMilli(), 10)
	nonce = hex.EncodeToString(b)
	m := hmac.New(sha256.New, []byte(k.Secret))
	m.Write([]byte(k.Token + t + nonce))
	return t, nonce, base64.StdEncoding.EncodeToString(m.Sum(nil)), nil
}

// get calls GET path and decodes the answer's body into out.
func (s *switchBot) get(ctx context.Context, in Input, path string, out any) error {
	k, err := s.keys(ctx, in)
	if err != nil {
		return err
	}
	if !switchBotTokenRe.MatchString(k.Token) || !switchBotSecretRe.MatchString(k.Secret) {
		return fmt.Errorf("%w: the stored SwitchBot key is not a key", ErrNotConnected)
	}
	t, nonce, sign, err := switchBotSign(k, s.now())
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.base+path, nil)
	if err != nil {
		return errors.New("switchbot: building the request failed")
	}
	req.Header.Set("Authorization", k.Token)
	req.Header.Set("sign", sign)
	req.Header.Set("t", t)
	req.Header.Set("nonce", nonce)
	req.Header.Set("Accept", "application/json")
	var a switchBotAnswer
	if err := sendJSON(ctx, s.hc, req, &a); err != nil {
		var pe *ProviderError
		if errors.As(err, &pe) && (pe.Status == http.StatusUnauthorized || pe.Status == http.StatusForbidden) {
			// the token was reset or is wrong: the person pastes it again
			return fmt.Errorf("%w: SwitchBot refused the key", ErrNotConnected)
		}
		return err
	}
	if err := switchBotCodeError(a.StatusCode); err != nil {
		return err
	}
	if err := json.Unmarshal(a.Body, out); err != nil {
		return &ProviderError{Status: http.StatusOK, Err: errors.New("switchbot: the answer is not the expected JSON")}
	}
	return nil
}

// switchBotCodeError turns the answer's statusCode (SwitchBot answers HTTP
// 200 for most failures) into the foundation's errors.
func switchBotCodeError(code int) error {
	switch code {
	case 100:
		return nil
	case 151, 152, 160:
		// wrong device type, no such device, not supported
		return fmt.Errorf("%w: SwitchBot does not have that device or reading (%d)", ErrUnknown, code)
	case 161, 171:
		// the device or its hub is offline
		return &ProviderError{Status: http.StatusServiceUnavailable, Err: fmt.Errorf("switchbot: offline (%d)", code)}
	case 190:
		return &ProviderError{Status: http.StatusBadGateway, Err: errors.New("switchbot: device error (190)")}
	}
	return &ProviderError{Status: http.StatusBadGateway, Err: fmt.Errorf("switchbot: statusCode %d", code)}
}

func (s *switchBot) devices(ctx context.Context, in Input) (Output, error) {
	var d switchBotDevices
	if err := s.get(ctx, in, "/v1.1/devices", &d); err != nil {
		return Output{}, err
	}
	out := Output{Rows: []Row{}}
	for _, it := range d.DeviceList {
		if !switchBotDeviceRe.MatchString(it.DeviceID) {
			continue
		}
		out.Rows = append(out.Rows, Row{Title: clip(it.DeviceName, 100), Subtitle: clip(it.DeviceType, 50), Value: it.DeviceID})
		if len(out.Rows) == switchBotMaxDevices {
			break
		}
	}
	return out, nil
}

func (s *switchBot) status(ctx context.Context, in Input) (Output, error) {
	id := strings.TrimSpace(in.Args["deviceId"])
	if !switchBotDeviceRe.MatchString(id) {
		return Output{}, fmt.Errorf("%w: deviceId", ErrUnknown)
	}
	var st switchBotStatus
	if err := s.get(ctx, in, "/v1.1/devices/"+url.PathEscape(id)+"/status", &st); err != nil {
		return Output{}, err
	}
	var rows []Row
	var cells [][]string
	var head []string
	add := func(label, key, v string) {
		rows = append(rows, Row{Title: label, Value: v})
		cells = append(cells, []string{key, v})
	}
	value := ""
	if st.Temperature != nil && finite(*st.Temperature) {
		v := celsius(*st.Temperature)
		add("温度", "temperature", v)
		head = append(head, v)
		value = v
	}
	if st.Humidity != nil && finite(*st.Humidity) {
		v := strconv.Itoa(int(math.Round(*st.Humidity))) + "%"
		add("湿度", "humidity", v)
		head = append(head, "湿度 "+v)
		if value == "" {
			value = v
		}
	}
	if st.CO2 != nil && finite(*st.CO2) {
		v := strconv.Itoa(int(math.Round(*st.CO2))) + " ppm"
		add("CO₂", "co2", v)
		head = append(head, "CO₂ "+v)
	}
	switch p := strings.ToLower(st.Power); p {
	case "on", "off":
		v := map[string]string{"on": "オン", "off": "オフ"}[p]
		add("電源", "power", v)
		head = append(head, "電源 "+v)
		if value == "" {
			value = v
		}
	}
	if st.Battery != nil && finite(*st.Battery) && *st.Battery >= 0 && *st.Battery <= 100 {
		add("電池", "battery", strconv.Itoa(int(math.Round(*st.Battery)))+"%")
	}
	if len(rows) == 0 {
		return Output{}, fmt.Errorf("%w: the device has no reading status returns", ErrUnknown)
	}
	title := strings.Join(head, "・")
	if title == "" {
		title = rows[0].Title + " " + rows[0].Value
	}
	return Output{Item: &Row{Title: title, Subtitle: clip(st.DeviceType, 50), Value: value}, Rows: rows, Cells: cells}, nil
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// clip cuts s to n runes.
func clip(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) > n {
		return string(r[:n])
	}
	return string(r)
}
