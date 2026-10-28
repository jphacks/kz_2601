package appconfig

import (
	"fmt"
	"strconv"
	"strings"
)

// maxAudioLen bounds a type=audio value: "audio:<id>|<seconds>" (#420).
const maxAudioLen = 200

// checkAudioValue accepts "audio:<id>" optionally followed by "|<seconds>"
// (whole seconds, 0..86400), or an https URL (reserved for a future upload).
// The m4a itself stays on the recording device, like video (#206).
func checkAudioValue(s string) error {
	if len(s) > maxAudioLen {
		return fmt.Errorf("exceeds %d bytes", maxAudioLen)
	}
	if strings.HasPrefix(s, "https://") {
		return nil
	}
	rest, ok := strings.CutPrefix(s, "audio:")
	if !ok {
		return fmt.Errorf("must be \"audio:<id>|<seconds>\" or an https URL")
	}
	id, secs, hasSecs := strings.Cut(rest, "|")
	if !videoIDPattern.MatchString(id) {
		return fmt.Errorf("has an invalid audio id")
	}
	if hasSecs {
		if n, err := strconv.Atoi(secs); err != nil || n < 0 || n > 86400 {
			return fmt.Errorf("length must be whole seconds")
		}
	}
	return nil
}
