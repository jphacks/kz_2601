package ai

import (
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func TestLocaleText2591(t *testing.T) {
	cases := []struct {
		name string
		l    Locale
		want summaryText
	}{
		{"ja", LocaleJA, summaryTexts[LocaleJA]},
		{"en", LocaleEN, summaryTexts[LocaleEN]},
		{"unknown falls back to ja", Locale("fr"), summaryTexts[LocaleJA]},
		{"empty falls back to ja", Locale(""), summaryTexts[LocaleJA]},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.l.text(); got != c.want {
				t.Errorf("text() = %+v, want %+v", got, c.want)
			}
		})
	}
	if summaryTexts[LocaleEN].nothingToUndo != "There is nothing to undo." {
		t.Errorf("en nothingToUndo = %q", summaryTexts[LocaleEN].nothingToUndo)
	}
}

func TestAddButtonEN2591(t *testing.T) {
	cases := []struct {
		name string
		c    dsl.Component
		want string
	}{
		{"bottom right", dsl.Component{Area: "E11:F12", Label: "Add"}, "the button at the bottom right"},
		{"top left", dsl.Component{Area: "A1:B2"}, "the button at the top left"},
		{"bottom full width", dsl.Component{Area: "A10:F12"}, "the button at the bottom"},
		{"middle uses label", dsl.Component{Area: "A5:F8", Label: "Add expense"}, "the Add expense button"},
		{"no area trims label", dsl.Component{Label: "  Add item  "}, "the Add item button"},
		{"bad area uses label", dsl.Component{Area: "Z9", Label: "Add"}, "the Add button"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := addButtonEN(&c.c); got != c.want {
				t.Errorf("addButtonEN = %q, want %q", got, c.want)
			}
		})
	}
}

// The add hint is phrased with the entity name in the plural.
func TestPluralEntityEN2591(t *testing.T) {
	cases := []struct{ in, want string }{
		{"expense", "expenses"},
		{"bus", "buses"},
		{"entry", "entries"},
		{"day", "days"},
		{"box", "boxes"},
		{"match", "matches"},
		{"spending", "spending"},
		{"pocket money", "pocket money"},
		{"", ""},
	}
	for _, c := range cases {
		if got := pluralEN(c.in); got != c.want {
			t.Errorf("pluralEN(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestAmpm2591(t *testing.T) {
	cases := []struct {
		hh   int
		want string
	}{
		{0, " am"},
		{11, " am"},
		{12, " pm"},
		{13, " pm"},
		{23, " pm"},
	}
	for _, c := range cases {
		if got := ampm(c.hh); got != c.want {
			t.Errorf("ampm(%d) = %q, want %q", c.hh, got, c.want)
		}
	}
}

func TestNotificationWhenENClock2591(t *testing.T) {
	cases := []struct {
		n    dsl.Notification
		want string
	}{
		{dsl.Notification{Time: "00:00"}, "at 12"},
		{dsl.Notification{Time: "12:00", Repeat: "daily"}, "every day at 12 pm"},
		{dsl.Notification{Time: "23:15", Repeat: "daily"}, "every evening at 11:15"},
	}
	for _, c := range cases {
		if got := notificationWhenEN(c.n); got != c.want {
			t.Errorf("notificationWhenEN(%+v) = %q, want %q", c.n, got, c.want)
		}
	}
}
