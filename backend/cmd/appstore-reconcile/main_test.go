package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/appstore"
)

func TestServerAPIFromEnv(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	pemKey := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	policy := appstore.Policy{BundleIDs: []string{"jp.example"}}
	env := map[string]string{"APPSTORE_API_KEY_ID": "kid", "APPSTORE_API_ISSUER_ID": "iss", "APPSTORE_API_PRIVATE_KEY": pemKey}

	api, err := serverAPI(func(k string) string { return env[k] }, policy)
	if err != nil || api.KeyID != "kid" || api.IssuerID != "iss" || api.BundleID != "jp.example" || api.Key == nil {
		t.Fatalf("api %+v %v", api, err)
	}
	for _, missing := range []string{"APPSTORE_API_KEY_ID", "APPSTORE_API_ISSUER_ID", "APPSTORE_API_PRIVATE_KEY"} {
		saved := env[missing]
		env[missing] = ""
		if _, err := serverAPI(func(k string) string { return env[k] }, policy); err == nil {
			t.Fatalf("%s missing accepted", missing)
		}
		env[missing] = saved
	}
}
