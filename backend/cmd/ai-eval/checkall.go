package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// fileShape is what a JSON file under internal/ai/eval holds.
type fileShape string

const (
	shapeGenerate fileShape = "generate" // []Case for /generate (default run)
	shapeChat     fileShape = "chat"     // []ChatCase for -chat
	shapeDSL      fileShape = "dsl"      // an app document used as an edit base (example:../eval/<name>)
)

// detectShape tells the loader for a file by its content, and requires the
// name to agree: chat cases are named *chat_cases.json / chat_cases.json,
// generate cases *cases.json, DSL bases anything else. An unknown shape or a
// name that disagrees is an error, so a new file can't be skipped silently.
func detectShape(name string, raw []byte) (fileShape, error) {
	base := filepath.Base(name)
	isCases := strings.HasSuffix(base, "cases.json")
	isChat := isCases && strings.Contains(base, "chat")
	shape, err := shapeOf(raw)
	if errors.Is(err, errEmptyArray) && isCases {
		// e.g. jphacks_coverage_pending_cases.json once every PR merged
		if isChat {
			return shapeChat, nil
		}
		return shapeGenerate, nil
	}
	if err != nil {
		return "", err
	}
	switch {
	case shape == shapeChat && !isChat:
		return "", fmt.Errorf("holds chat cases (steps) but is not named *chat_cases.json")
	case shape == shapeGenerate && (!isCases || isChat):
		return "", fmt.Errorf("holds generate cases (kind/transcript) but is not named *cases.json without chat")
	case shape == shapeDSL && isCases:
		return "", fmt.Errorf("holds a DSL app but is named *cases.json")
	}
	return shape, nil
}

// errEmptyArray: an empty array is a cases file only by its name.
var errEmptyArray = errors.New("unknown shape: empty array outside a *cases.json")

func shapeOf(raw []byte) (fileShape, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return "", errors.New("empty file")
	}
	switch trimmed[0] {
	case '{':
		var doc map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &doc); err != nil {
			return "", err
		}
		if _, ok := doc["schemaVersion"]; ok {
			if _, ok := doc["app"]; ok {
				return shapeDSL, nil
			}
		}
		return "", errors.New("unknown shape: object without schemaVersion/app")
	case '[':
		var items []map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &items); err != nil {
			return "", fmt.Errorf("unknown shape: %w", err)
		}
		if len(items) == 0 {
			return "", errEmptyArray
		}
		var shape fileShape
		for i, it := range items {
			var s fileShape
			_, steps := it["steps"]
			_, kind := it["kind"]
			_, transcript := it["transcript"]
			switch {
			case steps && !kind && !transcript:
				s = shapeChat
			case !steps && (kind || transcript):
				s = shapeGenerate
			default:
				return "", fmt.Errorf("unknown shape: item %d has neither steps nor kind/transcript", i+1)
			}
			if shape != "" && s != shape {
				return "", fmt.Errorf("mixed shapes: item %d is %s, earlier items %s", i+1, s, shape)
			}
			shape = s
		}
		return shape, nil
	}
	return "", errors.New("unknown shape: not a JSON object or array")
}

// checkFile validates one file with the loader its shape needs. root is the
// backend directory (chat bases are paths from it; example: bases live in
// root/internal/ai/examples).
func checkFile(path, root string) (fileShape, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	shape, err := detectShape(path, raw)
	if err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	switch shape {
	case shapeDSL:
		if _, err := dsl.Parse(raw); err != nil {
			return shape, fmt.Errorf("%s: %w", path, err)
		}
		return shape, nil
	case shapeChat:
		if err := checkCasesFile(path, true); err != nil {
			return shape, err
		}
		var cases []ChatCase
		_ = json.Unmarshal(raw, &cases)
		var bad []string
		for _, c := range cases {
			if c.Base != "" {
				if err := checkBaseFile(filepath.Join(root, c.Base)); err != nil {
					bad = append(bad, "["+c.ID+"] base "+c.Base+": "+err.Error())
				}
			}
		}
		return shape, joinBad(path, bad)
	default:
		if err := checkCasesFile(path, false); err != nil {
			return shape, err
		}
		var cases []Case
		_ = json.Unmarshal(raw, &cases)
		var bad []string
		for _, c := range cases {
			if name, ok := strings.CutPrefix(c.Base, "example:"); ok {
				if err := checkBaseFile(filepath.Join(root, "internal/ai/examples", name+".json")); err != nil {
					bad = append(bad, "["+c.ID+"] base "+c.Base+": "+err.Error())
				}
			}
		}
		return shape, joinBad(path, bad)
	}
}

// checkBaseFile: an edit base must exist and be a valid DSL app.
func checkBaseFile(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	_, err = dsl.Parse(raw)
	return err
}

func joinBad(path string, bad []string) error {
	if len(bad) == 0 {
		return nil
	}
	sort.Strings(bad)
	return fmt.Errorf("%s:\n  %s", path, strings.Join(bad, "\n  "))
}

// checkAll validates every *.json under dir (internal/ai/eval, subdirectories
// too) and returns how many files of each shape it checked. root is the
// backend directory.
func checkAll(dir, root string) (map[fileShape]int, error) {
	var files []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".json") {
			files = append(files, p)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no JSON files in %s", dir)
	}
	counts := map[fileShape]int{}
	var errs []error
	for _, f := range files {
		shape, err := checkFile(f, root)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		counts[shape]++
	}
	return counts, errors.Join(errs...)
}
