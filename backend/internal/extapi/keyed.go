package extapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Helpers of the key-based providers (openweather.go, deepl.go,
// googleplaces.go): one request, the answer decoded, and every failure
// turned into the foundation's errors without the URL (a key may sit in
// its query) or the provider's body.

// sendJSON sends req and decodes a 2xx answer into out.
//
//   - our deadline (or the client's timeout) → context.DeadlineExceeded:
//     upstream_timeout, the run and the coins go back
//   - 429 / 5xx / a transport error → ProviderError{Retryable}: given back
//   - another status → ProviderError (not billable: none of these
//     providers charges a refused request)
//   - a host outside Provider.Hosts → ErrEgress (internal)
func sendJSON(ctx context.Context, hc *http.Client, req *http.Request, out any) error {
	resp, err := hc.Do(req.WithContext(ctx))
	if err != nil {
		return transportError(ctx, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := ReadBody(resp.Body)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return &ProviderError{Status: resp.StatusCode, Retryable: true, Err: errors.New("reading the answer failed")}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return statusError(resp.StatusCode)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return &ProviderError{Status: resp.StatusCode, Err: errors.New("the answer is not the expected JSON")}
	}
	return nil
}

// statusError is a non-2xx answer. The body is not kept: it may echo the
// request.
func statusError(status int) error {
	retry := status == http.StatusTooManyRequests || status >= 500
	return &ProviderError{Status: status, Retryable: retry, Err: errors.New(http.StatusText(status))}
}

// transportError drops the *url.Error's URL (a key may be in the query).
func transportError(ctx context.Context, err error) error {
	if errors.Is(err, ErrEgress) {
		return ErrEgress
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return context.DeadlineExceeded
	}
	msg := "transport error"
	var ue *url.Error
	if errors.As(err, &ue) && ue.Err != nil {
		msg += ": " + ue.Err.Error()
	}
	return &ProviderError{Retryable: true, Err: errors.New(msg)}
}

// argLatLng reads args lat / lng (decimal degrees) and rounds them to
// digits places (2 ≈ 1 km: only the rough place leaves the runner).
// Missing both is ok=false with no error; one without the other, a value
// out of range or not a plain number is ErrUnknown (bad_request).
func argLatLng(args map[string]string, digits int) (lat, lng float64, ok bool, err error) {
	ls, gs := strings.TrimSpace(args["lat"]), strings.TrimSpace(args["lng"])
	if ls == "" && gs == "" {
		return 0, 0, false, nil
	}
	lat, err1 := parseCoord(ls, 90)
	lng, err2 := parseCoord(gs, 180)
	if err1 != nil || err2 != nil {
		return 0, 0, false, fmt.Errorf("%w: lat / lng", ErrUnknown)
	}
	p := math.Pow10(digits)
	return math.Round(lat*p) / p, math.Round(lng*p) / p, true, nil
}

func parseCoord(s string, limit float64) (float64, error) {
	if s == "" || len(s) > 32 || strings.Trim(s, "0123456789.-+") != "" {
		return 0, ErrUnknown // a plain decimal only (no hex, exponent, Inf)
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < -limit || v > limit {
		return 0, ErrUnknown
	}
	return v, nil
}

// argInt reads an optional integer arg within [lo, hi] (def when empty).
func argInt(args map[string]string, name string, def, lo, hi int) (int, error) {
	s := strings.TrimSpace(args[name])
	if s == "" {
		return def, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < lo || n > hi {
		return 0, fmt.Errorf("%w: %s must be %d-%d", ErrUnknown, name, lo, hi)
	}
	return n, nil
}
