package dsl

import "fmt"

// Record values (#250 #251): what every record carries besides its fields.
// They start with "$" so they never collide with a field id (^[a-z]...).
const (
	// RecordAuthor shows the poster's display name (authorName). Shared/public
	// entities only. As a filter field it compares the poster's uid and takes
	// the value {"me": true} (= the signed-in user).
	RecordAuthor = "$author"
	// RecordCreatedAt is when the record was created (a datetime).
	RecordCreatedAt = "$createdAt"
)

// RecordValues lists the reserved record values ($defs/Entity x-recordValues).
var RecordValues = []string{RecordAuthor, RecordCreatedAt}

// valueErr reports why id cannot be shown from entity e ("" = it can): a
// field of e, $createdAt, or $author on a shared/public entity. key names the
// property in the message (field / titleField / ...).
func valueErr(e *Entity, key, id string) string {
	switch {
	case e.FieldByID(id) != nil, id == RecordCreatedAt:
		return ""
	case id == RecordAuthor:
		if e.IsShared() {
			return ""
		}
		return fmt.Sprintf(`%s %q is only for shared/public entities (entity %q is private, so every record is the user's own)`, key, id, e.ID)
	}
	return fmt.Sprintf("%s %q not in entity %q", key, id, e.ID)
}

// isMeValue reports whether a decoded filter value is {"me": true}.
func isMeValue(v any) bool {
	m, ok := v.(map[string]any)
	if !ok || len(m) != 1 {
		return false
	}
	b, ok := m["me"].(bool)
	return ok && b
}

// hasFilterField: a component filter may name a field or $author (the rest
// of the $author rules live in checkMeFilter).
func hasFilterField(e *Entity, id string) bool {
	return id == RecordAuthor || e.FieldByID(id) != nil
}

// checkMeFilter validates "only my posts" on a screen component's own filter:
// {"field":"$author","op":"eq"|"neq","value":{"me":true}} on a shared/public
// entity. It reports whether f was such a filter (the caller then skips the
// Expr / period checks).
func checkMeFilter(where string, f *Filter, e *Entity, add func(string, ...any)) bool {
	me := isMeValue(f.Value)
	if f.Field != RecordAuthor && !me {
		return false
	}
	where += ".filter"
	if f.Field != RecordAuthor {
		add(`%s: value {"me":true} is only for field %q (the poster)`, where, RecordAuthor)
		return true
	}
	if !me {
		add(`%s: field %q compares the poster; use value {"me":true} with op eq (my posts) or neq (other people's posts)`, where, RecordAuthor)
	}
	if f.Op != "eq" && f.Op != "neq" {
		add(`%s: field %q takes op eq (my posts) or neq (other people's posts), not %q`, where, RecordAuthor, f.Op)
	}
	if e != nil && !e.IsShared() {
		add(`%s: filter by %q is only for shared/public entities (entity %q is private, so every record is the user's own; remove the filter)`, where, RecordAuthor, e.ID)
	}
	return true
}
