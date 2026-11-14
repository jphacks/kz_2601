package dsl

import (
	"fmt"
	"slices"
)

// Roles and record rules on shared data (#272 #273, docs/dsl/README.md §3.13).
//
// Roles: the app's creator is its first admin and an admin may make other
// participants admins (appconfig members.go). A field with editableBy
// "admins" is set only by admins, a button with visibleTo "admins" shows only
// to admins, and entity.deleteBy says who deletes a shared/public record.
// A member field holds the uid of one participant of the shared data.
//
// Unique: entity.unique lists 1..MaxUniqueFields field ids (or $author).
// appconfig refuses a write that would give two records the same values of
// all of them (409 duplicate_record), checked in a transaction.

// FieldMember is the uid of a participant (creator or joined member) of the
// app's shared data. Only on scope "shared" entities.
const FieldMember FieldType = "member"

// RoleAdmins is the value of Field.EditableBy and Component.VisibleTo.
const RoleAdmins = "admins"

// EntityDeleteBy says who may delete a shared/public record.
type EntityDeleteBy string

const (
	DeleteByAuthor         EntityDeleteBy = "author"         // the record's author (default)
	DeleteByAdmins         EntityDeleteBy = "admins"         // the app's admins only
	DeleteByAuthorOrAdmins EntityDeleteBy = "authorOrAdmins" // the author or an admin
)

// MaxUniqueFields bounds entity.unique.
const MaxUniqueFields = 3

// uniqueFieldTypes are the stored types entity.unique may name (short,
// comparable values; not long text, media, locations or lists).
var uniqueFieldTypes = []FieldType{FieldString, FieldInteger, FieldNumber, FieldCurrency, FieldBoolean,
	FieldDate, FieldDatetime, FieldEnum, FieldRef, FieldMember}

// EffectiveDeleteBy resolves the default (author). Private entities ignore it.
func (e *Entity) EffectiveDeleteBy() EntityDeleteBy {
	if e.DeleteBy == "" {
		return DeleteByAuthor
	}
	return e.DeleteBy
}

// AdminOnly reports whether only admins may set the field.
func (f *Field) AdminOnly() bool { return f.EditableBy == RoleAdmins }

// HasAdminOnlyFields reports whether any stored field is editableBy admins.
func (e *Entity) HasAdminOnlyFields() bool {
	return slices.ContainsFunc(e.Fields, func(f Field) bool { return f.AdminOnly() })
}

// UsesRoles reports whether the app declares anything that depends on who is
// an admin (iOS then loads the participants to know the caller's role).
func (a *App) UsesRoles() bool {
	for i := range a.Entities {
		e := &a.Entities[i]
		if e.HasAdminOnlyFields() || e.DeleteBy == DeleteByAdmins || e.DeleteBy == DeleteByAuthorOrAdmins {
			return true
		}
	}
	found := false
	for i := range a.Screens {
		eachScreenComponent(a.Screens[i].Components, func(_ string, c *Component) {
			found = found || c.VisibleTo != ""
		})
	}
	return found
}

func checkDataRules(a *App, add func(string, ...any)) {
	shared := false
	for i := range a.Entities {
		e := &a.Entities[i]
		shared = shared || e.IsShared()
		checkRoleFields(e, add)
		checkDeleteBy(e, add)
		checkUnique(e, add)
	}
	for i := range a.Screens {
		s := &a.Screens[i]
		eachScreenComponent(s.Components, func(where string, c *Component) {
			if c.VisibleTo == "" {
				return
			}
			where = fmt.Sprintf("screen %q %s", s.ID, where)
			switch {
			case c.Type != CompButton:
				add("%s: visibleTo is only for button (got %s)", where, c.Type)
			case c.VisibleTo != RoleAdmins:
				add("%s: visibleTo %q must be admins", where, c.VisibleTo)
			case !shared:
				add("%s: visibleTo admins needs a shared or public entity (in a private app the only user is the creator); remove visibleTo", where)
			}
		})
	}
}

func checkRoleFields(e *Entity, add func(string, ...any)) {
	for i := range e.Fields {
		f := &e.Fields[i]
		if f.EditableBy != "" {
			switch {
			case f.EditableBy != RoleAdmins:
				add("entity %q field %q: editableBy %q must be admins", e.ID, f.ID, f.EditableBy)
			case !e.IsShared():
				add("entity %q field %q: editableBy admins needs scope shared or public (a private record is only its owner's); remove editableBy", e.ID, f.ID)
			case f.Computed != nil:
				add("entity %q field %q: editableBy is for stored fields, not computed ones", e.ID, f.ID)
			}
		}
		if f.Type != FieldMember {
			continue
		}
		if e.EffectiveScope() != ScopeShared {
			add(`entity %q field %q: type member (a participant of the shared data) needs the entity scope "shared"; use ref to a members entity for public or private data`, e.ID, f.ID)
		}
		if f.DefaultValue != nil {
			add("entity %q field %q: a member field cannot have defaultValue (pick the participant in the form)", e.ID, f.ID)
		}
	}
}

func checkDeleteBy(e *Entity, add func(string, ...any)) {
	switch e.DeleteBy {
	case "":
		return
	case DeleteByAuthor, DeleteByAdmins, DeleteByAuthorOrAdmins:
	default:
		add("entity %q deleteBy %q must be author, admins or authorOrAdmins", e.ID, e.DeleteBy)
		return
	}
	switch {
	case !e.IsShared():
		add("entity %q deleteBy needs scope shared or public (private records are only their owner's); remove deleteBy", e.ID)
	case e.Match != "":
		add("entity %q deleteBy: a match %q entity is deleted by its author only; remove deleteBy", e.ID, e.Match)
	case e.Single:
		add("entity %q deleteBy: a single entity has no delete; remove deleteBy", e.ID)
	}
}

func checkUnique(e *Entity, add func(string, ...any)) {
	if len(e.Unique) == 0 {
		return
	}
	switch {
	case len(e.Unique) > MaxUniqueFields:
		add("entity %q unique has %d fields (max %d)", e.ID, len(e.Unique), MaxUniqueFields)
	case e.Single:
		add("entity %q unique: a single entity always has one record; remove unique", e.ID)
	case e.Match != "":
		add("entity %q unique: a match %q entity cannot have unique (one profile per person is built in)", e.ID, e.Match)
	}
	seen := map[string]bool{}
	for _, id := range e.Unique {
		if seen[id] {
			add("entity %q unique lists %q twice", e.ID, id)
			continue
		}
		seen[id] = true
		if id == RecordAuthor {
			if !e.IsShared() {
				add(`entity %q unique: %q (one per person) needs scope shared or public; every private record is the owner's`, e.ID, id)
			}
			continue
		}
		f := e.FieldByID(id)
		switch {
		case f == nil:
			add("entity %q unique: field %q does not exist (fields: %s)", e.ID, id, fieldIDs(e))
		case f.Computed != nil:
			add("entity %q unique: field %q is computed; name stored fields", e.ID, id)
		case !slices.Contains(uniqueFieldTypes, f.Type):
			add("entity %q unique: field %q is type %s; unique fields must be one of %v", e.ID, id, f.Type, uniqueFieldTypes)
		}
	}
}
