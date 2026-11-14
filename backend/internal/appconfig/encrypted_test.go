package appconfig

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func encryptedEntity() *dsl.Entity {
	return &dsl.Entity{ID: "diary", Fields: []dsl.Field{
		{ID: "title", Type: dsl.FieldString},
		{ID: "body", Type: dsl.FieldText, Encrypted: true},
	}}
}

// #233: only the envelope is stored; a plain value (an old build) is refused.
func TestNormalizeDataEncrypted(t *testing.T) {
	env := dsl.EncryptedPrefix + base64.StdEncoding.EncodeToString(make([]byte, 40))
	out, err := normalizeData(encryptedEntity(), map[string]any{"title": "t", "body": env}, true)
	if err != nil || out["body"] != env {
		t.Fatalf("envelope: out=%v err=%v", out, err)
	}
	if out, err := normalizeData(encryptedEntity(), map[string]any{"body": nil}, false); err != nil || out["body"] != nil {
		t.Fatalf("PATCH null clears: out=%v err=%v", out, err)
	}
	for name, v := range map[string]any{
		"plain":       "今日は疲れた",
		"bad base64":  dsl.EncryptedPrefix + "%%%",
		"too short":   dsl.EncryptedPrefix + base64.StdEncoding.EncodeToString(make([]byte, 10)),
		"not string":  42.0,
		"too long":    dsl.EncryptedPrefix + strings.Repeat("A", maxEncryptedLen(&encryptedEntity().Fields[1])),
		"plain + pfx": "enc:v2:abc",
	} {
		if _, err := normalizeData(encryptedEntity(), map[string]any{"body": v}, true); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestEncryptedFieldNotQueried(t *testing.T) {
	if _, err := buildFilter(encryptedEntity(), "body", "eq", "x"); err == nil {
		t.Fatal("filter on an encrypted field must be refused")
	}
	if _, err := buildFilter(encryptedEntity(), "title", "eq", "x"); err != nil {
		t.Fatal(err)
	}
	if encryptedQueryField(encryptedEntity().FieldByID("body")) == nil || encryptedQueryField(nil) != nil {
		t.Fatal("sort check")
	}
}
