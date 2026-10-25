package ai

import (
	"bytes"
	"encoding/json"
	"strings"
)

// keyWatcher scans streamed JSON text and reports when the value of one
// top-level key is complete, so the caller can act on it (e.g. the `app`
// block of an AppDSL, or `summary` of a PatchDocument) before the rest of
// the document has been generated. It tolerates a code fence or preamble
// before the opening brace, the way extractJSON does.
type keyWatcher struct {
	key   string
	buf   strings.Builder
	done  bool
	value []byte
}

func newKeyWatcher(key string) *keyWatcher { return &keyWatcher{key: key} }

// Feed appends a text delta and returns the key's value (raw JSON) exactly
// once, when it has been fully received.
func (w *keyWatcher) Feed(delta string) ([]byte, bool) {
	if w.done {
		return nil, false
	}
	w.buf.WriteString(delta)
	s := w.buf.String()
	start := strings.Index(s, "{")
	if start < 0 {
		return nil, false
	}
	v, ok := topLevelValue(s[start:], w.key)
	if !ok {
		return nil, false
	}
	w.done = true
	w.value = v
	return v, true
}

// topLevelValue finds `"key": <value>` at depth 1 of a (possibly truncated)
// JSON object and returns the value when it is complete.
func topLevelValue(s, key string) ([]byte, bool) {
	needle := `"` + key + `"`
	depth := 0
	inStr, esc := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr {
			switch {
			case esc:
				esc = false
			case c == '\\':
				esc = true
			case c == '"':
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			if depth == 1 && strings.HasPrefix(s[i:], needle) {
				j := i + len(needle)
				for j < len(s) && (s[j] == ' ' || s[j] == '\n' || s[j] == '\r' || s[j] == '\t') {
					j++
				}
				if j < len(s) && s[j] == ':' {
					return completeValue(s[j+1:])
				}
			}
			inStr = true
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		}
	}
	return nil, false
}

// completeValue returns the first JSON value at the start of s (after
// whitespace) when it is complete; strings, objects, arrays and scalars.
func completeValue(s string) ([]byte, bool) {
	s = strings.TrimLeft(s, " \n\r\t")
	if s == "" {
		return nil, false
	}
	switch s[0] {
	case '{', '[':
		depth := 0
		inStr, esc := false, false
		for i := 0; i < len(s); i++ {
			c := s[i]
			if inStr {
				switch {
				case esc:
					esc = false
				case c == '\\':
					esc = true
				case c == '"':
					inStr = false
				}
				continue
			}
			switch c {
			case '"':
				inStr = true
			case '{', '[':
				depth++
			case '}', ']':
				depth--
				if depth == 0 {
					return []byte(s[:i+1]), true
				}
			}
		}
		return nil, false
	case '"':
		esc := false
		for i := 1; i < len(s); i++ {
			switch {
			case esc:
				esc = false
			case s[i] == '\\':
				esc = true
			case s[i] == '"':
				return []byte(s[:i+1]), true
			}
		}
		return nil, false
	default: // number / true / false / null: complete once a delimiter follows
		end := strings.IndexAny(s, ",}] \n\r\t")
		if end < 0 {
			return nil, false
		}
		return []byte(s[:end]), json.Valid([]byte(s[:end]))
	}
}

// compactRaw is json.Compact into a fresh buffer.
func compactRaw(v json.RawMessage) ([]byte, error) {
	var b bytes.Buffer
	if err := json.Compact(&b, v); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
