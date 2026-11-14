package appconfig

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestNormalizeValueByType(t *testing.T) {
	enumField := &dsl.Field{ID: "e", Type: dsl.FieldEnum, Options: []string{"a", "b"}}
	cases := []struct {
		name    string
		f       *dsl.Field
		in      any
		want    any
		wantErr string
	}{
		{"string ok", &dsl.Field{ID: "s", Type: dsl.FieldString}, "hi", "hi", ""},
		{"string not string", &dsl.Field{ID: "s", Type: dsl.FieldString}, 1.0, nil, "must be a string"},
		{"string too long", &dsl.Field{ID: "s", Type: dsl.FieldString}, strings.Repeat("あ", maxStringLen+1), nil, "exceeds 4000"},
		{"text allows longer", &dsl.Field{ID: "s", Type: dsl.FieldText}, strings.Repeat("a", maxStringLen+1), strings.Repeat("a", maxStringLen+1), ""},
		{"text too long", &dsl.Field{ID: "s", Type: dsl.FieldText}, strings.Repeat("a", maxTextLen+1), nil, "exceeds 20000"},
		{"integer from float", &dsl.Field{ID: "n", Type: dsl.FieldInteger}, 3.0, int64(3), ""},
		{"integer from json.Number", &dsl.Field{ID: "n", Type: dsl.FieldInteger}, json.Number("7"), int64(7), ""},
		{"integer fraction", &dsl.Field{ID: "n", Type: dsl.FieldInteger}, 1.5, nil, "must be an integer"},
		{"integer string", &dsl.Field{ID: "n", Type: dsl.FieldInteger}, "1", nil, "must be an integer"},
		{"currency ok", &dsl.Field{ID: "n", Type: dsl.FieldCurrency}, -1200.0, int64(-1200), ""},
		{"number ok", &dsl.Field{ID: "n", Type: dsl.FieldNumber}, 1.25, 1.25, ""},
		{"number int", &dsl.Field{ID: "n", Type: dsl.FieldNumber}, 2, 2.0, ""},
		{"number bad", &dsl.Field{ID: "n", Type: dsl.FieldNumber}, "x", nil, "must be a number"},
		{"boolean ok", &dsl.Field{ID: "b", Type: dsl.FieldBoolean}, true, true, ""},
		{"boolean string", &dsl.Field{ID: "b", Type: dsl.FieldBoolean}, "true", nil, "must be a boolean"},
		{"date ok", &dsl.Field{ID: "d", Type: dsl.FieldDate}, "2026-11-12", "2026-11-12", ""},
		{"date from RFC3339", &dsl.Field{ID: "d", Type: dsl.FieldDate}, "2026-11-12T23:30:00+09:00", "2026-11-12", ""},
		{"date bad", &dsl.Field{ID: "d", Type: dsl.FieldDate}, "11/12", nil, "must be YYYY-MM-DD"},
		{"date not string", &dsl.Field{ID: "d", Type: dsl.FieldDate}, 1.0, nil, "must be a YYYY-MM-DD string"},
		{"datetime ok", &dsl.Field{ID: "d", Type: dsl.FieldDatetime}, "2026-11-12T10:00:00Z", "2026-11-12T10:00:00Z", ""},
		{"datetime from date is JST midnight", &dsl.Field{ID: "d", Type: dsl.FieldDatetime}, "2026-11-12", "2026-11-12T00:00:00+09:00", ""},
		{"datetime bad", &dsl.Field{ID: "d", Type: dsl.FieldDatetime}, "noon", nil, "must be RFC 3339"},
		{"enum ok", enumField, "a", "a", ""},
		{"enum unknown", enumField, "c", nil, "must be one of a, b"},
		{"enum without options", &dsl.Field{ID: "e", Type: dsl.FieldEnum}, "any", "any", ""},
		{"photo data url", &dsl.Field{ID: "p", Type: dsl.FieldPhoto}, "data:image/png;base64,AA", "data:image/png;base64,AA", ""},
		{"photo http", &dsl.Field{ID: "p", Type: dsl.FieldPhoto}, "http://x", nil, "data:image"},
		{"location normalized", &dsl.Field{ID: "l", Type: dsl.FieldLocation}, " 35.10 , 136.900 ", "35.1,136.9", ""},
		{"location out of range", &dsl.Field{ID: "l", Type: dsl.FieldLocation}, "91,0", nil, "within range"},
		{"location one part", &dsl.Field{ID: "l", Type: dsl.FieldLocation}, "35", nil, `must be "lat,lng"`},
		{"unsupported type", &dsl.Field{ID: "x", Type: dsl.FieldType("nope")}, "v", nil, "unsupported type"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := NormalizeValue(c.f, c.in)
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, c.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %#v, want %#v", got, c.want)
			}
		})
	}
}

// NormalizeValue is the plain door: an encrypted field is checked as its
// plain type, never as "enc:v1:..." (normalizeEncrypted is the device's).
func TestNormalizeValueIgnoresEncrypted(t *testing.T) {
	f := &dsl.Field{ID: "memo", Type: dsl.FieldString, Encrypted: true}
	got, err := NormalizeValue(f, "plain")
	if err != nil || got != "plain" {
		t.Fatalf("got %#v, %v; want plain value accepted", got, err)
	}
	if _, err := NormalizeValue(f, 1.0); err == nil {
		t.Fatal("non-string accepted for an encrypted string field")
	}
}

func TestDateOf(t *testing.T) {
	day := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }
	cases := []struct {
		name string
		in   any
		want time.Time
		ok   bool
	}{
		{"date", "2026-11-12", day(2026, 11, 12), true},
		{"datetime JST", "2026-11-12T08:00:00+09:00", day(2026, 11, 12), true},
		{"UTC evening is next JST day", "2026-11-12T15:30:00Z", day(2026, 11, 13), true},
		{"UTC before 15:00 stays", "2026-11-12T14:59:59Z", day(2026, 11, 12), true},
		{"other zone crossing back", "2026-11-13T05:00:00-05:00", day(2026, 11, 13), true},
		{"year boundary", "2026-12-31T16:00:00Z", day(2027, 1, 1), true},
		{"bad string", "tomorrow", time.Time{}, false},
		{"not string", 20261112.0, time.Time{}, false},
		{"nil", nil, time.Time{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := dateOf(c.in)
			if ok != c.ok || !got.Equal(c.want) {
				t.Fatalf("dateOf(%v) = %v, %v; want %v, %v", c.in, got, ok, c.want, c.ok)
			}
			if ok && got.Location() != time.UTC {
				t.Fatalf("location = %v, want UTC", got.Location())
			}
		})
	}
}
