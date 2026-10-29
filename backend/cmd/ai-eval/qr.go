package main

import (
	"fmt"
	"strings"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// QR code (#422) expect key: qrSource (string) = where some qrCode's value
// comes from: text (fixed) | field | value | record (field, text with
// {{fieldId}} or value with ref — reads the record on the screen) | any.
// Cases: internal/ai/eval/qr_cases.json (-cases).

var qrSources = map[string]bool{"text": true, "field": true, "value": true, "record": true, "any": true}

// qrCodeSources lists the source of every qrCode ("text", "field", "value",
// with "+record" when it reads the screen record).
func qrCodeSources(a *dsl.App) []string {
	var out []string
	for _, s := range a.Screens {
		for _, c := range s.Components {
			for _, x := range append([]dsl.Component{c}, c.Children...) {
				if x.Type != dsl.CompQrCode {
					continue
				}
				switch {
				case x.Field != "":
					out = append(out, "field+record")
				case x.Value != nil && s.Entity != "" && exprHasRef(x.Value):
					out = append(out, "value+record")
				case x.Value != nil:
					out = append(out, "value")
				case s.Entity != "" && strings.Contains(x.Text, "{{"):
					out = append(out, "text+record")
				default:
					out = append(out, "text")
				}
			}
		}
	}
	return out
}

func exprHasRef(e *dsl.Expr) bool {
	if e == nil {
		return false
	}
	if e.Kind == dsl.ExprRef {
		return true
	}
	for i := range e.Args {
		if exprHasRef(&e.Args[i]) {
			return true
		}
	}
	return exprHasRef(e.If) || exprHasRef(e.Then) || exprHasRef(e.Else)
}

func checkQR(exp map[string]any, a *dsl.App) []string {
	want, ok := exp["qrSource"].(string)
	if !ok {
		return nil
	}
	got := qrCodeSources(a)
	for _, g := range got {
		kind, record, _ := strings.Cut(g, "+")
		if want == "any" || want == kind || (want == "record" && record != "") {
			return nil
		}
	}
	return []string{fmt.Sprintf("qrSource=%v (want %s)", got, want)}
}
