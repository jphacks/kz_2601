package dsl

import (
	"fmt"
	"slices"
)

// lookupBook: fill the form from an ISBN. The button sits on a form screen
// (screen.entity); iOS reads bookLookup.isbnField from the draft, asks
// appconfig GET /v1/lookup/book (internal/books: one allowlisted key-less
// source) and writes the result into the mapped fields of the same draft.
// The DSL names only field ids, never a URL or a source.
// docs/backend/features/book-lookup.md

// BookLookup maps the looked-up book onto fields of the form's entity.
type BookLookup struct {
	ISBNField          string `json:"isbnField"`                    // string: what the user typed / scanned
	TitleField         string `json:"titleField,omitempty"`         // string / text
	AuthorsField       string `json:"authorsField,omitempty"`       // string / text
	PublisherField     string `json:"publisherField,omitempty"`     // string / text
	PublishedDateField string `json:"publishedDateField,omitempty"` // string / text / date
}

var bookTextTypes = []FieldType{FieldString, FieldText}

// checkBookLookups: lookupBook only runs from a button / toolbar of a form
// screen (it fills that form's draft), and its fields belong to that entity.
func checkBookLookups(a *App, entities map[string]*Entity, add func(string, ...any)) {
	for _, s := range a.Screens {
		form := entities[s.Entity]
		for i := range s.Toolbar {
			checkBookLookup(fmt.Sprintf("screen %q toolbar[%d]", s.ID, i), &s.Toolbar[i].Action, s.ID, form, add)
		}
		eachScreenComponent(s.Components, func(where string, c *Component) {
			where = fmt.Sprintf("screen %q %s", s.ID, where)
			checkBookLookup(where, c.Action, s.ID, form, add)
			taps := map[string]*Action{".onItemTap": c.OnItemTap, ".onDateTap": c.OnDateTap}
			if c.Threshold != nil {
				taps[".threshold.action"] = c.Threshold.Action
			}
			for suffix, act := range taps {
				if act != nil && act.Type == ActionLookupBook {
					add("%s%s: lookupBook fills the form being edited; put it on a button of the form screen (screen.entity), not on a tap", where, suffix)
				}
			}
		})
	}
}

func checkBookLookup(where string, act *Action, screen string, form *Entity, add func(string, ...any)) {
	if act == nil || act.Type != ActionLookupBook {
		return
	}
	if form == nil {
		add("%s: lookupBook needs a form screen; set screen %q entity to the book entity (it fills that form)", where, screen)
		return
	}
	b := act.BookLookup
	if b == nil {
		add("%s: lookupBook needs bookLookup{isbnField, titleField, ...}", where)
		return
	}
	field := func(key, id string, types []FieldType, required bool) {
		if id == "" {
			if required {
				add("%s: lookupBook bookLookup.%s is required (a string field of %q)", where, key, form.ID)
			}
			return
		}
		f := form.FieldByID(id)
		switch {
		case f == nil:
			add("%s: lookupBook bookLookup.%s %q not in entity %q (fields: %s)", where, key, id, form.ID, fieldIDs(form))
		case f.Computed != nil:
			add("%s: lookupBook bookLookup.%s %q is computed; map a stored field", where, key, id)
		case !slices.Contains(types, f.Type):
			add("%s: lookupBook bookLookup.%s %q is %s; use a %v field", where, key, id, f.Type, types)
		}
	}
	field("isbnField", b.ISBNField, []FieldType{FieldString}, true)
	field("titleField", b.TitleField, bookTextTypes, false)
	field("authorsField", b.AuthorsField, bookTextTypes, false)
	field("publisherField", b.PublisherField, bookTextTypes, false)
	field("publishedDateField", b.PublishedDateField, []FieldType{FieldString, FieldText, FieldDate}, false)
	if b.TitleField == "" && b.AuthorsField == "" && b.PublisherField == "" && b.PublishedDateField == "" {
		add("%s: lookupBook bookLookup maps no field; set titleField (and authorsField / publisherField / publishedDateField when the entity has them)", where)
	}
	targets := []string{b.TitleField, b.AuthorsField, b.PublisherField, b.PublishedDateField}
	for i, id := range targets {
		if id == "" {
			continue
		}
		if id == b.ISBNField || slices.Contains(targets[i+1:], id) {
			add("%s: lookupBook bookLookup maps field %q twice; give each value its own field", where, id)
		}
	}
}
