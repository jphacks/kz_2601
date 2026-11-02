package dsl

// Matching UI (#507 phase 2). list.item.interest puts a つながりたい button
// on the rows of a match:"profile" list; matchList shows the people who sent
// one (incoming), the ones the user sent (outgoing) or the people connected
// (matches), each drawn from their profile record. Sending, accepting,
// declining and unmatching live in the iOS app (appconfig /interests,
// /matches), so there is no Action for them. docs/backend/features/matching.md §9.

// MatchListShows is the closed set of matchList.show values.
var MatchListShows = map[string]bool{"incoming": true, "outgoing": true, "matches": true}

// checkListInterest validates list.item.interest against the list entity.
func checkListInterest(where string, item *ListItem, e *Entity, add func(string, ...any)) {
	if item == nil || !item.Interest {
		return
	}
	if e.Match != MatchProfile {
		add(`%s: list item.interest needs the list entity to have match "profile" (entity %q is not a profile)`, where, e.ID)
	}
}

// checkMatchList validates one matchList.
func checkMatchList(where string, c *Component, entities map[string]*Entity, add func(string, ...any)) {
	e := entities[c.Entity]
	switch {
	case c.Entity == "":
		add(`%s: matchList needs entity (the match "profile" entity)`, where)
		return
	case e == nil:
		add("%s: matchList entity %q does not exist", where, c.Entity)
		return
	case e.Match != MatchProfile:
		add(`%s: matchList entity %q must be the match "profile" entity (rows are the other people's profiles)`, where, c.Entity)
	}
	if !MatchListShows[c.Show] {
		add("%s: matchList show %q must be incoming, outgoing or matches", where, c.Show)
	}
	if c.Item == nil {
		return
	}
	for _, fid := range []string{c.Item.Title, c.Item.Subtitle, c.Item.Trailing} {
		if msg := valueErr(e, "field", fid); fid != "" && msg != "" {
			add("%s: matchList item %s", where, msg)
		}
	}
	if c.Item.Toggle != "" || c.Item.Mark != "" || c.Item.Interest || c.Item.Image != "" || c.Item.Resets != "" {
		add("%s: matchList item uses only title / subtitle / trailing / icon (the row menu sends, accepts and ends connections)", where)
	}
}
