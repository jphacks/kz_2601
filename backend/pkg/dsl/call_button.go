package dsl

import "regexp"

// callButton (#447): joins the voice call of a shared app (video too when
// video is true). Only people who take part in the app (its creator and the
// members who joined with a share link) can get into the room; there is no
// ringing, so the others hear it only while they have the app open.
// appconfig issues the room token (POST /v1/apps/{appId}/calls/token, from
// the published DSL) and charges the caller's own coins by the minute.
// room separates rooms inside one app (e.g. "lobby", "team-a"); "" is the
// app's room.

// callRoomRe is a callButton room key.
var callRoomRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// ValidCallRoom reports whether room is a valid callButton room key ("" too).
func ValidCallRoom(room string) bool { return room == "" || callRoomRe.MatchString(room) }

func checkCallButton(where string, c *Component, a *App, add func(string, ...any)) {
	if !ValidCallRoom(c.Room) {
		add("%s: callButton room %q must be 1-32 of a-z 0-9 _ - (or omitted for the app's room)", where, c.Room)
	}
	for i := range a.Entities {
		if a.Entities[i].IsShared() {
			return
		}
	}
	add(`%s: callButton works only in an app shared with other people: give an entity "scope": "shared" (or public) so participants can join`, where)
}

// CallButton returns a callButton with room key room on any screen (nil
// when the app has none): appconfig issues tokens only for rooms a
// published callButton names.
func (a *App) CallButton(room string) *Component {
	var out *Component
	for i := range a.Screens {
		eachScreenComponent(a.Screens[i].Components, func(_ string, c *Component) {
			if out == nil && c.Type == CompCallButton && c.Room == room {
				out = c
			}
		})
	}
	return out
}

// HasCallButton reports whether any screen has a callButton.
func (a *App) HasCallButton() bool {
	for i := range a.Screens {
		found := false
		eachScreenComponent(a.Screens[i].Components, func(_ string, c *Component) { found = found || c.Type == CompCallButton })
		if found {
			return true
		}
	}
	return false
}

// dupCallButtons (lint): two callButtons of the same room on one screen do
// the same thing.
func (l *linter) dupCallButtons() {
	for _, s := range l.app.Screens {
		seen := map[string]bool{}
		eachScreenComponent(s.Components, func(_ string, c *Component) {
			if c.Type != CompCallButton {
				return
			}
			if seen[c.Room] {
				l.add("screen %q: two callButtons join the same room %q; keep one (or give the second its own room)", s.ID, c.Room)
			}
			seen[c.Room] = true
		})
	}
}
