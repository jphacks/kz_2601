package triage

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/jphacks/kz_2601/backend/internal/loop/pii"
)

// Categories (#366 decision 3). There is deliberately no value that means
// "approved" or names a GitHub label: the model cannot emit one.
var Categories = []string{"bug", "feature", "question", "praise", "spam"}

// Areas a piece of feedback is about.
var Areas = []string{"generate", "render", "share", "billing", "other"}

// Impacts are the model's suggestion; the priority itself is computed (Priority).
var Impacts = []string{"high", "medium", "low"}

// MaxSummaryRunes bounds one summary (#366 検査器 1).
const MaxSummaryRunes = 200

// schemaJSON is the only output shape accepted (structured output, and the
// first layer of 検査器 1).
var schemaJSON = []byte(`{
  "type": "object",
  "additionalProperties": false,
  "required": ["items"],
  "properties": {
    "items": {
      "type": "array",
      "maxItems": 50,
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["id", "category", "area", "impact", "cluster", "summary", "pii_suspected"],
        "properties": {
          "id": {"type": "string", "pattern": "^i[0-9]{1,3}$"},
          "category": {"enum": ["bug", "feature", "question", "praise", "spam"]},
          "area": {"enum": ["generate", "render", "share", "billing", "other"]},
          "impact": {"enum": ["high", "medium", "low"]},
          "cluster": {"type": "string", "pattern": "^[a-z0-9][a-z0-9-]{0,39}$"},
          "summary": {"type": "string", "minLength": 1, "maxLength": 200},
          "pii_suspected": {"type": "boolean"}
        }
      }
    }
  }
}`)

// wireSchemaJSON is what the model is given as output_config.format: the
// same shape without the length and pattern limits, which structured output
// does not enforce. Parse still checks the full schema.
var wireSchemaJSON = []byte(`{
  "type": "object",
  "additionalProperties": false,
  "required": ["items"],
  "properties": {
    "items": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["id", "category", "area", "impact", "cluster", "summary", "pii_suspected"],
        "properties": {
          "id": {"type": "string"},
          "category": {"type": "string", "enum": ["bug", "feature", "question", "praise", "spam"]},
          "area": {"type": "string", "enum": ["generate", "render", "share", "billing", "other"]},
          "impact": {"type": "string", "enum": ["high", "medium", "low"]},
          "cluster": {"type": "string"},
          "summary": {"type": "string"},
          "pii_suspected": {"type": "boolean"}
        }
      }
    }
  }
}`)

// SchemaJSON is the output schema sent to the model.
func SchemaJSON() json.RawMessage { return wireSchemaJSON }

var compiled = func() *jsonschema.Schema {
	c := jsonschema.NewCompiler()
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaJSON))
	if err != nil {
		panic("triage: schema is not JSON: " + err.Error())
	}
	if err := c.AddResource("triage.schema.json", doc); err != nil {
		panic("triage: add schema: " + err.Error())
	}
	s, err := c.Compile("triage.schema.json")
	if err != nil {
		panic("triage: compile schema: " + err.Error())
	}
	return s
}()

// Verdict is one validated row of the model's output.
type Verdict struct {
	ID           string `json:"id"`
	Category     string `json:"category"`
	Area         string `json:"area"`
	Impact       string `json:"impact"`
	Cluster      string `json:"cluster"`
	Summary      string `json:"summary"`
	PIISuspected bool   `json:"pii_suspected"`
}

type output struct {
	Items []Verdict `json:"items"`
}

// ErrOutput is a rejected response: nothing in it is used.
var ErrOutput = errors.New("triage: output rejected")

// Parse validates a whole response (#366 検査器 1): the schema, ids that were
// sent (each at most once, never more rows than inputs). A row whose summary
// fails CheckSummary is returned in bad (its item is held), the rest in good.
// Any other failure rejects the whole response (fail closed).
func Parse(raw string, sent map[string]bool) (good []Verdict, bad map[string]string, err error) {
	doc, err := jsonschema.UnmarshalJSON(strings.NewReader(raw))
	if err != nil {
		return nil, nil, fmt.Errorf("%w: not JSON", ErrOutput)
	}
	if err := compiled.Validate(doc); err != nil {
		return nil, nil, fmt.Errorf("%w: schema", ErrOutput)
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	var out output
	if err := dec.Decode(&out); err != nil {
		return nil, nil, fmt.Errorf("%w: decode", ErrOutput)
	}
	if len(out.Items) > len(sent) {
		return nil, nil, fmt.Errorf("%w: more rows than inputs", ErrOutput)
	}
	seen := map[string]bool{}
	bad = map[string]string{}
	for _, v := range out.Items {
		if !sent[v.ID] {
			return nil, nil, fmt.Errorf("%w: unknown id", ErrOutput)
		}
		if seen[v.ID] {
			return nil, nil, fmt.Errorf("%w: duplicate id", ErrOutput)
		}
		seen[v.ID] = true
		if reason := CheckSummary(v.Summary); reason != "" {
			bad[v.ID] = reason
			continue
		}
		good = append(good, v)
	}
	return good, bad, nil
}

var (
	reURL     = regexp.MustCompile(`(?i)[a-z][a-z0-9+.\-]*://|www\.|\b[a-z0-9\-]+\.(?:com|net|org|io|dev|app|jp|co|me|ai|xyz)\b`)
	reMention = regexp.MustCompile(`@[A-Za-z0-9_\-]`)
	reHash    = regexp.MustCompile(`#[^0-9]|#$`)
	reHTML    = regexp.MustCompile(`<[^>]*>|&[a-zA-Z#0-9]+;`)
	reLabel   = regexp.MustCompile(`(?i)\b(?:agent|needs|eval)\s*:\s*[a-z]`)
)

// CheckSummary returns why s may not be stored or shown ("" = fine). It is
// the text part of 検査器 1 and is shared by the draft exit check.
func CheckSummary(s string) string {
	switch {
	case strings.TrimSpace(s) == "":
		return "empty"
	case utf8.RuneCountInString(s) > MaxSummaryRunes:
		return "too_long"
	case strings.Count(s, "\n") > 2:
		return "too_many_lines"
	case pii.HasInvisible(s):
		return "invisible_chars"
	case strings.Contains(s, "`") || strings.Contains(s, "```"):
		return "code"
	case reHTML.MatchString(s):
		return "html"
	case reURL.MatchString(s):
		return "url"
	case reMention.MatchString(s):
		return "mention"
	case reHash.MatchString(s):
		return "reference"
	case reLabel.MatchString(s):
		return "label_like"
	case len(pii.Find(s)) > 0:
		return "pii"
	}
	return ""
}
