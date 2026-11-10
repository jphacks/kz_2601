package ingest

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/loop"
)

// AppStoreReviews reads customer reviews with the App Store Connect API
// (GET /v1/apps/{id}/customerReviews). Off unless LOOP_SOURCE_APPSTORE=on and
// all four LOOP_ASC_* values are set (AppStoreConfigFromEnv). The key is
// given as the env value itself (Cloud Run maps a Secret Manager secret to
// it); this package never reads a key file and nothing is committed.
//
// Kept: rating, title + body (masked by ingest), created date. Dropped: the
// reviewer's nickname and territory (#367).
type AppStoreReviews struct {
	Cfg AppStoreConfig
	// BaseURL: "" = https://api.appstoreconnect.apple.com
	BaseURL string
	HTTP    *http.Client
	Now     func() time.Time
	// MaxPages bounds one run (200 reviews a page). Default 5.
	MaxPages int
}

// AppStoreConfig is the API key of an App Store Connect team (role: Customer
// Support is enough to read reviews).
type AppStoreConfig struct {
	IssuerID string
	KeyID    string
	Key      *ecdsa.PrivateKey
	AppID    string // the app's Apple ID (numeric)
}

// Env names (values come from Secret Manager in Cloud Run; locally from your shell).
const (
	EnvAppStoreOn     = "LOOP_SOURCE_APPSTORE"
	EnvAppStoreIssuer = "LOOP_ASC_ISSUER_ID"
	EnvAppStoreKeyID  = "LOOP_ASC_KEY_ID"
	EnvAppStoreKey    = "LOOP_ASC_PRIVATE_KEY" // the .p8 contents (PEM), not a path
	EnvAppStoreAppID  = "LOOP_ASC_APP_ID"
)

// ErrAppStoreOff is returned by AppStoreConfigFromEnv when the source is off.
var ErrAppStoreOff = errors.New("app store reviews are off (" + EnvAppStoreOn + " != on)")

// AppStoreConfigFromEnv reads the LOOP_ASC_* values.
func AppStoreConfigFromEnv(getenv func(string) string) (AppStoreConfig, error) {
	if getenv(EnvAppStoreOn) != "on" {
		return AppStoreConfig{}, ErrAppStoreOff
	}
	cfg := AppStoreConfig{IssuerID: getenv(EnvAppStoreIssuer), KeyID: getenv(EnvAppStoreKeyID), AppID: getenv(EnvAppStoreAppID)}
	if cfg.IssuerID == "" || cfg.KeyID == "" || cfg.AppID == "" || getenv(EnvAppStoreKey) == "" {
		return AppStoreConfig{}, fmt.Errorf("%s=on needs %s, %s, %s and %s", EnvAppStoreOn, EnvAppStoreIssuer, EnvAppStoreKeyID, EnvAppStoreKey, EnvAppStoreAppID)
	}
	key, err := ParseP8([]byte(getenv(EnvAppStoreKey)))
	if err != nil {
		return AppStoreConfig{}, err
	}
	cfg.Key = key
	return cfg, nil
}

// ParseP8 parses an App Store Connect API key (PKCS#8 PEM, P-256).
func ParseP8(pemBytes []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("app store key: no PEM block")
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("app store key: not PKCS#8")
	}
	ec, ok := k.(*ecdsa.PrivateKey)
	if !ok || ec.Curve.Params().Name != "P-256" {
		return nil, errors.New("app store key: not a P-256 key")
	}
	return ec, nil
}

// Source implements Fetcher.
func (AppStoreReviews) Source() loop.Source { return loop.SourceStoreReview }

type reviewPage struct {
	Data []struct {
		ID         string `json:"id"`
		Attributes struct {
			Rating      int       `json:"rating"`
			Title       string    `json:"title"`
			Body        string    `json:"body"`
			CreatedDate time.Time `json:"createdDate"`
		} `json:"attributes"`
	} `json:"data"`
	Links struct {
		Next string `json:"next"`
	} `json:"links"`
}

// Fetch implements Fetcher: newest first, stopping at the first review older
// than since.
func (a AppStoreReviews) Fetch(ctx context.Context, since time.Time) ([]Raw, error) {
	base := a.BaseURL
	if base == "" {
		base = "https://api.appstoreconnect.apple.com"
	}
	hc := a.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 20 * time.Second}
	}
	pages := a.MaxPages
	if pages <= 0 {
		pages = 5
	}
	next := base + "/v1/apps/" + url.PathEscape(a.Cfg.AppID) + "/customerReviews?sort=-createdDate&limit=200"
	out := []Raw{}
	for p := 0; p < pages && next != ""; p++ {
		if !strings.HasPrefix(next, base+"/") {
			return nil, errors.New("app store: next link points elsewhere")
		}
		page, err := a.get(ctx, hc, next)
		if err != nil {
			return nil, err
		}
		for _, d := range page.Data {
			at := d.Attributes
			if at.CreatedDate.Before(since) {
				return out, nil
			}
			text := strings.TrimSpace(at.Title + "\n" + at.Body)
			out = append(out, Raw{Source: loop.SourceStoreReview, SourceID: d.ID, Text: text,
				Kind: fmt.Sprintf("rating-%d", at.Rating), Rating: at.Rating, CreatedAt: at.CreatedDate})
		}
		next = page.Links.Next
	}
	return out, nil
}

func (a AppStoreReviews) get(ctx context.Context, hc *http.Client, u string) (*reviewPage, error) {
	now := time.Now
	if a.Now != nil {
		now = a.Now
	}
	tok, err := signASC(a.Cfg, now())
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("app store: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		return nil, fmt.Errorf("app store: status %d", resp.StatusCode)
	}
	var page reviewPage
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&page); err != nil {
		return nil, fmt.Errorf("app store: decode: %w", err)
	}
	return &page, nil
}

// signASC makes the ES256 token App Store Connect expects (at most 20 minutes).
func signASC(cfg AppStoreConfig, now time.Time) (string, error) {
	if cfg.Key == nil {
		return "", errors.New("app store: no key")
	}
	enc := base64.RawURLEncoding
	h, _ := json.Marshal(map[string]string{"alg": "ES256", "kid": cfg.KeyID, "typ": "JWT"})
	c, _ := json.Marshal(map[string]any{"iss": cfg.IssuerID, "iat": now.Unix(), "exp": now.Add(15 * time.Minute).Unix(), "aud": "appstoreconnect-v1"})
	signing := enc.EncodeToString(h) + "." + enc.EncodeToString(c)
	sum := sha256.Sum256([]byte(signing))
	r, s, err := ecdsa.Sign(rand.Reader, cfg.Key, sum[:])
	if err != nil {
		return "", err
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return signing + "." + enc.EncodeToString(sig), nil
}
