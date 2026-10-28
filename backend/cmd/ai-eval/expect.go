package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// Expect format check and the per-group pass rate (#281). Both run without
// the Claude API: `-check` validates a cases file and exits, and every run
// validates its cases before the first call so a typo in an expect key fails
// fast instead of silently checking nothing.

type expectKind int

const (
	expectBool expectKind = iota
	expectString
	expectNumber
	expectStrings
)

// expectKeys lists every `expect` key the checks in main.go read and the JSON
// type each one takes. Add a key here together with its check.
var expectKeys = map[string]expectKind{
	// checkExpect
	"substitution": expectBool,
	"health":       expectString,
	"healthField":  expectBool,
	"grid":         expectBool,
	"calendar":     expectBool,
	"photo":        expectBool,
	"chart":        expectBool,
	"notification": expectString,
	"calc":         expectBool,
	"computed":     expectBool,
	"statValue":    expectBool,
	"settlement":   expectBool,
	"ref":          expectBool,
	"noShared":     expectBool,
	"shared":       expectBool,
	"scope":        expectString,
	"editMembers":  expectBool,   // #252 (PR #378)
	"noOmission":   expectBool,   // #257 (PR #379)
	"authorShown":  expectBool,   // #250 (PR #384)
	"nameInput":    expectBool,   // #250 (PR #384)
	"meFilter":     expectBool,   // #251 (PR #384)
	"scopeNot":     expectString, // #254 (PR #405)
	"summaryHas":   expectStrings,
	"prefillRef":   expectBool, // #256 (PR #387)
	// checkMarks (#255, PR #398)
	"mark":       expectBool,
	"markedByMe": expectBool,
	"markEntity": expectBool,
	// checkGroupBy (#261, PR #404)
	"groupedTable": expectBool,
	// checkRoute (#411)
	"routeDest": expectString,
	"routeMode": expectString,
	// checkDashcam (#410)
	"dashcamAuto": expectBool,
	// checkExport (#421)
	"export": expectString,
	// checkSensor (#419)
	"sensor":          expectStrings,
	"sensorSave":      expectBool,
	"sensorThreshold": expectBool,
	"sensorDecline":   expectBool,
	// checkLiveActivity (#423)
	"liveTimer":       expectBool,
	"activityButtons": expectBool,
	// checkQR (#422)
	"qrSource": expectString,
	// checkPhotoAI (#418)
	"photoAnalyze": expectStrings,
	// checkAudio (#420)
	"audio":       expectBool,
	"audioInList": expectBool,
	// checkAIGenerate (#417): aigen.go
	"aiGenerate":       expectString, // line|list|fields|any
	"aiGenerateEntity": expectBool,
	// checkNav
	"patch":       expectBool,
	"unreachable": expectNumber,
	"nav":         expectStrings,
	"tabs":        expectNumber,
	"tabsMin":     expectNumber,
	"tabOrder":    expectString,
	"tabHas":      expectString,
	"refs":        expectBool,
	"deletable":   expectStrings,
	"has":         expectStrings,
	"lacks":       expectStrings,
	// checkEnum
	"enumField": expectString,
	"optHas":    expectStrings,
	"optNot":    expectStrings,
	"fields":    expectNumber,
	"refPick":   expectBool,
	// checkKeep
	"keepEntity": expectString,
	"keep":       expectStrings,
	"growTo":     expectNumber,
	// checkExpect (#416)
	"oneAdd": expectBool,
}

var hhmm = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

// validateExpect reports unknown keys, wrong JSON types and bad values.
func validateExpect(exp map[string]any) []string {
	var bad []string
	for k, v := range exp {
		kind, ok := expectKeys[k]
		if !ok {
			bad = append(bad, "unknown key "+k)
			continue
		}
		switch kind {
		case expectBool:
			_, ok = v.(bool)
		case expectString:
			_, ok = v.(string)
		case expectNumber:
			_, ok = v.(float64)
		case expectStrings:
			var arr []any
			arr, ok = v.([]any)
			for _, s := range arr {
				if _, isStr := s.(string); !isStr {
					ok = false
				}
			}
			ok = ok && len(arr) > 0
		}
		if !ok {
			bad = append(bad, fmt.Sprintf("%s has the wrong type (%T)", k, v))
			continue
		}
		switch k {
		case "notification":
			if s := v.(string); s != "any" && !hhmm.MatchString(s) {
				bad = append(bad, "notification must be any or HH:MM: "+s)
			}
		case "scope":
			if s := v.(string); s != "public" && s != "shared" && s != "private" {
				bad = append(bad, "scope must be public|shared|private: "+s)
			}
		case "routeDest":
			if s := v.(string); !slices.Contains(routeDests, s) {
				bad = append(bad, "routeDest must be entity|record|address|any: "+s)
			}
		case "routeMode":
			if s := v.(string); !slices.Contains(routeModes, s) {
				bad = append(bad, "routeMode must be automobile|walking: "+s)
			}
		case "export":
			if s := v.(string); s != "csv" && s != "pdf" && s != "both" {
				bad = append(bad, "export must be csv|pdf|both: "+s)
			}
		case "qrSource":
			if s := v.(string); !qrSources[s] {
				bad = append(bad, "qrSource must be text|field|value|record|any: "+s)
			}
		case "aiGenerate":
			if s := v.(string); s != "any" && !dsl.AIShapes[s] {
				bad = append(bad, "aiGenerate must be line|list|fields|any: "+s)
			}
		}
	}
	if _, ok := exp["keepEntity"]; !ok && (exp["keep"] != nil || exp["growTo"] != nil) {
		bad = append(bad, "keep / growTo need keepEntity")
	}
	if _, ok := exp["enumField"]; !ok && (exp["optHas"] != nil || exp["optNot"] != nil || exp["fields"] != nil || exp["refPick"] != nil) {
		bad = append(bad, "optHas / optNot / fields / refPick need enumField")
	}
	sort.Strings(bad)
	return bad
}

