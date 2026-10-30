package dsl

// dashcam (#410): a drive recorder on the rear camera. Clips stay on the
// device (never in records), so it needs no entity. Ranges are enforced by the
// schema; Go checks what the schema cannot say.

// DashcamAutoStartConditions is the closed set of dashcam.autoStart values.
var DashcamAutoStartConditions = map[string]bool{"landscape": true, "still": true, "charging": true}

func checkDashcam(where string, c *Component, add func(string, ...any)) {
	if c.Entity != "" || c.Field != "" {
		add("%s: dashcam keeps clips on the device; remove entity/field", where)
	}
	seen := map[string]bool{}
	for _, v := range c.AutoStart {
		if !DashcamAutoStartConditions[v] {
			add("%s: dashcam autoStart %q must be landscape, still or charging", where, v)
		}
		if seen[v] {
			add("%s: dashcam autoStart %q is listed twice", where, v)
		}
		seen[v] = true
	}
}

// checkDashcamCount: the camera serves one recorder at a time, so a second
// dashcam anywhere in the app would only fight over it.
func checkDashcamCount(a *App, add func(string, ...any)) {
	var at []string
	for _, s := range a.Screens {
		for _, c := range s.Components {
			for _, x := range append([]Component{c}, c.Children...) {
				if x.Type == CompDashcam {
					at = append(at, s.ID)
				}
			}
		}
	}
	if len(at) > 1 {
		add("only one dashcam per app (found on screens %v)", at)
	}
}
