package dsl

// The creator's on / off switch of a rule (#347). A rule that is off stays
// in the DSL (still validated and counted against the plan's limits, so
// turning it back on never fails); appconfig queues nothing for it, the
// runner does not schedule it and skips an already queued run uncounted.

// DisabledNote ends the description of a rule that is off.
const DisabledNote = "(オフ)"

// IsEnabled reports whether the rule runs (enabled omitted = on).
func (r *ServerRule) IsEnabled() bool { return r.Enabled == nil || *r.Enabled }

func disabledNote(r *ServerRule) string {
	if r.IsEnabled() {
		return ""
	}
	return DisabledNote
}
