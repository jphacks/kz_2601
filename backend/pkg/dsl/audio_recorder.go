package dsl

// audioRecorder (#420): records the microphone into a stored audio field.
// The clip stays on the recording device (AAC m4a); the record only keeps
// "audio:<id>|<seconds>", like video (#206).
const (
	FieldAudio        FieldType     = "audio"
	CompAudioRecorder ComponentType = "audioRecorder"
)

// AudioMaxDuration bounds audioRecorder.maxDuration in seconds (0 = default 600).
const AudioMaxDuration = 7200

func checkAudioRecorder(where string, c *Component, formEntity *Entity, add func(string, ...any)) {
	if c.MaxDuration < 0 || c.MaxDuration > AudioMaxDuration {
		add("%s: audioRecorder maxDuration must be 1..%d seconds", where, AudioMaxDuration)
	}
	if formEntity == nil {
		add("%s: audioRecorder requires screen.entity", where)
		return
	}
	if f := formEntity.FieldByID(c.Field); f == nil {
		add("%s: field %q not in entity %q (fields: %s)", where, c.Field, formEntity.ID, fieldIDs(formEntity))
	} else if f.Type != FieldAudio || f.Computed != nil {
		add("%s: audioRecorder field %q must be a stored audio field", where, c.Field)
	}
}
