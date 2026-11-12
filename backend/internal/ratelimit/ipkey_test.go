package ratelimit

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

// Every spelling of one /64 shares a key, including "::" inside the prefix.
func TestIPKeyGroupsAnySpellingOf64(t *testing.T) {
	same := [][2]string{
		{"2001:db8::1", "2001:db8::2"},                    // zero prefix groups compressed
		{"2400::1:2:3:4", "2400::1:9:3:4"},                // "::" in the prefix, IID spelled out
		{"2001:0DB8:0001:0002::1", "2001:db8:1:2:ffff::"}, // case and leading zeros
		{"::ffff:203.0.113.7", "203.0.113.7"},             // IPv4-mapped counts as IPv4
	}
	for _, p := range same {
		if ipKey(p[0]) != ipKey(p[1]) {
			t.Errorf("%s and %s must share a key", p[0], p[1])
		}
	}
	if ipKey("2001:db8::1") == ipKey("2001:db8:0:1::1") {
		t.Error("different /64 must differ")
	}
	if ipKey("203.0.113.7") == ipKey("203.0.113.8") {
		t.Error("IPv4 is not grouped")
	}
}

// Canonical addresses keep the key they had before the /64 was parsed, so
// live counters are not reset by the change.
func TestIPKeyStableForCanonicalAddresses(t *testing.T) {
	hash := func(s string) string {
		sum := sha256.Sum256([]byte(s))
		return hex.EncodeToString(sum[:8])
	}
	for ip, hashed := range map[string]string{
		"203.0.113.7":            "203.0.113.7",
		"2400:4050:a1b2:c300::1": "2400:4050:a1b2:c300::/64",
		"2001:db8:0:1:2::9":      "2001:db8:0:1::/64",
	} {
		if got := ipKey(ip); got != hash(hashed) {
			t.Errorf("ipKey(%s) = %s, want hash of %s", ip, got, hashed)
		}
	}
}

func TestIPKeyKeepsNoRawAddress(t *testing.T) {
	if IPKey("") != "" {
		t.Fatal("no address, no key")
	}
	for _, ip := range []string{"203.0.113.7", "2001:db8::1", "not-an-ip", strings.Repeat("a", 4096)} {
		k := IPKey(ip)
		if len(k) != 16 || strings.Contains(k, ip) || k != ipKey(ip) {
			t.Errorf("IPKey(%.20q) = %q", ip, k)
		}
	}
}
