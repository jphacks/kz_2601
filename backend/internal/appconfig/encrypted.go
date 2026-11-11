package appconfig

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Field.encrypted (#233, pkg/dsl/encrypted.go): iOS seals the value on the
// device; appconfig only stores the envelope "enc:v1:<base64>" and refuses a
// plain value (an old build that does not know encrypted, or a bug), so the
// plaintext never reaches Firestore. Filter / sort on it mean nothing on
// ciphertext and are refused too.

// gcmOverhead is the nonce (12) + tag (16) iOS adds (CryptoKit AES.GCM combined).
const gcmOverhead = 28

// maxEncryptedLen bounds the envelope of a value at the field's plain limit
// (4 bytes per rune at most, then base64).
func maxEncryptedLen(f *dsl.Field) int {
	limit := maxStringLen
	if f.Type == dsl.FieldText {
		limit = maxTextLen
	}
	return len(dsl.EncryptedPrefix) + base64.StdEncoding.EncodedLen(limit*4+gcmOverhead)
}

// normalizeEncrypted accepts "" or an envelope.
func normalizeEncrypted(f *dsl.Field, v any) (any, error) {
	s, ok := v.(string)
	if !ok {
		return nil, fmt.Errorf("must be an encrypted string")
	}
	if s == "" {
		return s, nil
	}
	body, ok := strings.CutPrefix(s, dsl.EncryptedPrefix)
	if !ok {
		return nil, fmt.Errorf("is encrypted: the app must send %q..., not the plain value", dsl.EncryptedPrefix)
	}
	if len(s) > maxEncryptedLen(f) {
		return nil, fmt.Errorf("exceeds %d bytes", maxEncryptedLen(f))
	}
	raw, err := base64.StdEncoding.DecodeString(body)
	if err != nil || len(raw) <= gcmOverhead {
		return nil, fmt.Errorf("is not a valid encrypted value")
	}
	return s, nil
}

func encryptedQueryField(f *dsl.Field) error {
	if f != nil && f.Encrypted {
		return apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "field is encrypted: filter and sort cannot read it").With("field", f.ID)
	}
	return nil
}
