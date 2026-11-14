package dsl

// checkAIInputPhoto: aiButton inputPhoto sends one photo of the form draft
// with the call (read a prescription, a label, a whiteboard). Only the cloud
// AI (Pro) reads it, so it needs screen.entity and a photo field of that
// entity; the iOS app downscales it before sending (/v1/text image).
func checkAIInputPhoto(where string, c *Component, form *Entity, add func(string, ...any)) {
	if c.InputPhoto == "" {
		return
	}
	if form == nil {
		add("%s: aiButton inputPhoto needs screen.entity", where)
		return
	}
	switch f := form.FieldByID(c.InputPhoto); {
	case f == nil:
		add("%s: aiButton inputPhoto %q not in entity %q", where, c.InputPhoto, form.ID)
	case f.Type != FieldPhoto:
		add("%s: aiButton inputPhoto %q must be a photo field (got %s)", where, c.InputPhoto, f.Type)
	}
}
