package dsl

// drawingField: draw with a finger or Apple Pencil (PencilKit) into a stored
// photo field. The drawing is saved as a picture in the same form as a photo
// ("data:image/jpeg;base64,..." <=800px), so lists, image, share and the
// privacy label treat it as a photo. No new field type.
const CompDrawingField ComponentType = "drawingField"

func checkDrawingField(where string, c *Component, formEntity *Entity, add func(string, ...any)) {
	if formEntity == nil {
		add("%s: drawingField requires screen.entity", where)
		return
	}
	if f := formEntity.FieldByID(c.Field); f == nil {
		add("%s: field %q not in entity %q (fields: %s)", where, c.Field, formEntity.ID, fieldIDs(formEntity))
	} else if f.Type != FieldPhoto || f.Computed != nil {
		add("%s: drawingField field %q must be a stored photo field (the drawing is saved as a picture)", where, c.Field)
	}
}