// validateCases checks ids, kinds, edit bases and every expect.
func validateCases(cases []Case) []string {
	var bad []string
	seen := map[string]bool{}
	for _, c := range cases {
		where := "[" + c.ID + "] "
		if c.ID == "" {
			bad = append(bad, "case without id")
		}
		if seen[c.ID] {
			bad = append(bad, where+"duplicate id")
		}
		switch c.Kind {
		case "create":
		case "edit":
			// a base is an earlier case in the run or an example file
			if c.Base != "" && !strings.HasPrefix(c.Base, "example:") && !seen[c.Base] {
				bad = append(bad, where+"base "+c.Base+" is not an earlier case")
			}
		default:
			bad = append(bad, where+"kind must be create|edit: "+c.Kind)
		}
		if strings.TrimSpace(c.Transcript) == "" {
			bad = append(bad, where+"empty transcript")
		}
		for _, b := range validateExpect(c.Expect) {
			bad = append(bad, where+b)
		}
		seen[c.ID] = true
	}
	return bad
}

// checkCasesFile decodes a cases file strictly (unknown fields fail) and
// validates it as generate cases, or chat cases with chat.
func checkCasesFile(path string, chat bool) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var bad []string
	if chat {
		var cases []ChatCase
		if err := dec.Decode(&cases); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		bad = validateChatCases(cases)
	} else {
		var cases []Case
		if err := dec.Decode(&cases); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		bad = validateCases(cases)
	}
	if len(bad) > 0 {
		return fmt.Errorf("%s:\n  %s", path, strings.Join(bad, "\n  "))
	}
	return nil
}

var chatKinds = map[string]bool{"": true, "question": true, "ready": true, "answer": true, "proposal": true}

// validateChatCases checks ids and each step's expect kind.
func validateChatCases(cases []ChatCase) []string {
	var bad []string
	seen := map[string]bool{}
	for _, c := range cases {
		where := "[" + c.ID + "] "
		if c.ID == "" || seen[c.ID] {
			bad = append(bad, where+"missing or duplicate id")
		}
		seen[c.ID] = true
		if len(c.Steps) == 0 {
			bad = append(bad, where+"no steps")
		}
		for i, st := range c.Steps {
			if strings.TrimSpace(st.Say) == "" {
				bad = append(bad, fmt.Sprintf("%sstep %d: empty say", where, i+1))
			}
			if !chatKinds[st.Expect] {
				bad = append(bad, fmt.Sprintf("%sstep %d: expect must be question|ready|answer|proposal: %s", where, i+1, st.Expect))
			}
		}
	}
	return bad
}

// groupPass is one row of the pass rate: a case passes when it ran, had an
// expect and missed nothing. Cases without expect are counted apart.
type groupPass struct {
	Group     string
	Checked   int
	Pass      int
	Unchecked int
}

func passByGroup(recs []Record, hasExpect map[string]bool) []groupPass {
	rows := map[string]*groupPass{}
	var order []string
	for _, r := range recs {
		g := rows[r.Group]
		if g == nil {
			g = &groupPass{Group: r.Group}
			rows[r.Group] = g
			order = append(order, r.Group)
		}
		if !hasExpect[r.ID] {
			g.Unchecked++
			continue
		}
		g.Checked++
		if r.OK && len(r.ExpectMisses) == 0 {
			g.Pass++
		}
	}
	out := make([]groupPass, 0, len(order)+1)
	total := groupPass{Group: "total"}
	for _, name := range order {
		g := *rows[name]
		out = append(out, g)
		total.Checked += g.Checked
		total.Pass += g.Pass
		total.Unchecked += g.Unchecked
	}
	return append(out, total)
}

func printPassByGroup(rows []groupPass) {
	fmt.Println("\n| group | pass / checked | pass% | no expect |")
	fmt.Println("|---|---|---|---|")
	for _, g := range rows {
		rate := "-"
		if g.Checked > 0 {
			rate = fmt.Sprintf("%.0f%%", float64(g.Pass)*100/float64(g.Checked))
		}
		fmt.Printf("| %s | %d / %d | %s | %d |\n", g.Group, g.Pass, g.Checked, rate, g.Unchecked)
	}
}
