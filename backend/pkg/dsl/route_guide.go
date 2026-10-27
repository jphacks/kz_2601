package dsl

import (
	"strings"
	"unicode/utf8"
)

// routeGuide (#411): MapKit directions to one destination with the next turn,
// remaining distance / ETA and voice guidance (iOS, foreground location only).
// The destination is exactly one of
//   - locationField on the screen's record (detail / form screen: the place a
//     locationPicker on the same form picked),
//   - locationField of a record of entity chosen inside the component (home),
//   - a fixed address (searched with MapKit on the device).
// Shared fields: Title / Label / Entity / LocationField / TitleField / Mode.

// RouteGuideModes is the closed set of routeGuide.mode values ("" = automobile).
var RouteGuideModes = map[string]bool{"": true, "automobile": true, "walking": true}

// maxRouteAddressRunes caps routeGuide.address (one address or place name).
const maxRouteAddressRunes = 200

func checkRouteGuide(where string, c *Component, formEntity *Entity, entities map[string]*Entity, add func(string, ...any)) {
	if !RouteGuideModes[c.Mode] {
		add("%s: routeGuide mode %q must be automobile or walking", where, c.Mode)
	}
	address := strings.TrimSpace(c.Address)
	switch {
	case c.LocationField == "" && address == "":
		add("%s: routeGuide needs a destination: locationField (a location field of the record) or address", where)
		return
	case c.LocationField != "" && c.Address != "":
		add("%s: routeGuide takes either locationField or address, not both", where)
		return
	case address != "":
		if c.Entity != "" || c.TitleField != "" {
			add("%s: routeGuide entity / titleField are for locationField; a fixed address needs neither", where)
		}
		if utf8.RuneCountInString(address) > maxRouteAddressRunes {
			add("%s: routeGuide address is longer than %d characters", where, maxRouteAddressRunes)
		}
		return
	}
	e := formEntity
	if c.Entity != "" {
		if e = entities[c.Entity]; e == nil {
			add("%s: routeGuide entity %q does not exist", where, c.Entity)
			return
		}
	}
	if e == nil {
		add("%s: routeGuide locationField needs screen.entity (the record's place) or entity (choose among its records)", where)
		return
	}
	if f := e.FieldByID(c.LocationField); f == nil {
		add("%s: routeGuide locationField %q not in entity %q (fields: %s)", where, c.LocationField, e.ID, fieldIDs(e))
	} else if f.Type != FieldLocation {
		add("%s: routeGuide locationField %q must be type location", where, c.LocationField)
	}
	if c.TitleField != "" && e.FieldByID(c.TitleField) == nil {
		add("%s: routeGuide titleField %q not in entity %q", where, c.TitleField, e.ID)
	}
}
