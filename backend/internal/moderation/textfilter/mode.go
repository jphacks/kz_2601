package textfilter

import (
	"fmt"
	"strings"
)

// Mode is TEXT_FILTER: the operator's switch for the filter (a false
// positive can be turned off without a deploy).
type Mode string

// Modes.
const (
	// ModeEnforce (default): a match is refused (422 text_not_allowed).
	ModeEnforce Mode = "enforce"
	// ModeLog: a match is logged with its category but saved.
	ModeLog Mode = "log"
	// ModeOff: the text is not looked at.
	ModeOff Mode = "off"
)

// ModeEnv is the environment variable of Mode.
const ModeEnv = "TEXT_FILTER"

// ModeFromEnv reads TEXT_FILTER (unset = enforce). Another value is an
// error: a typo must not switch the filter off silently.
func ModeFromEnv(getenv func(string) string) (Mode, error) {
	switch m := Mode(strings.ToLower(strings.TrimSpace(getenv(ModeEnv)))); m {
	case "":
		return ModeEnforce, nil
	case ModeEnforce, ModeLog, ModeOff:
		return m, nil
	default:
		return ModeEnforce, fmt.Errorf("%s must be enforce|log|off, got %q", ModeEnv, getenv(ModeEnv))
	}
}
