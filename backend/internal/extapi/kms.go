package extapi

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"

	cloudkms "google.golang.org/api/cloudkms/v1"
	"google.golang.org/api/option"
)

// kmsKeyRe is a Cloud KMS key name (no version: encrypt uses the primary).
var kmsKeyRe = regexp.MustCompile(`^projects/[^/]+/locations/[^/]+/keyRings/[^/]+/cryptoKeys/[^/]+$`)

// KMSSealer seals tokens with a Cloud KMS symmetric key
// (EXT_TOKEN_KMS_KEY). Only the runner's SA has
// roles/cloudkms.cryptoKeyEncrypterDecrypter on the key (infra, a human's
// step), so appconfig and the clients cannot open a token even when they
// can read the document. A token is far below the 64 KiB a direct
// encrypt takes, so no data key is kept.
type KMSSealer struct {
	keys *cloudkms.ProjectsLocationsKeyRingsCryptoKeysService
	key  string
}

// NewKMSSealer uses the default credentials (the runner's SA on Cloud Run).
// opts go to the client (tests: option.WithEndpoint).
func NewKMSSealer(ctx context.Context, key string, opts ...option.ClientOption) (*KMSSealer, error) {
	if !kmsKeyRe.MatchString(key) {
		return nil, fmt.Errorf("EXT_TOKEN_KMS_KEY must be projects/…/locations/…/keyRings/…/cryptoKeys/…, got %q", key)
	}
	svc, err := cloudkms.NewService(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return &KMSSealer{keys: svc.Projects.Locations.KeyRings.CryptoKeys, key: key}, nil
}

// Seal implements Sealer.
func (s *KMSSealer) Seal(ctx context.Context, plaintext, aad []byte) ([]byte, error) {
	enc := base64.StdEncoding
	resp, err := s.keys.Encrypt(s.key, &cloudkms.EncryptRequest{
		Plaintext: enc.EncodeToString(plaintext), AdditionalAuthenticatedData: enc.EncodeToString(aad),
	}).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("kms encrypt: %w", err)
	}
	return enc.DecodeString(resp.Ciphertext)
}

// Open implements Sealer.
func (s *KMSSealer) Open(ctx context.Context, ciphertext, aad []byte) ([]byte, error) {
	if len(ciphertext) == 0 {
		return nil, errors.New("extapi: empty sealed value")
	}
	enc := base64.StdEncoding
	resp, err := s.keys.Decrypt(s.key, &cloudkms.DecryptRequest{
		Ciphertext: enc.EncodeToString(ciphertext), AdditionalAuthenticatedData: enc.EncodeToString(aad),
	}).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("kms decrypt: %w", err)
	}
	return enc.DecodeString(resp.Plaintext)
}
