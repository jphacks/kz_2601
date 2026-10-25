package dsl

// checkLocationPicker (#212): a form input that picks a place (search / map
// pin / current location) into a stored location field ("lat,lng", the same
// shape as locationButton) and optionally its name into a string field.
func checkLocationPicker(where string, c *Component, formEntity *Entity, add func(string, ...any)) {
	if formEntity == nil {
		add("%s: locationPicker requires screen.entity", where)
		return
	}
	if f := formEntity.FieldByID(c.Field); f == nil {
		add("%s: field %q not in entity %q (fields: %s)", where, c.Field, formEntity.ID, fieldIDs(formEntity))
	} else if f.Type != FieldLocation || f.Computed != nil {
		add("%s: locationPicker field %q must be a stored location field", where, c.Field)
	}
	if c.NameField == "" {
		return
	}
	if c.NameField == c.Field {
		add("%s: locationPicker nameField must differ from field", where)
	} else if f := formEntity.FieldByID(c.NameField); f == nil {
		add("%s: locationPicker nameField %q not in entity %q (fields: %s)", where, c.NameField, formEntity.ID, fieldIDs(formEntity))
	} else if f.Type != FieldString || f.Computed != nil {
		add("%s: locationPicker nameField %q must be a stored string field (the place name)", where, c.NameField)
	}
}
