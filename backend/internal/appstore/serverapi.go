package appstore

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
	"time"
)

// App Store Server API hosts (#315: reconciling missed notifications).
const (
	ServerAPIProduction = "https://api.storekit.itunes.apple.com"
	ServerAPISandbox    = "https://api.storekit-sandbox.itunes.apple.com"
)

// ErrNotFound is the Server API's 404 (unknown transaction in that environment).
var ErrNotFound = errors.New("appstore: transaction not found")

// ServerAPI calls the App Store Server API with an In-App Purchase key
// (App Store Connect > Users and Access > Integrations). Only "Get All
// Subscription Statuses" is used.
type ServerAPI struct {
	KeyID    string
	IssuerID string
	BundleID string
	Key      *ecdsa.PrivateKey
	// BaseURL per environment; empty = ServerAPIProduction / ServerAPISandbox.
	// Tests point both at an httptest server.
	BaseURL map[string]string
	HTTP    *http.Client
	Now     func() time.Time
}

// ParsePrivateKey reads the .p8 (PKCS#8 PEM, P-256) downloaded from App Store Connect.
func ParsePrivateKey(pemBytes []byte) (*ecdsa.PrivateKey, error) {
	b, _ := pem.Decode(pemBytes)
	if b == nil {
		return nil, errors.New("appstore: no PEM block in private key")
	}
	k, err := x509.ParsePKCS8PrivateKey(b.Bytes)
	if err != nil {
		return nil, fmt.Errorf("appstore: private key: %w", err)
	}
	ec, ok := k.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("appstore: private key is not ECDSA")
	}
	return ec, nil
}

// SubscriptionStatus is one lastTransactions item (still signed).
type SubscriptionStatus struct {
	OriginalTransactionID string `json:"originalTransactionId"`
	Status                int    `json:"status"`
	SignedTransactionInfo string `json:"signedTransactionInfo"`
	SignedRenewalInfo     string `json:"signedRenewalInfo"`
}

// SubscriptionStatuses calls GET /inApps/v1/subscriptions/{transactionId} in
// environment (EnvSandbox / EnvProduction) and returns the latest signed
// transaction / renewal info of each subscription of that customer.
func (s *ServerAPI) SubscriptionStatuses(ctx context.Context, environment, transactionID string) ([]SubscriptionStatus, error) {
	base := s.baseURL(environment)
	if base == "" {
		return nil, fmt.Errorf("appstore: unknown environment %q", environment)
	}
	token, err := s.token()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/inApps/v1/subscriptions/"+url.PathEscape(transactionID), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	client := s.HTTP
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	switch {
	case res.StatusCode == http.StatusNotFound:
		return nil, ErrNotFound
	case res.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("appstore: server api status %d", res.StatusCode)
	}
	var out struct {
		Data []struct {
			LastTransactions []SubscriptionStatus `json:"lastTransactions"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("appstore: server api body: %w", err)
	}
	var all []SubscriptionStatus
	for _, g := range out.Data {
		all = append(all, g.LastTransactions...)
	}
	return all, nil
}

func (s *ServerAPI) baseURL(env string) string {
	if u, ok := s.BaseURL[env]; ok {
		return u
	}
	switch env {
	case EnvProduction:
		return ServerAPIProduction
	case EnvSandbox:
		return ServerAPISandbox
	}
	return ""
}

// token is the ES256 JWT Apple requires (valid 5 minutes; max 60).
func (s *ServerAPI) token() (string, error) {
	if s.Key == nil || s.KeyID == "" || s.IssuerID == "" || s.BundleID == "" {
		return "", errors.New("appstore: server api key is not configured")
	}
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	h, _ := json.Marshal(map[string]string{"alg": "ES256", "kid": s.KeyID, "typ": "JWT"})
	p, _ := json.Marshal(map[string]any{
		"iss": s.IssuerID,
		"iat": now.Unix(),
		"exp": now.Add(5 * time.Minute).Unix(),
		"aud": "appstoreconnect-v1",
		"bid": s.BundleID,
	})
	input := base64.RawURLEncoding.EncodeToString(h) + "." + base64.RawURLEncoding.EncodeToString(p)
	sum := sha256.Sum256([]byte(input))
	r, ss, err := ecdsa.Sign(rand.Reader, s.Key, sum[:])
	if err != nil {
		return "", err
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	ss.FillBytes(sig[32:])
	return input + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}
