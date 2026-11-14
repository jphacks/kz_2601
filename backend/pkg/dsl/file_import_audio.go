package dsl

import "slices"

// fileImport accept audio: a recording picked in Files (m4a / mp3 / wav /
// aiff / caf, e.g. a lecture or meeting) is transcribed on the device with
// the same model as voice input (WhisperKit) into field, and one record is
// added (titleField gets the file name, like text / pdf). The import sends
// neither the audio nor the transcript anywhere; only the added record syncs
// as usual. iOS caps a file at MaxImportAudioMinutes and MaxImportAudioMB,
// shows progress and can cancel; it runs only while the app is in front.

// MaxImportAudioMinutes / MaxImportAudioMB bound one audio file (iOS).
const (
	MaxImportAudioMinutes = 30
	MaxImportAudioMB      = 200
)

// checkImportAudio: a transcript of up to 30 minutes (about 9000 Japanese
// characters) needs a text field; string (4000) would cut most of it.
func checkImportAudio(where string, c *Component, e *Entity, add func(string, ...any)) {
	if !slices.Contains(c.Accept, "audio") {
		return
	}
	if f := e.FieldByID(c.Field); f != nil && f.Computed == nil && f.Type == FieldString {
		add("%s: fileImport accept audio puts a transcript of up to %d minutes into field %q; make it type text (string holds only 4000 characters)", where, MaxImportAudioMinutes, f.ID)
	}
}
