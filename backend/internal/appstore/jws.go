// Package appstore verifies StoreKit 2 signed transactions (JWSTransaction)
// locally, without calling Apple (#314): the x5c chain must end at the pinned
// Apple Root CA - G3, the payload must be signed ES256 by the leaf, and the
// bundle id / product id / environment must be allowed.
//
// The same JWS format is used by App Store Server Notifications v2 and the
// App Store Server API (#315), so those can reuse Verifier.Verify.
package appstore

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"time"
)

// Environments in JWSTransaction.environment.
const (
	EnvSandbox    = "Sandbox"
	EnvProduction = "Production"
)

// Reasons a JWS is rejected (Error.Reason; also the 422 details.reason).
const (
	ReasonMalformed       = "malformed"
	ReasonChain           = "chain"
	ReasonSignature       = "signature"
	ReasonBundle          = "bundle_id"
	ReasonProduct         = "product_id"
	ReasonEnvironment     = "environment"
	ReasonNotSubscription = "not_subscription"
	ReasonNotConsumable   = "not_consumable"
	ReasonRevoked         = "revoked"
	// ReasonAccount: appAccountToken is not the caller's (AccountToken).
	ReasonAccount = "account"
)

// AccountToken is the StoreKit appAccountToken iOS sets for uid: a UUIDv8
// from SHA-256 (Firebase uids are not UUIDs). ios: ProAccountToken.swift.
func AccountToken(uid string) string {
	b := sha256.Sum256([]byte("tailorit:appAccountToken:" + uid))
	b[6] = b[6]&0x0f | 0x80 // version 8
	b[8] = b[8]&0x3f | 0x80 // RFC 9562 variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// TypeConsumable is JWSTransaction.type of a consumable in-app purchase (coin packs, #630).
const TypeConsumable = "Consumable"

// Apple marker extensions (Apple's app-store-server-library checks them too).
var (
	oidLeafMarker         = asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 6, 11, 1}
	oidIntermediateMarker = asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 6, 2, 1}
)

// Error is a rejected JWS.
type Error struct {
	Reason string
	Err    error
}

