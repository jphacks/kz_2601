package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// Pass-rate gate and Markdown report (#371). -min-pass and -thresholds make
// -score and a generate run exit non-zero when a pass rate is below its
// minimum; -report md writes the results as a Markdown table for a PR
// comment. Without these flags the output and exit code are unchanged.

// gate holds the minimum pass rate (0..1) per group; "total" is every checked
// case. A group named here that has no checked case fails: a typo or an
// empty run must not pass silently.
type gate map[string]float64

// loadGate merges -thresholds (a JSON object {"group": rate}) and -min-pass
// (the total). When both name the total, the stricter one wins.
func loadGate(minPass float64, file string) (gate, error) {
	g := gate{}
	if file != "" {
		raw, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &g); err != nil {
			return nil, fmt.Errorf("%s: want a JSON object {\"group\": 0.9}: %w", file, err)
		}
	}
	if minPass != 0 {
		g["total"] = max(g["total"], minPass)
	}
	var bad []string
	for name, v := range g {
		if v < 0 || v > 1 {
			bad = append(bad, fmt.Sprintf("%s: %v is not a rate between 0 and 1", name, v))
		}
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		return nil, errors.New("thresholds: " + strings.Join(bad, "; "))
	}
	return g, nil
}

// gateRow is the verdict of one group with a minimum.
type gateRow struct {
	groupPass
	Min float64
	OK  bool
}

func (r gateRow) rate() string {
	if r.Checked == 0 {
		return "-"
	}
	return fmt.Sprintf("%.0f%%", float64(r.Pass)*100/float64(r.Checked))
}

// check returns one row per group in g, in the order of rows (groups missing
// from rows last, sorted). A group passes when pass/checked >= its minimum.
func (g gate) check(rows []groupPass) []gateRow {
	seen := map[string]bool{}
	var out []gateRow
	add := func(p groupPass, min float64) {
		ok := p.Checked > 0 && float64(p.Pass)+1e-9 >= min*float64(p.Checked)
		out = append(out, gateRow{groupPass: p, Min: min, OK: ok})
	}
	for _, p := range rows {
		if min, ok := g[p.Group]; ok {
			add(p, min)
			seen[p.Group] = true
		}
	}
	var missing []string
	for name := range g {
		if !seen[name] {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	for _, name := range missing {
		add(groupPass{Group: name}, g[name])
	}
	return out
}

// gateError lists the groups below their minimum, or nil.
func gateError(rows []gateRow) error {
	var bad []string
	for _, r := range rows {
		if r.OK {
			continue
		}
		if r.Checked == 0 {
			bad = append(bad, fmt.Sprintf("%s: no checked case (min %.0f%%)", r.Group, r.Min*100))
			continue
		}
		bad = append(bad, fmt.Sprintf("%s: %s (%d / %d) < min %.0f%%", r.Group, r.rate(), r.Pass, r.Checked, r.Min*100))
	}
	if len(bad) == 0 {
		return nil
	}
	return errors.New("pass rate below the threshold:\n  " + strings.Join(bad, "\n  "))
}

// evalOutput is what -report / -report-out / -min-pass / -thresholds ask for.
type evalOutput struct {
	Format string // "" = plain text (today's output), "md" = Markdown report
	Out    string // -report-out: file for the report; empty = stdout
	Gate   gate
}

func (o evalOutput) active() bool { return o.Format != "" || len(o.Gate) > 0 }

func validateReportFormat(f string) error {
	if f != "" && f != "md" {
		return fmt.Errorf("-report: unknown format %q (want md)", f)
	}
	return nil
}

// emit prints the results and applies the gate. plain prints today's text
// output; it is skipped only when the Markdown report goes to stdout. The
// report is written before the gate fails, so CI can still post it.
func (o evalOutput) emit(stdout io.Writer, recs []Record, hasExpect map[string]bool, source string, plain func()) error {
	rows := passByGroup(recs, hasExpect)
	checked := o.Gate.check(rows)
	switch {
	case o.Format != "md":
		plain()
	case o.Out == "":
		if err := writeMarkdown(stdout, recs, hasExpect, rows, checked, source); err != nil {
			return err
		}
	default:
		plain()
		var b bytes.Buffer
		if err := writeMarkdown(&b, recs, hasExpect, rows, checked, source); err != nil {
			return err
		}
		if err := os.WriteFile(o.Out, b.Bytes(), 0o644); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(stdout, "\nwrote %s\n", o.Out); err != nil {
			return err
		}
	}
	return gateError(checked)
}

// writeMarkdown writes a self-contained report: a verdict line (when a gate
// is set), the pass rate per group with its minimum, and the cases that did
// not pass.
func writeMarkdown(w io.Writer, recs []Record, hasExpect map[string]bool, rows []groupPass, checked []gateRow, source string) error {
	mins := map[string]gateRow{}
	for _, r := range checked {
		mins[r.Group] = r
	}
	var b strings.Builder
	b.WriteString("## ai-eval\n\n")
	if source != "" {
		fmt.Fprintf(&b, "対象: `%s`\n\n", mdCell(source))
	}
	if len(checked) > 0 {
		if gateError(checked) == nil {
			b.WriteString("**判定: PASS**(すべてのグループが基準以上)\n\n")
		} else {
			b.WriteString("**判定: FAIL**(基準を下回るグループがある)\n\n")
		}
	}
	b.WriteString("| group | pass / checked | pass% | min | result | no expect |\n")
	b.WriteString("|---|---|---|---|---|---|\n")
	line := func(r gateRow, hasMin bool) {
		min, result := "-", "-"
		if hasMin {
			min = fmt.Sprintf("%.0f%%", r.Min*100)
			result = "FAIL"
			if r.OK {
				result = "PASS"
			}
		}
		fmt.Fprintf(&b, "| %s | %d / %d | %s | %s | %s | %d |\n", mdCell(r.Group), r.Pass, r.Checked, r.rate(), min, result, r.Unchecked)
	}
	inRows := map[string]bool{}
	for _, p := range rows {
		inRows[p.Group] = true
		g, hasMin := mins[p.Group]
		if !hasMin {
			g = gateRow{groupPass: p}
		}
		line(g, hasMin)
	}
	for _, r := range checked {
		if !inRows[r.Group] {
			line(r, true)
		}
	}
	var failed []string
	for _, r := range recs {
		switch {
		case !hasExpect[r.ID]:
		case !r.OK:
			failed = append(failed, fmt.Sprintf("| %s | %s | error: %s |", mdCell(r.ID), mdCell(r.Group), mdCell(r.Error)))
		case len(r.ExpectMisses) > 0:
			failed = append(failed, fmt.Sprintf("| %s | %s | %s |", mdCell(r.ID), mdCell(r.Group), mdCell(strings.Join(r.ExpectMisses, ", "))))
		}
	}
	if len(failed) > 0 {
		b.WriteString("\n<details><summary>通らなかったケース (" + fmt.Sprint(len(failed)) + ")</summary>\n\n")
		b.WriteString("| id | group | misses |\n|---|---|---|\n")
		b.WriteString(strings.Join(failed, "\n"))
		b.WriteString("\n\n</details>\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// mdCell keeps a value inside one table cell.
func mdCell(s string) string {
	s = strings.ReplaceAll(s, "|", `\|`)
	s = strings.ReplaceAll(s, "\r", " ")
	return strings.ReplaceAll(s, "\n", " ")
}
