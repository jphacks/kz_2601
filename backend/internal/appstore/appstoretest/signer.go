// Package appstoretest mints StoreKit-style JWS transactions signed by a fake
// CA (root -> intermediate -> leaf with Apple's marker extensions), so the
// verifier's chain and signature logic is tested without Apple.
package appstoretest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"testing"
	"time"
)

// Signer holds a fake chain. Roots trusts only its root.
type Signer struct {
	Root, Intermediate, Leaf *x509.Certificate
	Roots                    *x509.CertPool
	key                      *ecdsa.PrivateKey
	// X5c overrides the header chain when set (tests of broken chains).
	X5c []string
}

// Options tweak the generated chain.
type Options struct {
	OmitLeafMarker         bool
	OmitIntermediateMarker bool
	NotBefore, NotAfter    time.Time // leaf validity; zero = now-1y .. now+1y
}

var (
	leafMarker  = asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 6, 11, 1}
	interMarker = asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 6, 2, 1}
)

// New builds a fresh fake chain.
func New(t testing.TB, o Options) *Signer {
	t.Helper()
	now := time.Now()
	rootKey, rootCert := mint(t, "Fake Root CA", nil, nil, true, nil, now.Add(-10*365*24*time.Hour), now.Add(10*365*24*time.Hour))
	var interExt []pkix.Extension
	if !o.OmitIntermediateMarker {
		interExt = []pkix.Extension{{Id: interMarker, Value: []byte{5, 0}}}
	}
	interKey, interCert := mint(t, "Fake WWDR", rootCert, rootKey, true, interExt, now.Add(-5*365*24*time.Hour), now.Add(5*365*24*time.Hour))
	var leafExt []pkix.Extension
	if !o.OmitLeafMarker {
		leafExt = []pkix.Extension{{Id: leafMarker, Value: []byte{5, 0}}}
	}
	nb, na := o.NotBefore, o.NotAfter
	if nb.IsZero() {
		nb = now.Add(-365 * 24 * time.Hour)
	}
	if na.IsZero() {
		na = now.Add(365 * 24 * time.Hour)
	}
	leafKey, leafCert := mint(t, "Fake StoreKit", interCert, interKey, false, leafExt, nb, na)
	pool := x509.NewCertPool()
	pool.AddCert(rootCert)
	return &Signer{Root: rootCert, Intermediate: interCert, Leaf: leafCert, Roots: pool, key: leafKey}
}

func mint(t testing.TB, cn string, parent *x509.Certificate, parentKey *ecdsa.PrivateKey, ca bool, ext []pkix.Extension, nb, na time.Time) (*ecdsa.PrivateKey, *x509.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             nb,
		NotAfter:              na,
		BasicConstraintsValid: true,
		IsCA:                  ca,
		ExtraExtensions:       ext,
	}
	if ca {
		tmpl.KeyUsage = x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature
	} else {
		tmpl.KeyUsage = x509.KeyUsageDigitalSignature
	}
	signer, signerKey := tmpl, key
	if parent != nil {
		signer, signerKey = parent, parentKey
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, signer, &key.PublicKey, signerKey)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return key, cert
}

// Chain is the default x5c header value.
func (s *Signer) Chain() []string {
	return []string{
		base64.StdEncoding.EncodeToString(s.Leaf.Raw),
		base64.StdEncoding.EncodeToString(s.Intermediate.Raw),
		base64.StdEncoding.EncodeToString(s.Root.Raw),
	}
}

// Sign returns a compact ES256 JWS of payload.
func (s *Signer) Sign(t testing.TB, payload any) string {
	t.Helper()
	x5c := s.X5c
	if x5c == nil {
		x5c = s.Chain()
	}
	h, _ := json.Marshal(map[string]any{"alg": "ES256", "x5c": x5c})
	p, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	input := b64(h) + "." + b64(p)
	sum := sha256.Sum256([]byte(input))
	r, ss, err := ecdsa.Sign(rand.Reader, s.key, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	ss.FillBytes(sig[32:])
	return input + "." + b64(sig)
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// Transaction is a JWSTransaction payload with sensible defaults.
type Transaction struct {
	TransactionID         string `json:"transactionId"`
	OriginalTransactionID string `json:"originalTransactionId"`
	BundleID              string `json:"bundleId"`
	ProductID             string `json:"productId"`
	Type                  string `json:"type"`
	Environment           string `json:"environment"`
	AppAccountToken       string `json:"appAccountToken,omitempty"`
	PurchaseDate          int64  `json:"purchaseDate"`
	ExpiresDate           int64  `json:"expiresDate,omitempty"`
	RevocationDate        int64  `json:"revocationDate,omitempty"`
	SignedDate            int64  `json:"signedDate"`
}

// Monthly is an active one-month subscription bought now.
func Monthly(bundleID, productID, otid string, now time.Time) Transaction {
	return Transaction{
		TransactionID:         otid + "-1",
		OriginalTransactionID: otid,
		BundleID:              bundleID,
		ProductID:             productID,
		Type:                  "Auto-Renewable Subscription",
		Environment:           "Sandbox",
		PurchaseDate:          now.UnixMilli(),
		ExpiresDate:           now.AddDate(0, 1, 0).UnixMilli(),
		SignedDate:            now.UnixMilli(),
	}
}

// Consumable is a consumable purchase (a coin pack, #630) bought now.
func Consumable(bundleID, productID, txid string, now time.Time) Transaction {
	return Transaction{
		TransactionID:         txid,
		OriginalTransactionID: txid,
		BundleID:              bundleID,
		ProductID:             productID,
		Type:                  "Consumable",
		Environment:           "Sandbox",
		PurchaseDate:          now.UnixMilli(),
		SignedDate:            now.UnixMilli(),
	}
}