func (e *Error) Error() string { return "appstore: " + e.Reason + ": " + e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

func reject(reason, format string, a ...any) error {
	return &Error{Reason: reason, Err: fmt.Errorf(format, a...)}
}

// Transaction is the verified payload (the fields Tailor It uses).
type Transaction struct {
	TransactionID         string
	OriginalTransactionID string
	BundleID              string
	ProductID             string
	Type                  string
	Environment           string
	AppAccountToken       string
	PurchaseDate          time.Time
	ExpiresDate           time.Time // zero when absent
	RevocationDate        time.Time // zero unless refunded / revoked
	SignedDate            time.Time
}

// Revoked reports whether Apple revoked the transaction (refund, family sharing removal).
func (t *Transaction) Revoked() bool { return !t.RevocationDate.IsZero() }

// Policy is what a verified transaction must match.
type Policy struct {
	BundleIDs    []string
	ProductIDs   []string
	Environments []string
}

// Verifier checks JWS transactions against Roots and Policy.
type Verifier struct {
	// Roots trusted for the x5c chain; nil = AppleRoots(). Tests pass a fake CA.
	Roots *x509.CertPool
	Policy
}

type header struct {
	Alg string   `json:"alg"`
	X5c []string `json:"x5c"`
}

type payload struct {
	TransactionID         string `json:"transactionId"`
	OriginalTransactionID string `json:"originalTransactionId"`
	BundleID              string `json:"bundleId"`
	ProductID             string `json:"productId"`
	Type                  string `json:"type"`
	Environment           string `json:"environment"`
	AppAccountToken       string `json:"appAccountToken"`
	PurchaseDate          int64  `json:"purchaseDate"`
	ExpiresDate           int64  `json:"expiresDate"`
	RevocationDate        int64  `json:"revocationDate"`
	SignedDate            int64  `json:"signedDate"`
}

// Verify checks signedJWS and returns its transaction. Errors are *Error.
//
// The chain is validated at the payload's signedDate (as Apple's library does
// without online checks), so a transaction signed while the leaf was valid
// stays verifiable. Expiry and revocation are not errors: they are reported in
// the Transaction for the caller to store.
func (v *Verifier) Verify(signedJWS string) (*Transaction, error) {
	t, err := v.verifyTransaction(signedJWS)
	if err != nil {
		return nil, err
	}
	if t.ExpiresDate.IsZero() {
		// Pro is sold only as an auto-renewable subscription
		return nil, reject(ReasonNotSubscription, "no expiresDate (type %q)", t.Type)
	}
	return t, nil
}

// VerifyConsumable is Verify for a consumable purchase (coin packs, #630):
// the same chain, signature and bundle / product / environment checks, but
// the type must be Consumable (no expiresDate). A revoked (refunded)
// consumable is ReasonRevoked: it must not be credited.
func (v *Verifier) VerifyConsumable(signedJWS string) (*Transaction, error) {
	t, err := v.verifyTransaction(signedJWS)
	if err != nil {
		return nil, err
	}
	if t.Type != TypeConsumable || !t.ExpiresDate.IsZero() {
		return nil, reject(ReasonNotConsumable, "type %q", t.Type)
	}
	if t.Revoked() {
		return nil, reject(ReasonRevoked, "revoked at %s", t.RevocationDate)
	}
	return t, nil
}

func (v *Verifier) verifyTransaction(signedJWS string) (*Transaction, error) {
	var p payload
	signedAt, err := v.verifyJWS(signedJWS, &p)
	if err != nil {
		return nil, err
	}

	t := &Transaction{
		TransactionID:         p.TransactionID,
		OriginalTransactionID: p.OriginalTransactionID,
		BundleID:              p.BundleID,
		ProductID:             p.ProductID,
		Type:                  p.Type,
		Environment:           p.Environment,
		AppAccountToken:       p.AppAccountToken,
		PurchaseDate:          millis(p.PurchaseDate),
		ExpiresDate:           millis(p.ExpiresDate),
		RevocationDate:        millis(p.RevocationDate),
		SignedDate:            signedAt.UTC(),
	}
	if t.OriginalTransactionID == "" || t.TransactionID == "" {
		return nil, reject(ReasonMalformed, "missing transaction ids")
	}
	if !slices.Contains(v.BundleIDs, t.BundleID) {
		return nil, reject(ReasonBundle, "bundleId %q not allowed", t.BundleID)
	}
	if !slices.Contains(v.ProductIDs, t.ProductID) {
		return nil, reject(ReasonProduct, "productId %q not allowed", t.ProductID)
	}
	if !slices.Contains(v.Environments, t.Environment) {
		return nil, reject(ReasonEnvironment, "environment %q not allowed", t.Environment)
	}
	return t, nil
}

// verifyJWS checks the compact JWS (ES256, x5c chain up to the trusted roots
// at the payload's signedDate, signature) and decodes its payload into out.
// Shared by transactions, renewal infos and notifications (#315).
func (v *Verifier) verifyJWS(signedJWS string, out any) (time.Time, error) {
	parts := strings.Split(strings.TrimSpace(signedJWS), ".")
	if len(parts) != 3 {
		return time.Time{}, reject(ReasonMalformed, "want 3 segments, got %d", len(parts))
	}
	var h header
	if err := decodeSegment(parts[0], &h); err != nil {
		return time.Time{}, reject(ReasonMalformed, "header: %v", err)
	}
	if h.Alg != "ES256" {
		return time.Time{}, reject(ReasonSignature, "alg %q", h.Alg)
	}
	var sd struct {
		SignedDate int64 `json:"signedDate"`
	}
	if err := decodeSegment(parts[1], &sd); err != nil {
		return time.Time{}, reject(ReasonMalformed, "payload: %v", err)
	}
	if err := decodeSegment(parts[1], out); err != nil {
		return time.Time{}, reject(ReasonMalformed, "payload: %v", err)
	}
	if sd.SignedDate <= 0 {
		return time.Time{}, reject(ReasonMalformed, "no signedDate")
	}
	signedAt := time.UnixMilli(sd.SignedDate)

	leaf, err := v.verifyChain(h.X5c, signedAt)
	if err != nil {
		return time.Time{}, err
	}
	if err := verifyES256(leaf, parts[0]+"."+parts[1], parts[2]); err != nil {
		return time.Time{}, err
	}
	return signedAt, nil
}

// verifyChain checks x5c = [leaf, intermediate, root] up to the trusted roots
// and returns the leaf. The root sent in x5c is not trusted by itself.
func (v *Verifier) verifyChain(x5c []string, at time.Time) (*x509.Certificate, error) {
	if len(x5c) != 3 {
		return nil, reject(ReasonChain, "x5c has %d certificates, want 3", len(x5c))
	}
	certs := make([]*x509.Certificate, len(x5c))
	for i, s := range x5c {
		der, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return nil, reject(ReasonChain, "x5c[%d]: %v", i, err)
		}
		if certs[i], err = x509.ParseCertificate(der); err != nil {
			return nil, reject(ReasonChain, "x5c[%d]: %v", i, err)
		}
	}
	leaf, inter := certs[0], certs[1]
	if !hasExtension(leaf, oidLeafMarker) {
		return nil, reject(ReasonChain, "leaf lacks the Apple marker extension")
	}
	if !hasExtension(inter, oidIntermediateMarker) {
		return nil, reject(ReasonChain, "intermediate lacks the Apple marker extension")
	}
	roots := v.Roots
	if roots == nil {
		roots = AppleRoots()
	}
	inters := x509.NewCertPool()
	inters.AddCert(inter)
	_, err := leaf.Verify(x509.VerifyOptions{
		Roots:         roots,
		Intermediates: inters,
		CurrentTime:   at,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	})
	if err != nil {
		return nil, reject(ReasonChain, "%v", err)
	}
	return leaf, nil
}

// verifyES256 checks the JWS signature (raw r||s, RFC 7518 §3.4).
func verifyES256(leaf *x509.Certificate, signingInput, sig string) error {
	pub, ok := leaf.PublicKey.(*ecdsa.PublicKey)
	if !ok || pub.Curve != elliptic.P256() {
		return reject(ReasonSignature, "leaf key is not ECDSA P-256")
	}
	raw, err := decodeB64URL(sig)
	if err != nil || len(raw) != 64 {
		return reject(ReasonSignature, "signature is not 64 bytes")
	}
	sum := sha256.Sum256([]byte(signingInput))
	r, s := new(big.Int).SetBytes(raw[:32]), new(big.Int).SetBytes(raw[32:])
	if !ecdsa.Verify(pub, sum[:], r, s) {
		return reject(ReasonSignature, "signature mismatch")
	}
	return nil
}

func hasExtension(c *x509.Certificate, oid asn1.ObjectIdentifier) bool {
	for _, e := range c.Extensions {
		if e.Id.Equal(oid) {
			return true
		}
	}
	return false
}

func decodeSegment(seg string, v any) error {
	raw, err := decodeB64URL(seg)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	return dec.Decode(v)
}

func decodeB64URL(s string) ([]byte, error) {
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
	if err != nil {
		return nil, errors.New("bad base64url")
	}
	return b, nil
}

func millis(ms int64) time.Time {
	if ms <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms).UTC()
}
