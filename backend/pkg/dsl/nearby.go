package dsl

// nearbyShare modes (#36 send / #425 passby).
const (
	NearbyModeSend   = "send"
	NearbyModePassby = "passby"
	// NearbyPassbyMaxLimit caps records sent per encounter (iOS also caps the payload at 16 KB).
	NearbyPassbyMaxLimit = 20
)

// nearbyPassbyBlockedTypes cannot be exchanged by passby: media is too large
// and never sent by default, refs point at the sender's own records.
var nearbyPassbyBlockedTypes = map[FieldType]bool{FieldPhoto: true, FieldVideo: true, FieldRef: true, FieldRefs: true}

func checkNearbyShare(where string, c *Component, entities map[string]*Entity, add func(string, ...any)) {
	if c.Label == "" {
		add("%s: nearbyShare needs label", where)
	}
	e := entities[c.Entity]
	if e == nil {
		add("%s: nearbyShare entity %q does not exist", where, c.Entity)
	}
	switch c.Mode {
	case "", NearbyModeSend:
		if len(c.Fields) > 0 || c.Limit != 0 || c.Distance {
			add("%s: nearbyShare fields / limit / distance are only for mode \"passby\"", where)
		}
		return
	case NearbyModePassby:
	default:
		add("%s: nearbyShare mode %q must be send or passby", where, c.Mode)
		return
	}
	if c.Limit < 0 || c.Limit > NearbyPassbyMaxLimit {
		add("%s: nearbyShare passby limit %d must be 1..%d", where, c.Limit, NearbyPassbyMaxLimit)
	}
	if len(c.Fields) == 0 {
		add("%s: nearbyShare passby needs fields (the field ids to exchange; nothing else is sent)", where)
	}
	if e == nil {
		return
	}
	if e.IsShared() {
		// Received profiles would be published to everyone else in the app.
		add("%s: nearbyShare passby entity %q must be private (received records are saved into it)", where, e.ID)
	}
	seen := map[string]bool{}
	for _, id := range c.Fields {
		f := e.FieldByID(id)
		switch {
		case f == nil:
			add("%s: nearbyShare passby field %q not in entity %q (fields: %s)", where, id, e.ID, fieldIDs(e))
		case seen[id]:
			add("%s: nearbyShare passby field %q is listed twice", where, id)
		case f.Hidden:
			add("%s: nearbyShare passby field %q is hidden and is never sent", where, id)
		case f.Computed != nil:
			add("%s: nearbyShare passby field %q is computed; list stored fields only", where, id)
		case nearbyPassbyBlockedTypes[f.Type]:
			add("%s: nearbyShare passby field %q is %s; photo / video / ref / refs are not exchanged", where, id, f.Type)
		}
		seen[id] = true
	}
}
