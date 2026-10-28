package dsl

// Location tracking (#168): locationTracker records points (one record per
// point: a location field + a datetime field) only while the user has pressed
// start. mapView kind "route" draws them as a line and stat aggregate
// distance/duration sums them (iOS: Track.swift; gaps of 10+ minutes split
// separate walks).

// IsTrackAgg reports whether a stat aggregate reads locationTracker points.
func IsTrackAgg(agg string) bool { return agg == "distance" || agg == "duration" }

// checkLocationTracker: entity with a location field, an optional datetime
// field, and no other required field without a defaultValue (points are saved
// without a form, so nobody can fill it).
func checkLocationTracker(where string, c *Component, entities map[string]*Entity, add func(string, ...any)) {
	e := entities[c.Entity]
	if e == nil {
		add("%s: locationTracker entity %q does not exist", where, c.Entity)
		return
	}
	if f := e.FieldByID(c.LocationField); f == nil {
		add("%s: locationTracker locationField %q not in entity %q (fields: %s)", where, c.LocationField, e.ID, fieldIDs(e))
	} else if f.Type != FieldLocation || f.Computed != nil {
		add("%s: locationTracker locationField %q must be a stored location field", where, c.LocationField)
	}
	if c.DateField != "" {
		if f := e.FieldByID(c.DateField); f == nil {
			add("%s: locationTracker dateField %q not in entity %q (fields: %s)", where, c.DateField, e.ID, fieldIDs(e))
		} else if f.Type != FieldDatetime || f.Computed != nil {
			add("%s: locationTracker dateField %q must be a stored datetime field", where, c.DateField)
		}
	}
	if c.Seconds < 0 {
		add("%s: locationTracker seconds must be positive (auto stop, default 3600)", where)
	}
	for _, f := range e.Fields {
		if f.Required && f.Computed == nil && f.DefaultValue == nil && f.ID != c.LocationField && f.ID != c.DateField {
			add("%s: locationTracker saves points without a form, so required field %q of entity %q needs a defaultValue (or make it optional)", where, f.ID, e.ID)
		}
	}
}

// checkMapRoute: kind pins|route; dateField (route only) orders the points.
func checkMapRoute(where string, c *Component, e *Entity, add func(string, ...any)) {
	switch c.Kind {
	case "", "pins":
		if c.DateField != "" {
			add("%s: mapView dateField is only for kind route (remove it)", where)
		}
	case "route":
		if c.DateField == "" {
			return
		}
		if f := e.FieldByID(c.DateField); f == nil {
			add("%s: mapView dateField %q not in entity %q (fields: %s)", where, c.DateField, e.ID, fieldIDs(e))
		} else if f.Type != FieldDatetime && f.Type != FieldDate {
			add("%s: mapView dateField %q must be a datetime field", where, c.DateField)
		}
	default:
		add("%s: mapView kind must be pins or route", where)
	}
}

// checkTrackStat: distance/duration need field = location field and by = a
// date/datetime field (omitted = creation order).
func checkTrackStat(where string, c *Component, e *Entity, add func(string, ...any)) {
	if f := e.FieldByID(c.Field); f != nil && f.Type != FieldLocation {
		add("%s: stat aggregate %s field %q must be the location field of the points (got %s)", where, c.Aggregate, c.Field, f.Type)
	}
	if c.Format != "" && c.Format != "plain" {
		add("%s: stat aggregate %s is shown as m/km or m:ss; remove format", where, c.Aggregate)
	}
	checkPickBy(where, c.Aggregate, c.By, e, add)
}

// trackedEntities: entities a locationTracker or a saving sensorValue (#419)
// on a reachable screen writes without a form.
func trackedEntities(a *App, reach map[string]bool) map[string]bool {
	out := map[string]bool{}
	for _, s := range a.Screens {
		if !reach[s.ID] {
			continue
		}
		eachScreenComponent(s.Components, func(_ string, c *Component) {
			if c.Type == CompLocationTracker || (c.Type == CompSensorValue && c.Entity != "") {
				out[c.Entity] = true
			}
		})
	}
	return out
}
