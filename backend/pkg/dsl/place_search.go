package dsl

import (
	"slices"
	"strings"
	"unicode/utf8"
)

// placeSearch: nearby places from Apple Maps (MKLocalSearch on the device,
// no key, never through our server). "記録に追加" writes the chosen place into
// the open form (name / "lat,lng" / address); saving stays saveRecord.
// locationButton.addressField reverse-geocodes the captured point the same way.

// PlaceCategories is the closed set of placeSearch.category values (iOS maps
// each to an MKPointOfInterestCategory available on iOS 17).
var PlaceCategories = []string{
	"cafe", "restaurant", "bakery", "store", "station", "park", "hospital", "pharmacy", "parking",
	"gasStation", "evCharger", "hotel", "museum", "library", "movieTheater", "school", "university", "fitness",
}

// maxPlaceQueryRunes caps placeSearch.query (a few search words).
const maxPlaceQueryRunes = 50

func checkPlaceSearch(where string, c *Component, formEntity *Entity, add func(string, ...any)) {
	if c.Category != "" && !slices.Contains(PlaceCategories, c.Category) {
		add("%s: placeSearch category %q must be one of %s", where, c.Category, strings.Join(PlaceCategories, ", "))
	}
	if c.Near != "" && c.Near != "current" {
		add("%s: placeSearch near %q must be current", where, c.Near)
	}
	if q := strings.TrimSpace(c.Query); c.Query != "" && q == "" {
		add("%s: placeSearch query is blank", where)
	} else if utf8.RuneCountInString(q) > maxPlaceQueryRunes {
		add("%s: placeSearch query is longer than %d characters", where, maxPlaceQueryRunes)
	}
	if formEntity == nil {
		add("%s: placeSearch requires screen.entity (the form 記録に追加 fills)", where)
		return
	}
	if c.NameField == "" && c.LocationField == "" && c.AddressField == "" {
		add("%s: placeSearch needs nameField, locationField or addressField (what 記録に追加 fills)", where)
		return
	}
	seen := map[string]string{}
	bind := func(key, id string, ok func(*Field) bool, want string) {
		if id == "" {
			return
		}
		if prev, dup := seen[id]; dup {
			add("%s: placeSearch %s and %s are both %q", where, prev, key, id)
			return
		}
		seen[id] = key
		if f := formEntity.FieldByID(id); f == nil {
			add("%s: placeSearch %s %q not in entity %q (fields: %s)", where, key, id, formEntity.ID, fieldIDs(formEntity))
		} else if f.Computed != nil || !ok(f) {
			add("%s: placeSearch %s %q must be a stored %s field", where, key, id, want)
		}
	}
	bind("nameField", c.NameField, isPlaceTextField, "string or text")
	bind("locationField", c.LocationField, func(f *Field) bool { return f.Type == FieldLocation }, "location")
	bind("addressField", c.AddressField, isPlaceTextField, "string or text")
}

// checkLocationButtonAddress: locationButton.addressField gets the address of
// the captured point. Only the field itself is checked here (validate.go has
// already reported a bad field / no screen.entity).
func checkLocationButtonAddress(where string, c *Component, formEntity *Entity, add func(string, ...any)) {
	if c.AddressField == "" || formEntity == nil {
		return
	}
	if c.Type != CompLocationButton {
		add("%s: addressField is only for locationButton / placeSearch", where)
		return
	}
	if c.AddressField == c.Field {
		add("%s: locationButton addressField must differ from field", where)
	} else if f := formEntity.FieldByID(c.AddressField); f == nil {
		add("%s: locationButton addressField %q not in entity %q (fields: %s)", where, c.AddressField, formEntity.ID, fieldIDs(formEntity))
	} else if f.Computed != nil || !isPlaceTextField(f) {
		add("%s: locationButton addressField %q must be a stored string or text field (the address)", where, c.AddressField)
	}
}

func isPlaceTextField(f *Field) bool { return f.Type == FieldString || f.Type == FieldText }
