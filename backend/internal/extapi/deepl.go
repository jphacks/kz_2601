package extapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// DeepL API Pro (#1500): translate a text. Paid per character: a call is
// one `runs` and coins (hold COIN_RESERVE_EXT_DEEPL → commit the cost → the
// rest back).
//
//	translate(text, targetLang, sourceLang?)   Item: the translation / "EN → JA"
//
// The cost is characters × EXT_COST_USD_DEEPL_PER_CHAR: the characters
// DeepL billed (show_billed_characters), or the text's characters when the
// answer has no count (the estimate). Without EXT_COST_USD_DEEPL_PER_CHAR
// the op does not price the call and the foundation charges
// EXT_COST_USD_DEEPL_TRANSLATE or the hold. Set the hold to cover
// deeplMaxBytes characters, since a commit never goes over it.
//
// Terms: Pro only (Pro does not keep or train on the texts; the free API's
// data use and commercial use are unclear, docs/decisions/external-apis.md),
// so a free key (":fx") keeps the provider off and only api.deepl.com is
// reached. Texts are never cached or logged. The key goes in the
// Authorization header, never in a URL.
const (
	DeepL = "deepl"

	deeplHost = "https://api.deepl.com"
	// deeplMaxBytes caps the text: 4 KB, as /v1/text.
	deeplMaxBytes = 4096
)

var (
	deeplKeyRe    = regexp.MustCompile(`^[A-Za-z0-9-]{16,64}$`)
	deeplTargetRe = regexp.MustCompile(`^[A-Z]{2,3}(-[A-Z0-9]{2,4})?$`)
	deeplSourceRe = regexp.MustCompile(`^[A-Z]{2,3}$`)
)

func init() {
	register(Provider{ID: DeepL, Billing: BillingPaid,
		Ops:   []OpSpec{{Name: "translate", Args: []string{"text", "targetLang", "sourceLang"}}},
		Hosts: []string{deeplHost},
		New: func(sec Secrets) (map[string]Operation, error) {
			perChar, err := deeplPerChar(os.Getenv("EXT_COST_USD_DEEPL_PER_CHAR"))
			if err != nil {
				return nil, err
			}
			return newDeepL(sec.APIKey, deeplHost, perChar, NewHTTPClient([]string{deeplHost}, DefaultTimeout))
		}})
}

// deeplPerChar reads EXT_COST_USD_DEEPL_PER_CHAR (-1 = not set).
func deeplPerChar(v string) (float64, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return -1, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f < 0 || math.IsInf(f, 0) || math.IsNaN(f) {
		return 0, fmt.Errorf("EXT_COST_USD_DEEPL_PER_CHAR must be a number ≥ 0, got %q", v)
	}
	return f, nil
}

// newDeepL builds the op against base (the API, or a test server).
// perChar < 0: the op does not price the call.
func newDeepL(key, base string, perChar float64, hc *http.Client) (map[string]Operation, error) {
	switch {
	case strings.HasSuffix(key, ":fx"):
		return nil, errors.New("EXT_DEEPL_API_KEY is a free-API key: only DeepL API Pro is used")
	case !deeplKeyRe.MatchString(key):
		return nil, errors.New("EXT_DEEPL_API_KEY is not set or not a key")
	}
	d := &deepl{key: key, base: base, perChar: perChar, hc: hc}
	return map[string]Operation{"translate": OperationFunc(d.translate)}, nil
}

type deepl struct {
	key     string
	base    string
	perChar float64
	hc      *http.Client
}

type deeplRequest struct {
	Text                 []string `json:"text"`
	TargetLang           string   `json:"target_lang"`
	SourceLang           string   `json:"source_lang,omitempty"`
	ShowBilledCharacters bool     `json:"show_billed_characters"`
}

type deeplAnswer struct {
	Translations []struct {
		DetectedSourceLanguage string `json:"detected_source_language"`
		Text                   string `json:"text"`
		BilledCharacters       *int   `json:"billed_characters"`
	} `json:"translations"`
}

func (d *deepl) translate(ctx context.Context, in Input) (Output, error) {
	text := in.Args["text"]
	target := strings.ToUpper(strings.TrimSpace(in.Args["targetLang"]))
	source := strings.ToUpper(strings.TrimSpace(in.Args["sourceLang"]))
	switch {
	case strings.TrimSpace(text) == "" || len(text) > deeplMaxBytes || !utf8.ValidString(text):
		return Output{}, fmt.Errorf("%w: text must be 1-%d bytes of UTF-8", ErrBadArgs, deeplMaxBytes)
	case !deeplTargetRe.MatchString(target):
		return Output{}, fmt.Errorf("%w: targetLang", ErrBadArgs)
	case source != "" && !deeplSourceRe.MatchString(source):
		return Output{}, fmt.Errorf("%w: sourceLang", ErrBadArgs)
	}
	body, err := json.Marshal(deeplRequest{Text: []string{text}, TargetLang: target, SourceLang: source, ShowBilledCharacters: true})
	if err != nil {
		return Output{}, errors.New("deepl: building the request failed")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.base+"/v2/translate", bytes.NewReader(body))
	if err != nil {
		return Output{}, errors.New("deepl: building the request failed")
	}
	req.Header.Set("Authorization", "DeepL-Auth-Key "+d.key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	var a deeplAnswer
	if err := sendJSON(ctx, d.hc, req, &a); err != nil {
		return Output{}, err
	}
	if len(a.Translations) == 0 {
		return Output{}, &ProviderError{Status: http.StatusOK, Err: errors.New("deepl: no translation")}
	}
	t := a.Translations[0]
	from := source
	if from == "" {
		from = strings.ToUpper(t.DetectedSourceLanguage)
	}
	out := Output{Item: &Row{Title: t.Text, Subtitle: strings.TrimSpace(from + " → " + target), Value: t.Text}}
	if d.perChar >= 0 {
		chars := utf8.RuneCountInString(text) // the estimate
		if t.BilledCharacters != nil && *t.BilledCharacters >= 0 {
			chars = *t.BilledCharacters
		}
		out.Usage = Usage{USD: float64(chars) * d.perChar, Priced: true}
	}
	return out, nil
}
