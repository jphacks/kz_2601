package extapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Google Cloud Translation (Basic, v2): translate a text, guess its
// language, list the languages. Paid per character: a call is one `runs`
// and coins (hold COIN_RESERVE_EXT_GOOGLE_TRANSLATE → commit the cost → the
// rest back). docs/decisions/external-apis.md candidate 14.
//
//	translate(text, targetLang, sourceLang?)   Item: the translation / "en → ja"
//	detect(text)                               Item: the language / confidence
//	languages(displayLang?)                    Rows: code / name (cached 1 day)
//
// The cost of translate / detect is the text's characters ×
// EXT_COST_USD_GOOGLE_TRANSLATE_PER_CHAR (Google bills the characters sent;
// the answer has no count). Without it the op does not price the call and
// the foundation charges EXT_COST_USD_GOOGLE_TRANSLATE_<OP> or the hold.
// languages is not billed by Google (priced 0).
//
// Texts are never cached or logged (the answer cache is shared and keyed by
// the arguments); only the language list is cached. The key goes in the
// X-Goog-Api-Key header, never in a URL.
const (
	GoogleTranslate = "googleTranslate"

	googleTranslateHost = "https://translation.googleapis.com"
	googleTranslatePath = "/language/translate/v2"
	// googleTranslateMaxBytes caps the text (Google advises at most 5K
	// characters a request).
	googleTranslateMaxBytes = 4096
	googleTranslateLangTTL  = 24 * time.Hour
)

var (
	googleTranslateKeyRe  = regexp.MustCompile(`^[A-Za-z0-9_-]{20,64}$`)
	googleTranslateLangRe = regexp.MustCompile(`^[a-z]{2,3}(-[A-Za-z0-9]{2,4})?$`)
)

func init() {
	register(Provider{ID: GoogleTranslate, Billing: BillingPaid,
		Ops: []OpSpec{
			{Name: "translate", Args: []string{"text", "targetLang", "sourceLang"}, Check: checkGoogleTranslate},
			{Name: "detect", Args: []string{"text"}, Check: checkGoogleTranslateDetect},
			{Name: "languages", Args: []string{"displayLang"}, Check: checkGoogleTranslateLanguages, CacheTTL: googleTranslateLangTTL},
		},
		Hosts: []string{googleTranslateHost},
		New: func(sec Secrets) (map[string]Operation, error) {
			perChar, err := googleTranslatePerChar(os.Getenv("EXT_COST_USD_GOOGLE_TRANSLATE_PER_CHAR"))
			if err != nil {
				return nil, err
			}
			return newGoogleTranslate(sec.APIKey, googleTranslateHost, perChar, NewHTTPClient([]string{googleTranslateHost}, DefaultTimeout))
		}})
}

// googleTranslatePerChar reads EXT_COST_USD_GOOGLE_TRANSLATE_PER_CHAR (-1 = not set).
func googleTranslatePerChar(v string) (float64, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return -1, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f < 0 || math.IsInf(f, 0) || math.IsNaN(f) {
		return 0, fmt.Errorf("EXT_COST_USD_GOOGLE_TRANSLATE_PER_CHAR must be a number ≥ 0, got %q", v)
	}
	return f, nil
}

// googleTranslateText: 1-4096 bytes of UTF-8, not blank.
func googleTranslateText(v string) error {
	if strings.TrimSpace(v) == "" || len(v) > googleTranslateMaxBytes || !utf8.ValidString(v) {
		return fmt.Errorf("text must be 1-%d bytes of UTF-8", googleTranslateMaxBytes)
	}
	return nil
}

// googleTranslateLang: a language code (ja, en, zh-CN ...), "" when empty.
func googleTranslateLang(name, v string, required bool) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" && !required {
		return "", nil
	}
	if p, rest, ok := strings.Cut(v, "-"); ok {
		v = strings.ToLower(p) + "-" + rest
	} else {
		v = strings.ToLower(v)
	}
	if !googleTranslateLangRe.MatchString(v) {
		return "", fmt.Errorf("%s must be a language code (ja, en, zh-CN ...)", name)
	}
	return v, nil
}

func checkGoogleTranslate(a map[string]string) error {
	if err := googleTranslateText(a["text"]); err != nil {
		return err
	}
	if _, err := googleTranslateLang("targetLang", a["targetLang"], true); err != nil {
		return err
	}
	_, err := googleTranslateLang("sourceLang", a["sourceLang"], false)
	return err
}

func checkGoogleTranslateDetect(a map[string]string) error { return googleTranslateText(a["text"]) }

func checkGoogleTranslateLanguages(a map[string]string) error {
	_, err := googleTranslateLang("displayLang", a["displayLang"], false)
	return err
}

// newGoogleTranslate builds the ops against base (the API, or a test
// server). perChar < 0: the ops do not price the call.
func newGoogleTranslate(key, base string, perChar float64, hc *http.Client) (map[string]Operation, error) {
	if !googleTranslateKeyRe.MatchString(key) {
		return nil, errors.New("EXT_GOOGLE_TRANSLATE_API_KEY is not set or not a key")
	}
	g := &googleTranslate{key: key, base: strings.TrimRight(base, "/"), perChar: perChar, hc: hc}
	return map[string]Operation{
		"translate": OperationFunc(g.translate),
		"detect":    OperationFunc(g.detect),
		"languages": OperationFunc(g.languages),
	}, nil
}

