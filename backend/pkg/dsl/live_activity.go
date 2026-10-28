package dsl

// Live Activity (#423): startActivity shows "now doing X" on the lock screen
// and Dynamic Island; timerDisplay.liveActivity mirrors a running timer there.
// iOS updates them locally (no push), so the only inputs are what the DSL
// says here plus the referenced stat / progressBar value.

// MaxActivityMinutes is the countdown cap: iOS ends a Live Activity after 8 hours.
const MaxActivityMinutes = 480

// Activity is the content of a startActivity action.
type Activity struct {
	Title    string `json:"title"`
	Subtitle string `json:"subtitle,omitempty"`
	Icon     string `json:"icon,omitempty"`    // SF Symbol, default timer
	Minutes  int    `json:"minutes,omitempty"` // countdown from now; 0 = elapsed time
	Stat     string `json:"stat,omitempty"`    // id of a stat / progressBar component
}

func checkActivity(where string, act *Activity, a *App, add func(string, ...any)) {
	if act == nil {
		add("%s: startActivity needs activity{title}", where)
		return
	}
	if act.Title == "" {
		add("%s: startActivity activity.title is empty", where)
	}
	if act.Minutes < 0 || act.Minutes > MaxActivityMinutes {
		add("%s: startActivity activity.minutes must be 1-%d (got %d); omit it to count elapsed time", where, MaxActivityMinutes, act.Minutes)
	}
	if act.Stat == "" {
		return
	}
	var found *Component
	for _, s := range a.Screens {
		eachScreenComponent(s.Components, func(_ string, c *Component) {
			if found == nil && c.ID == act.Stat {
				found = c
			}
		})
	}
	switch {
	case found == nil:
		add("%s: startActivity activity.stat %q is not a component id; give the stat / progressBar an id and use it", where, act.Stat)
	case found.Type != CompStat && found.Type != CompProgressBar:
		add("%s: startActivity activity.stat %q is a %s; point it at a stat or progressBar", where, act.Stat, found.Type)
	}
}
