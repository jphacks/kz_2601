package dsl

import (
	"regexp"
	"unicode/utf8"
)

// qrCode (#422): shows a QR code built on iOS (CoreImage, error correction M)
// from exactly one source: text (fixed, `{{fieldId}}` = the screen record),
// field (screen.entity field) or value (Expr; `ref` reads the screen record).
// Uses shared keys only: Text / Field / Value / Label (caption) / Title.

const (
	// QRCodeMaxText caps the fixed text (characters).
	QRCodeMaxText = 1000
	// qrCodeMaxBytes is the byte-mode capacity of a version 40 QR at level M.
	qrCodeMaxBytes = 2331
)

var qrPlaceholderRe = regexp.MustCompile(`\{\{([^{}]*)\}\}`)

// qrUnencodable lists field types whose value is not a short text.
var qrUnencodable = map[FieldType]bool{FieldPhoto: true, FieldVideo: true, FieldRefs: true}

func checkQRCode(where string, c *Component, formEntity *Entity, entities map[string]*Entity, add func(string, ...any)) {
	n := 0
	for _, set := range []bool{c.Text != "", c.Field != "", c.Value != nil} {
		if set {
			n++
		}
	}
	if n != 1 {
		add("%s: qrCode takes exactly one of text (fixed text / URL), field (a field of screen.entity) or value (an expression)", where)
	}
	if c.Text != "" {
		if utf8.RuneCountInString(c.Text) > QRCodeMaxText || len(c.Text) > qrCodeMaxBytes {
			add("%s: qrCode text is too long for a QR code (max %d characters / %d bytes)", where, QRCodeMaxText, qrCodeMaxBytes)
		}
		for _, m := range qrPlaceholderRe.FindAllStringSubmatch(c.Text, -1) {
			if formEntity == nil {
				add("%s: qrCode text {{%s}} requires screen.entity", where, m[1])
			} else if formEntity.FieldByID(m[1]) == nil {
				add("%s: qrCode text {{%s}} is not a field of entity %q (fields: %s)", where, m[1], formEntity.ID, fieldIDs(formEntity))
			}
		}
	}
	if c.Field != "" {
		if formEntity == nil {
			add("%s: qrCode.field requires screen.entity (use text for a fixed value)", where)
		} else if f := formEntity.FieldByID(c.Field); f == nil {
			add("%s: field %q not in entity %q (fields: %s)", where, c.Field, formEntity.ID, fieldIDs(formEntity))
		} else if qrUnencodable[f.Type] {
			add("%s: qrCode field %q is type %s; use a text-like field (string / text / number / date / enum ...)", where, c.Field, f.Type)
		} else if f.Hidden {
			add("%s: qrCode field %q is hidden; a QR code shows it to anyone — %s", where, c.Field, hiddenHint)
		}
	}
	if c.Value != nil {
		ch := &exprChecker{entities: entities, add: add}
		ch.check(where+".value", c.Value, formEntity, 1)
		checkHiddenExpr(where+".value", c.Value, formEntity, entities, false, add)
	}
}
