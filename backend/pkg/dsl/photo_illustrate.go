package dsl

import "fmt"

// photoField.illustrate: next to camera / library, iOS offers 「イラストを作る」,
// which opens Apple Image Playground on the device (iOS 18.1+ with Apple
// Intelligence; hidden elsewhere) with the record's title as the idea. The
// picture is saved into the photo field exactly like a picked photo, so
// nothing new is stored or sent, and nothing is generated in the cloud.

func checkPhotoIllustrate(where string, c *Component, add func(string, ...any)) {
	if c.Illustrate && c.Type != CompPhotoField {
		add("%s: illustrate is only for photoField", where)
	}
}

// photoIllustrate warns when the form has no text to start the picture from
// (the sheet would open empty and the user has to type the idea).
func (l *linter) photoIllustrate() {
	for i := range l.app.Screens {
		s := &l.app.Screens[i]
		e := l.entities[s.Entity]
		if e == nil || hasIllustrateConcept(e) {
			continue
		}
		var walk func(where string, cs []Component)
		walk = func(where string, cs []Component) {
			for j := range cs {
				at := fmt.Sprintf("%s[%d]", where, j)
				walk(at+".children", cs[j].Children)
				if cs[j].Type == CompPhotoField && cs[j].Illustrate {
					l.add("screens[%s].components%s: photoField illustrate starts the picture from the record's text, but entity %q has no string/text field; add one (e.g. titleField name) or drop illustrate", s.ID, at, e.ID)
				}
			}
		}
		walk("", s.Components)
	}
}

// hasIllustrateConcept reports whether the entity has a stored, visible text
// field iOS can use as the idea (titleField first, else the first such field).
func hasIllustrateConcept(e *Entity) bool {
	for _, f := range e.Fields {
		if (f.Type == FieldString || f.Type == FieldText) && !f.Hidden && f.Computed == nil {
			return true
		}
	}
	return false
}
