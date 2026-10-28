package dsl

import (
	"fmt"
	"slices"
	"strings"
)

// photoField.analyze (#418): after a photo is taken or picked, iOS reads it on
// the device (Vision OCR / image classification / body pose, plus Foundation
// Models when available) and writes each result into the draft's Into field.
// Nothing leaves the device and nothing is saved until the user saves.

// PhotoAnalyzeJoints lists the jointAngle joints (bare names = the better
// detected side).
var PhotoAnalyzeJoints = map[string]bool{
	"knee": true, "elbow": true, "hip": true, "shoulder": true,
	"leftKnee": true, "rightKnee": true, "leftElbow": true, "rightElbow": true,
	"leftHip": true, "rightHip": true, "leftShoulder": true, "rightShoulder": true,
}

// MaxPhotoAnalyze caps analyze entries per photoField.
const MaxPhotoAnalyze = 6

// photoAnalyzeInto is the field types each kind may write into.
var photoAnalyzeInto = map[string][]FieldType{
	"text":       {FieldString, FieldText},
	"amount":     {FieldCurrency, FieldInteger, FieldNumber},
	"date":       {FieldDate, FieldDatetime},
	"classify":   {FieldEnum, FieldString, FieldText},
	"describe":   {FieldString, FieldText},
	"jointAngle": {FieldNumber, FieldInteger},
}

func checkPhotoAnalyze(where string, c *Component, form *Entity, add func(string, ...any)) {
	if len(c.Analyze) == 0 || form == nil {
		return
	}
	if len(c.Analyze) > MaxPhotoAnalyze {
		add("%s: photoField analyze has at most %d entries", where, MaxPhotoAnalyze)
	}
	seen := map[string]bool{}
	for i, a := range c.Analyze {
		at := fmt.Sprintf("%s.analyze[%d]", where, i)
		types, ok := photoAnalyzeInto[a.Kind]
		if !ok {
			add("%s: kind %q must be one of text|amount|date|classify|describe|jointAngle", at, a.Kind)
			continue
		}
		if a.Joint != "" && a.Kind != "jointAngle" {
			add("%s: joint is only for kind jointAngle", at)
		}
		if a.Kind == "jointAngle" && !PhotoAnalyzeJoints[a.Joint] {
			add("%s: jointAngle needs joint (knee|elbow|hip|shoulder, or a side such as leftKnee), got %q", at, a.Joint)
		}
		if len(a.Options) > 0 && a.Kind != "classify" {
			add("%s: options are only for kind classify", at)
		}
		if a.Into == c.Field {
			add("%s: into must be another field than the photo field %q", at, c.Field)
			continue
		}
		if seen[a.Into] {
			add("%s: into %q is already used by another analyze entry", at, a.Into)
		}
		seen[a.Into] = true
		f := form.FieldByID(a.Into)
		switch {
		case f == nil:
			add("%s: into %q not in entity %q", at, a.Into, form.ID)
			continue
		case f.Computed != nil:
			add("%s: into %q is computed (read-only)", at, a.Into)
			continue
		case !slices.Contains(types, f.Type):
			add("%s: %s into %q must be %s (got %s)", at, a.Kind, a.Into, typeList(types), f.Type)
			continue
		}
		if a.Kind == "classify" {
			checkPhotoClassify(at, a, f, add)
		}
	}
}

func checkPhotoClassify(at string, a PhotoAnalyze, f *Field, add func(string, ...any)) {
	if f.Type != FieldEnum {
		if len(a.Options) < 2 {
			add("%s: classify into a %s field needs options (2+), or use an enum field", at, f.Type)
		}
		return
	}
	if len(a.Options) == 0 && len(f.Options) < 2 {
		add("%s: classify into %q needs at least 2 enum options", at, a.Into)
	}
	for _, o := range a.Options {
		if !slices.Contains(f.Options, o) {
			add("%s: option %q is not an option of enum field %q", at, o, a.Into)
		}
	}
}

func typeList(ts []FieldType) string {
	s := make([]string, len(ts))
	for i, t := range ts {
		s[i] = string(t)
	}
	return strings.Join(s, "|")
}
