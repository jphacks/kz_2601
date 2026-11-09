package dsl

import "strings"

// Blank list titles: a list / timeline row whose title is an optional
// free-text field (kakeibo title = memo) shows an empty bold line for every
// record saved without it (voice 「500円 食費」). iOS ListRow falls back to
// displayTitleField only when item.title is unset, not when the value is
// empty. When the entity has a required string / enum (or ref) field, that
// one is the title and the optional text goes to the subtitle.

func (l *linter) optionalTitle() {
	for _, s := range l.app.Screens {
		eachScreenComponent(s.Components, func(where string, c *Component) {
			title, key := listTitleField(c)
			e := l.entities[c.Entity]
			if title == "" || e == nil {
				return
			}
			f := e.FieldByID(title)
			if f == nil || (f.Type != FieldString && f.Type != FieldText) || f.Required || hasDefault(f) || f.Computed != nil {
				return
			}
			alt := requiredLabelField(e, title)
			if alt == "" {
				return
			}
			l.add("screen %q %s: %s %s %q is an optional field, so a record saved without it shows a blank title; set the title to the required field %q and move %q to the subtitle",
				s.ID, where, c.Type, key, title, alt, title)
		})
	}
}

// listTitleField: the field bound to a list / timeline row's title and the
// key it is written under.
func listTitleField(c *Component) (field, key string) {
	switch c.Type {
	case CompList, CompTimeline:
	default:
		return "", ""
	}
	if c.Item != nil && c.Item.Title != "" {
		return c.Item.Title, "item.title"
	}
	if c.Type == CompTimeline && c.TitleField != "" {
		return c.TitleField, "titleField"
	}
	return "", ""
}

// hasDefault: a default that never shows blank ("" / "  " count as none).
func hasDefault(f *Field) bool {
	if s, ok := f.DefaultValue.(string); ok {
		return strings.TrimSpace(s) != ""
	}
	return f.DefaultValue != nil
}

// requiredLabelField: the first required, visible, user-entered string /
// enum field other than skip, else the first such ref (iOS shows the
// referenced record's title); "" if none. An enum whose options are all
// blank shows nothing, and refs is a list of names, not a title.
func requiredLabelField(e *Entity, skip string) string {
	ref := ""
	for _, f := range e.Fields {
		if f.ID == skip || !f.Required || f.Computed != nil || f.Hidden {
			continue
		}
		switch {
		case f.Type == FieldString, f.Type == FieldEnum && hasLabel(f.Options):
			return f.ID
		case f.Type == FieldRef && ref == "":
			ref = f.ID
		}
	}
	return ref
}

func hasLabel(options []string) bool {
	for _, o := range options {
		if strings.TrimSpace(o) != "" {
			return true
		}
	}
	return false
}
