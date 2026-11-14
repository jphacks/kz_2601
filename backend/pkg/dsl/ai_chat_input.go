package dsl

// aiChat inputEntity: the AI reads the user's own records as reference
// material ("日記を踏まえて相談" / "保存した問題について質問" / a few
// imported documents). With each turn iOS sends the newest `limit` records
// (default and max MaxAIChatInputRecords) as one text block of at most
// MaxAIChatMaterialBytes: only the user's own records (a shared / public
// entity is filtered to the records the user wrote, like the conversation),
// never hidden or encrypted fields, only the AIChatInputFieldTypes. /v1/text task chat
// puts the block into the user turn as data, not instructions (text_chat.go).

// Bounds of aiChat inputEntity; /v1/text task chat applies the same.
const (
	MaxAIChatInputRecords  = 20
	MaxAIChatMaterialBytes = 8 << 10
)

// AIChatInputFieldTypes are the field types whose values go into the
// material (media, places, references and members never do).
var AIChatInputFieldTypes = map[FieldType]bool{
	FieldString: true, FieldText: true, FieldInteger: true, FieldNumber: true, FieldBoolean: true,
	FieldDate: true, FieldDatetime: true, FieldEnum: true, FieldCurrency: true,
}

// checkAIChatInput: inputEntity exists, differs from the conversation entity
// and has a stored, non-hidden field of AIChatInputFieldTypes; limit (only
// with inputEntity) is 1..MaxAIChatInputRecords.
func checkAIChatInput(where string, c *Component, entities map[string]*Entity, add func(string, ...any)) {
	if c.InputEntity == "" {
		if c.Limit != 0 {
			add("%s: aiChat limit is the number of inputEntity records; set inputEntity or drop limit", where)
		}
		return
	}
	if c.Limit < 0 || c.Limit > MaxAIChatInputRecords {
		add("%s: aiChat limit %d must be 1..%d (the newest records the AI reads)", where, c.Limit, MaxAIChatInputRecords)
	}
	e := entities[c.InputEntity]
	switch {
	case e == nil:
		add("%s: aiChat inputEntity %q does not exist", where, c.InputEntity)
		return
	case e.ID == c.Entity:
		add("%s: aiChat inputEntity must differ from the conversation entity %q (the AI already reads the conversation)", where, e.ID)
		return
	}
	for i := range e.Fields {
		f := &e.Fields[i]
		if f.Computed == nil && !f.Hidden && !f.Encrypted && AIChatInputFieldTypes[f.Type] {
			return
		}
	}
	add("%s: aiChat inputEntity %q has no field the AI can read (string / text / number / date / enum ..., not hidden or encrypted)", where, e.ID)
}
