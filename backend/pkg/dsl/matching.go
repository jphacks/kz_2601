package dsl

// Matching apps (#507): people find peers by interest (hobby, study, event)
// and connect only when both approve. Entity.match names the two record
// sets appconfig treats specially; who asked whom lives in appconfig
// (apps_public/{appId}/match_pairs), not in records. Design and the reasons
// for each rule: docs/backend/features/matching.md.

// EntityMatch is Entity.match.
type EntityMatch string

const (
	// MatchProfile: one card per person that others browse and send an
	// interest to. Authors who blocked each other do not see each other's.
	MatchProfile EntityMatch = "profile"
	// MatchMatched: read only by its author and the people matched with the
	// author (contact info, what to do next).
	MatchMatched EntityMatch = "matched"
)

// matchForbiddenTypes: fields an entity of that role must not have. No exact
// location anywhere (a coarse region is an enum); no photos on the profile
// that strangers browse.
var matchForbiddenTypes = map[EntityMatch][]FieldType{
	MatchProfile: {FieldLocation, FieldPhoto, FieldVideo},
	MatchMatched: {FieldLocation},
}

// ProfileEntity returns the match:"profile" entity or nil.
func (a *App) ProfileEntity() *Entity {
	for i := range a.Entities {
		if a.Entities[i].Match == MatchProfile {
			return &a.Entities[i]
		}
	}
	return nil
}

// MatchedEntities lists the ids of match:"matched" entities.
func (a *App) MatchedEntities() []string {
	var out []string
	for i := range a.Entities {
		if a.Entities[i].Match == MatchMatched {
			out = append(out, a.Entities[i].ID)
		}
	}
	return out
}

// MatchEntities lists the ids of entities with any match role.
func (a *App) MatchEntities() []string {
	var out []string
	for i := range a.Entities {
		if a.Entities[i].Match != "" {
			out = append(out, a.Entities[i].ID)
		}
	}
	return out
}

// HasMatching reports whether the app uses Entity.match at all.
func (a *App) HasMatching() bool {
	for i := range a.Entities {
		if a.Entities[i].Match != "" {
			return true
		}
	}
	return false
}

// checkEntityMatch validates one entity's match.
func checkEntityMatch(e *Entity, add func(string, ...any)) {
	if e.Match == "" {
		return
	}
	if e.Match != MatchProfile && e.Match != MatchMatched {
		add("entity %q match %q must be profile or matched", e.ID, e.Match)
		return
	}
	if !e.IsShared() {
		add(`entity %q match %q needs scope public or shared (other people read it)`, e.ID, e.Match)
	}
	if e.Edit == EditMembers {
		add(`entity %q match %q: omit edit (only the author may change a profile or contact record)`, e.ID, e.Match)
	}
	for _, f := range e.Fields {
		for _, t := range matchForbiddenTypes[e.Match] {
			if f.Type == t {
				add("entity %q field %q: a match %q entity cannot have a %s field (%s)", e.ID, f.ID, e.Match, t, matchForbiddenWhy(t))
			}
		}
	}
}

func matchForbiddenWhy(t FieldType) string {
	if t == FieldLocation {
		return "use an enum of coarse regions such as prefectures or オンライン instead of an exact location"
	}
	return "people browsing profiles are strangers; leave photos out of the profile"
}

// checkMatching validates the app-wide rules: one profile entity, and
// matched entities only next to a profile.
func checkMatching(a *App, add func(string, ...any)) {
	profiles := 0
	for i := range a.Entities {
		if a.Entities[i].Match == MatchProfile {
			profiles++
		}
	}
	if profiles > 1 {
		add("only one entity may have match \"profile\" (found %d)", profiles)
	}
	if profiles == 0 {
		for _, id := range a.MatchedEntities() {
			add(`entity %q match "matched" needs a match "profile" entity in the app (whose authors get matched)`, id)
		}
	}
}
