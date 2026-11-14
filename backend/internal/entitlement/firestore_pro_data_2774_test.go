package entitlement

import (
	"reflect"
	"testing"
	"time"
)

func proBase2774() time.Time {
	return time.Date(2026, 3, 1, 12, 34, 56, 123456000, time.UTC)
}

func proAllFields2774() Pro {
	base := proBase2774()
	willRenew := true
	return Pro{
		ProductID:             "pro_monthly",
		OriginalTransactionID: "2000000012345678",
		TransactionID:         "2000000087654321",
		Environment:           "Production",
		ExpiresAt:             base.Add(24 * time.Hour),
		RevokedAt:             base.Add(-time.Hour),
		GraceExpiresAt:        base.Add(48 * time.Hour),
		WillRenew:             &willRenew,
		UpdatedAt:             base,
		SignedAt:              base.Add(-time.Minute),
		Platform:              PlatformGooglePlay,
	}
}

func TestProDataAllFields(t *testing.T) {
	p := proAllFields2774()
	want := map[string]any{
		"productId":             p.ProductID,
		"originalTransactionId": p.OriginalTransactionID,
		"transactionId":         p.TransactionID,
		"environment":           p.Environment,
		"expiresAt":             p.ExpiresAt,
		"updatedAt":             p.UpdatedAt,
		"revokedAt":             p.RevokedAt,
		"graceExpiresAt":        p.GraceExpiresAt,
		"willRenew":             true,
		"signedAt":              p.SignedAt,
		"platform":              PlatformGooglePlay,
	}
	if got := proData(p); !reflect.DeepEqual(got, want) {
		t.Errorf("proData = %#v, want %#v", got, want)
	}
}

func TestProDataRequiredFieldsOnly(t *testing.T) {
	base := proBase2774()
	p := Pro{
		ProductID:             "pro_yearly",
		OriginalTransactionID: "2000000000000001",
		TransactionID:         "2000000000000002",
		Environment:           "Sandbox",
		ExpiresAt:             base.Add(time.Hour),
		UpdatedAt:             base,
	}
	want := map[string]any{
		"productId":             p.ProductID,
		"originalTransactionId": p.OriginalTransactionID,
		"transactionId":         p.TransactionID,
		"environment":           p.Environment,
		"expiresAt":             p.ExpiresAt,
		"updatedAt":             p.UpdatedAt,
	}
	if got := proData(p); !reflect.DeepEqual(got, want) {
		t.Errorf("proData = %#v, want %#v", got, want)
	}
}

func TestProDataOmitsTheFieldsThatAreNotSet(t *testing.T) {
	willRenew := false
	for _, tc := range []struct {
		name    string
		edit    func(*Pro)
		missing string
	}{
		{"取り消されていない", func(p *Pro) { p.RevokedAt = time.Time{} }, "revokedAt"},
		{"猶予の期限が無い", func(p *Pro) { p.GraceExpiresAt = time.Time{} }, "graceExpiresAt"},
		{"更新の予定が分からない", func(p *Pro) { p.WillRenew = nil }, "willRenew"},
		{"署名の時刻が無い", func(p *Pro) { p.SignedAt = time.Time{} }, "signedAt"},
		{"App Store の購入", func(p *Pro) { p.Platform = "" }, "platform"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := proAllFields2774()
			tc.edit(&p)
			m := proData(p)
			if _, ok := m[tc.missing]; ok {
				t.Errorf("%s = %#v, want 欄が無い", tc.missing, m[tc.missing])
			}
		})
	}
	t.Run("更新しないと分かっている", func(t *testing.T) {
		p := proAllFields2774()
		p.WillRenew = &willRenew
		if got, ok := proData(p)["willRenew"]; !ok || got != false {
			t.Errorf("willRenew = %#v (ある: %t), want false", got, ok)
		}
	})
}

func TestStoredPrecisionTruncatesToMicroseconds(t *testing.T) {
	base := time.Date(2026, 3, 1, 12, 34, 56, 123456789, time.UTC)
	willRenew := true
	p := Pro{
		ProductID:             "pro_monthly",
		OriginalTransactionID: "2000000012345678",
		TransactionID:         "2000000087654321",
		Environment:           "Production",
		ExpiresAt:             base,
		RevokedAt:             base.Add(time.Nanosecond),
		GraceExpiresAt:        base.Add(2 * time.Nanosecond),
		WillRenew:             &willRenew,
		UpdatedAt:             base.Add(3 * time.Nanosecond),
		SignedAt:              base.Add(4 * time.Nanosecond),
		Platform:              PlatformGooglePlay,
	}
	got := storedPrecision(p)
	micro := time.Date(2026, 3, 1, 12, 34, 56, 123456000, time.UTC)
	for _, tc := range []struct {
		what string
		got  time.Time
	}{
		{"expiresAt", got.ExpiresAt},
		{"revokedAt", got.RevokedAt},
		{"graceExpiresAt", got.GraceExpiresAt},
		{"updatedAt", got.UpdatedAt},
		{"signedAt", got.SignedAt},
	} {
		if !tc.got.Equal(micro) {
			t.Errorf("%s = %s, want %s", tc.what, tc.got.Format(time.RFC3339Nano), micro.Format(time.RFC3339Nano))
		}
	}
	want := p
	want.ExpiresAt, want.RevokedAt, want.GraceExpiresAt, want.UpdatedAt, want.SignedAt = micro, micro, micro, micro, micro
	if !reflect.DeepEqual(got, want) {
		t.Errorf("storedPrecision = %#v, want %#v", got, want)
	}
}

func TestStoredPrecisionKeepsZeroTimesZero(t *testing.T) {
	p := Pro{ProductID: "pro_monthly", ExpiresAt: proBase2774(), UpdatedAt: proBase2774()}
	got := storedPrecision(p)
	for _, tc := range []struct {
		what string
		got  time.Time
	}{
		{"revokedAt", got.RevokedAt},
		{"graceExpiresAt", got.GraceExpiresAt},
		{"signedAt", got.SignedAt},
	} {
		if !tc.got.IsZero() {
			t.Errorf("%s = %s, want ゼロ値", tc.what, tc.got.Format(time.RFC3339Nano))
		}
	}
}

func TestStoredPrecisionDoesNotChangeTheCaller(t *testing.T) {
	base := time.Date(2026, 3, 1, 12, 34, 56, 123456789, time.UTC)
	p := Pro{ExpiresAt: base, UpdatedAt: base}
	_ = storedPrecision(p)
	if !p.ExpiresAt.Equal(base) || !p.UpdatedAt.Equal(base) {
		t.Errorf("呼び元の Pro = %s / %s, want %s", p.ExpiresAt.Format(time.RFC3339Nano), p.UpdatedAt.Format(time.RFC3339Nano), base.Format(time.RFC3339Nano))
	}
}
