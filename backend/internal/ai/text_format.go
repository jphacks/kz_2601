package ai

import (
	"slices"
	"strings"
	"unicode/utf8"
)

// Formats of shape fields (aiButton outputs): how an item should be written
// so the app can put it into a number / date / enum field. The device sends
// the field kinds and the server writes the hint into the user turn, outside
// <instruction>: the instruction stays the button prompt, which is what the
// creator-paid scope matches.

// TextFormat is the requested form of one fields item.
type TextFormat struct {
	Item    string   `json:"item"`
	Kind    string   `json:"kind"`              // number | date | datetime | enum
	Options []string `json:"options,omitempty"` // enum: the choices
}

var textFormatKinds = map[string]bool{"number": true, "date": true, "datetime": true, "enum": true}

// normalizeTextFormats checks req.Formats (generate only; the caller rejects them elsewhere).
func normalizeTextFormats(req *TextRequest) error {
	if len(req.Formats) == 0 {
		return nil
	}
	if req.Shape != "fields" {
		return badText("formats are only for shape fields")
	}
	seen := map[string]bool{}
	for i := range req.Formats {
		f := &req.Formats[i]
		f.Item = strings.TrimSpace(f.Item)
		if !slices.Contains(req.Items, f.Item) || seen[f.Item] {
			return badText("formats must name each item at most once")
		}
		seen[f.Item] = true
		if !textFormatKinds[f.Kind] {
			return badText("format kind must be number, date, datetime or enum")
		}
		if f.Kind != "enum" {
			if len(f.Options) > 0 {
				return badText("format options are only for kind enum")
			}
			continue
		}
		if len(f.Options) == 0 || len(f.Options) > TextMaxOptions {
			return badText("format enum needs 1..20 options")
		}
		for k, o := range f.Options {
			o = strings.TrimSpace(o)
			if o == "" || utf8.RuneCountInString(o) > TextMaxOptionRunes {
				return badText("format options must be non-empty strings of at most 40 characters")
			}
			f.Options[k] = o
		}
	}
	return nil
}

// textFormatHint is the line of the user turn that asks for the formats ("" = none).
func textFormatHint(req TextRequest, loc Locale) string {
	if len(req.Formats) == 0 {
		return ""
	}
	en := loc == LocaleEN
	pick := func(e, ja string) string {
		if en {
			return e
		}
		return ja
	}
	parts := make([]string, 0, len(req.Formats))
	for _, f := range req.Formats {
		var s string
		switch f.Kind {
		case "number":
			s = pick("number only", "数字だけ")
		case "date":
			s = pick("YYYY/M/D (M/D if no year)", "YYYY/M/D(年が無ければ M/D)")
		case "datetime":
			s = pick("YYYY/M/D H:mm (M/D H:mm if no year)", "YYYY/M/D H:mm(年が無ければ M/D H:mm)")
		case "enum":
			s = strings.Join(f.Options, "/") + pick(" (one of these)", " のどれか")
		}
		parts = append(parts, f.Item+"="+s)
	}
	return pick("Format: ", "書き方: ") + strings.Join(parts, " / ") + pick(" (unknown: -)", "(分からなければ -)") + "\n"
}
