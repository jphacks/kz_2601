package appstore_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/appstore"
	"github.com/jphacks/kz_2601/backend/internal/appstore/appstoretest"
)

const (
	bundle  = "jp.kz2601.metamon"
	product = "jp.kz2601.metamon.pro.monthly"
)

func verifier(s *appstoretest.Signer) *appstore.Verifier {
	return &appstore.Verifier{Roots: s.Roots, Policy: appstore.Policy{
		BundleIDs:    []string{bundle},
		ProductIDs:   []string{product},
		Environments: []string{appstore.EnvSandbox, appstore.EnvProduction},
	}}
}

func reason(t *testing.T, err error) string {
	t.Helper()
	var e *appstore.Error
	if !errors.As(err, &e) {
		t.Fatalf("want *appstore.Error, got %v", err)
	}
	return e.Reason
}

func TestPinnedRootIsAppleRootCAG3(t *testing.T) {
	c := appstore.AppleRootCA()
	sum := sha256.Sum256(c.Raw)
	if got := strings.ToUpper(hex.EncodeToString(sum[:])); got != "63343ABFB89A6A03EBB57E9B3F5FA7BE7C4F5C756F3017B3A8C488C3653E9179" {
		t.Fatalf("fingerprint %s", got)
	}
	if c.Subject.CommonName != "Apple Root CA - G3" {
		t.Fatalf("subject %q", c.Subject.CommonName)
	}
}

func TestVerifyValid(t *testing.T) {
	s := appstoretest.New(t, appstoretest.Options{})
	now := time.Now()
	tx := appstoretest.Monthly(bundle, product, "2000000111", now)
	tx.AppAccountToken = "6f1e2c3a-0000-8000-8000-000000000000"
	got, err := verifier(s).Verify(s.Sign(t, tx))
	if err != nil {
		t.Fatal(err)
	}
	if got.OriginalTransactionID != "2000000111" || got.ProductID != product || got.Environment != "Sandbox" ||
		got.AppAccountToken != tx.AppAccountToken || got.Revoked() {
		t.Fatalf("got %+v", got)
	}
	if !got.ExpiresDate.Equal(time.UnixMilli(tx.ExpiresDate)) {
		t.Fatalf("expires %v", got.ExpiresDate)
	}
}

func TestVerifyRejects(t *testing.T) {
	now := time.Now()
	good := appstoretest.New(t, appstoretest.Options{})
	base := appstoretest.Monthly(bundle, product, "2000000222", now)

	cases := []struct {
		name   string
		jws    func() string
		v      func() *appstore.Verifier
		reason string
	}{
		{"tampered payload", func() string {
			parts := strings.Split(good.Sign(t, base), ".")
			other := strings.Split(good.Sign(t, func() appstoretest.Transaction {
				x := base
				x.ExpiresDate = now.AddDate(10, 0, 0).UnixMilli()
				return x
			}()), ".")
			return parts[0] + "." + other[1] + "." + parts[2]
		}, nil, appstore.ReasonSignature},
		{"signed by another key", func() string {
			other := appstoretest.New(t, appstoretest.Options{})
			other.X5c = good.Chain() // claims good's chain, signs with its own leaf
			return other.Sign(t, base)
		}, nil, appstore.ReasonSignature},
		{"untrusted root", func() string {
			return appstoretest.New(t, appstoretest.Options{}).Sign(t, base)
		}, nil, appstore.ReasonChain},
		{"real Apple root does not trust fake chain", func() string { return good.Sign(t, base) },
			func() *appstore.Verifier { v := verifier(good); v.Roots = nil; return v }, appstore.ReasonChain},
		{"leaf without Apple marker", func() string {
			s := appstoretest.New(t, appstoretest.Options{OmitLeafMarker: true})
			good.Roots.AddCert(s.Root)
			return s.Sign(t, base)
		}, nil, appstore.ReasonChain},
		{"intermediate without Apple marker", func() string {
			s := appstoretest.New(t, appstoretest.Options{OmitIntermediateMarker: true})
			good.Roots.AddCert(s.Root)
			return s.Sign(t, base)
		}, nil, appstore.ReasonChain},
		{"leaf expired at signedDate", func() string {
			s := appstoretest.New(t, appstoretest.Options{NotBefore: now.Add(-48 * time.Hour), NotAfter: now.Add(-24 * time.Hour)})
			good.Roots.AddCert(s.Root)
			return s.Sign(t, base)
		}, nil, appstore.ReasonChain},
		{"two certs only", func() string {
			s := appstoretest.New(t, appstoretest.Options{})
			s.X5c = s.Chain()[:2]
			good.Roots.AddCert(s.Root)
			return s.Sign(t, base)
		}, nil, appstore.ReasonChain},
		{"wrong bundle", func() string { x := base; x.BundleID = "com.evil"; return good.Sign(t, x) }, nil, appstore.ReasonBundle},
		{"unknown product", func() string { x := base; x.ProductID = "jp.kz2601.metamon.tip"; return good.Sign(t, x) }, nil, appstore.ReasonProduct},
		{"Xcode environment", func() string { x := base; x.Environment = "Xcode"; return good.Sign(t, x) }, nil, appstore.ReasonEnvironment},
		{"sandbox not allowed", func() string { return good.Sign(t, base) },
			func() *appstore.Verifier {
				v := verifier(good)
				v.Environments = []string{appstore.EnvProduction}
				return v
			}, appstore.ReasonEnvironment},
		{"no expiresDate", func() string { x := base; x.ExpiresDate = 0; return good.Sign(t, x) }, nil, appstore.ReasonNotSubscription},
		{"garbage", func() string { return "not.a.jws" }, nil, appstore.ReasonMalformed},
		{"two segments", func() string { return "a.b" }, nil, appstore.ReasonMalformed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := verifier(good)
			if c.v != nil {
				v = c.v()
			}
			_, err := v.Verify(c.jws())
			if got := reason(t, err); got != c.reason {
				t.Fatalf("reason %q, want %q (%v)", got, c.reason, err)
			}
		})
	}
}

func TestVerifyReportsRevocationAndExpiry(t *testing.T) {
	s := appstoretest.New(t, appstoretest.Options{})
	now := time.Now()
	tx := appstoretest.Monthly(bundle, product, "2000000333", now.AddDate(0, -2, 0))
	tx.RevocationDate = now.Add(-time.Hour).UnixMilli()
	tx.SignedDate = now.UnixMilli()
	got, err := verifier(s).Verify(s.Sign(t, tx))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Revoked() || !got.ExpiresDate.Before(now) {
		t.Fatalf("got %+v", got)
	}
}

func TestAlgNone(t *testing.T) {
	s := appstoretest.New(t, appstoretest.Options{})
	jws := s.Sign(t, appstoretest.Monthly(bundle, product, "1", time.Now()))
	parts := strings.Split(jws, ".")
	// {"alg":"none"}
	if r := reason(t, func() error { _, err := verifier(s).Verify("eyJhbGciOiJub25lIn0." + parts[1] + "."); return err }()); r != appstore.ReasonSignature {
		t.Fatalf("reason %q", r)
	}
}