type googleTranslate struct {
	key     string
	base    string
	perChar float64
	hc      *http.Client
}

type googleTranslateAnswer struct {
	Data struct {
		Translations []struct {
			TranslatedText         string `json:"translatedText"`
			DetectedSourceLanguage string `json:"detectedSourceLanguage"`
		} `json:"translations"`
		Detections [][]struct {
			Language   string  `json:"language"`
			Confidence float64 `json:"confidence"`
		} `json:"detections"`
		Languages []struct {
			Language string `json:"language"`
			Name     string `json:"name"`
		} `json:"languages"`
	} `json:"data"`
}

func (g *googleTranslate) usage(text string) Usage {
	if g.perChar < 0 {
		return Usage{}
	}
	return Usage{USD: float64(utf8.RuneCountInString(text)) * g.perChar, Priced: true}
}

func (g *googleTranslate) translate(ctx context.Context, in Input) (Output, error) {
	if err := checkGoogleTranslate(in.Args); err != nil {
		return Output{}, fmt.Errorf("%w: %v", ErrBadArgs, err)
	}
	text := in.Args["text"]
	target, _ := googleTranslateLang("targetLang", in.Args["targetLang"], true)
	source, _ := googleTranslateLang("sourceLang", in.Args["sourceLang"], false)
	body := map[string]any{"q": []string{text}, "target": target, "format": "text"}
	if source != "" {
		body["source"] = source
	}
	var a googleTranslateAnswer
	if err := g.send(ctx, http.MethodPost, googleTranslatePath, body, &a); err != nil {
		return Output{}, err
	}
	if len(a.Data.Translations) == 0 {
		return Output{}, &ProviderError{Status: http.StatusOK, Err: errors.New("googleTranslate: no translation")}
	}
	t := a.Data.Translations[0]
	from := source
	if from == "" {
		from = strings.TrimSpace(t.DetectedSourceLanguage)
	}
	out := Output{Item: &Row{Title: t.TranslatedText, Subtitle: strings.TrimSpace(from + " → " + target), Value: t.TranslatedText}}
	out.Usage = g.usage(text)
	return out, nil
}

func (g *googleTranslate) detect(ctx context.Context, in Input) (Output, error) {
	if err := checkGoogleTranslateDetect(in.Args); err != nil {
		return Output{}, fmt.Errorf("%w: %v", ErrBadArgs, err)
	}
	text := in.Args["text"]
	var a googleTranslateAnswer
	if err := g.send(ctx, http.MethodPost, googleTranslatePath+"/detect", map[string]any{"q": []string{text}}, &a); err != nil {
		return Output{}, err
	}
	if len(a.Data.Detections) == 0 || len(a.Data.Detections[0]) == 0 || a.Data.Detections[0][0].Language == "" {
		return Output{}, &ProviderError{Status: http.StatusOK, Err: errors.New("googleTranslate: no detection")}
	}
	d := a.Data.Detections[0][0]
	sub := ""
	if d.Confidence > 0 && d.Confidence <= 1 {
		sub = "確からしさ " + strconv.Itoa(int(math.Round(d.Confidence*100))) + "%"
	}
	return Output{Item: &Row{Title: d.Language, Subtitle: sub, Value: d.Language}, Usage: g.usage(text)}, nil
}

func (g *googleTranslate) languages(ctx context.Context, in Input) (Output, error) {
	display, err := googleTranslateLang("displayLang", in.Args["displayLang"], false)
	if err != nil {
		return Output{}, fmt.Errorf("%w: %v", ErrBadArgs, err)
	}
	if display == "" {
		display = "ja"
	}
	var a googleTranslateAnswer
	if err := g.send(ctx, http.MethodGet, googleTranslatePath+"/languages?"+url.Values{"target": {display}}.Encode(), nil, &a); err != nil {
		return Output{}, err
	}
	rows := make([]Row, 0, len(a.Data.Languages))
	for _, l := range a.Data.Languages {
		if l.Language == "" {
			continue
		}
		name := clip(strings.TrimSpace(l.Name), 80)
		if name == "" {
			name = l.Language
		}
		rows = append(rows, Row{Title: name, Value: l.Language})
	}
	return Output{Rows: rows, Usage: Usage{Priced: true}}, nil
}

// send calls the API with the key in the header. A 400 (a language Google
// does not know ...) is the caller's bad argument; it is not billed.
func (g *googleTranslate) send(ctx context.Context, method, path string, body any, out any) error {
	var rd *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return errors.New("googleTranslate: building the request failed")
		}
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, g.base+path, rd)
	if err != nil {
		return errors.New("googleTranslate: building the request failed")
	}
	req.Header.Set("X-Goog-Api-Key", g.key)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	err = sendJSON(ctx, g.hc, req, out)
	var pe *ProviderError
	if errors.As(err, &pe) && pe.Status == http.StatusBadRequest {
		return fmt.Errorf("%w: googleTranslate refused the arguments", ErrUnknown)
	}
	return err
}
