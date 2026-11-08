package dsl

// Blank list titles: a list / timeline row whose title is an optional
// free-text field (kakeibo title = memo) shows an empty bold line for every
// record saved without it (voice 「500円 食費」). iOS ListRow falls back to
// displayTitleField only when item.title is unset, not when the value is
// empty. When the entity has a required string / enum field, that one is the
// title and the optional text goes to the subtitle.

func (l *linter) optionalTitle() {
	for _, s := range l.app.Screens {
		eachScreenComponent(s.Components, func(where string, c *Component) {
			title, key := listTitleField(c)
			e := l.entities[c.Entity]
			if title == "" || e == nil {
				return
			}
			f := e.FieldByID(title)
			if f == nil || (f.Type != FieldString && f.Type != FieldText) || f.Required || f.DefaultValue != nil || f.Computed != nil {
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

// requiredLabelField: the first required, user-entered string / enum field
// other than skip ("" if none).
func requiredLabelField(e *Entity, skip string) string {
	for _, f := range e.Fields {
		if f.ID != skip && f.Required && f.Computed == nil && !f.Hidden && (f.Type == FieldString || f.Type == FieldEnum) {
			return f.ID
		}
	}
	return ""
}
