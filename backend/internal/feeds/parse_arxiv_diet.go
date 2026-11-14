package feeds

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// ---- arxiv (export.arxiv.org/api/query, Atom) --------------------------------

type arxivFeed struct {
	XMLName xml.Name `xml:"http://www.w3.org/2005/Atom feed"`
	Entries []struct {
		ID        string    `xml:"id"`
		Title     string    `xml:"title"`
		Published string    `xml:"published"`
		Links     []xmlLink `xml:"link"`
		Authors   []struct {
			Name string `xml:"name"`
		} `xml:"author"`
	} `xml:"entry"`
}

// arxivMaxAuthors is how many author names the subtitle lists.
const arxivMaxAuthors = 3

// parseArxiv keeps title, the abstract page link, the first submission time
// and up to 3 authors. Abstracts are not kept (only metadata is shown).
func parseArxiv(body []byte) ([]Item, error) {
	var f arxivFeed
	if err := xml.NewDecoder(bytes.NewReader(body)).Decode(&f); err != nil {
		return nil, fmt.Errorf("arxiv: %w", err)
	}
	out := make([]Item, 0, len(f.Entries))
	for _, e := range f.Entries {
		title := clean(e.Title, maxTitleRunes)
		if title == "" {
			continue
		}
		it := Item{Title: title, URL: arxivLink(e.Links, e.ID)}
		if t, ok := parseFeedTime(e.Published); ok {
			it.Date = t.In(jst).Format(time.RFC3339)
		}
		var names []string
		for _, a := range e.Authors {
			if n := clean(a.Name, 40); n != "" {
				names = append(names, n)
			}
		}
		if len(names) > arxivMaxAuthors {
			names = append(names[:arxivMaxAuthors], "ほか")
		}
		it.Subtitle = clean(strings.Join(names, ", "), maxSubtitleRunes)
		out = append(out, it)
	}
	return out, nil
}

// arxivLink is the abstract page: the alternate link, else the entry id.
// The API may write them as http://arxiv.org/...; those become https.
func arxivLink(ls []xmlLink, id string) string {
	for _, l := range ls {
		if l.Rel == "" || l.Rel == "alternate" {
			if u := httpsURL(upgradeArxiv(l.Href)); u != "" {
				return u
			}
		}
	}
	return httpsURL(upgradeArxiv(id))
}

// upgradeArxiv turns http on arxiv.org / export.arxiv.org into https.
func upgradeArxiv(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "http" || (u.Host != "arxiv.org" && u.Host != "export.arxiv.org") {
		return raw
	}
	u.Scheme = "https"
	return u.String()
}

// ---- diet (kokkai.ndl.go.jp/api/speech, JSON) ---------------------------------

type dietResponse struct {
	NumberOfRecords *int `json:"numberOfRecords"`
	Message         any  `json:"message"`
	SpeechRecord    []struct {
		Speaker       string `json:"speaker"`
		SpeakerGroup  string `json:"speakerGroup"`
		NameOfHouse   string `json:"nameOfHouse"`
		NameOfMeeting string `json:"nameOfMeeting"`
		Issue         string `json:"issue"`
		Date          string `json:"date"`
		SpeechOrder   int    `json:"speechOrder"`
		SpeechURL     string `json:"speechURL"`
	} `json:"speechRecord"`
}

// parseDiet makes one row per speech: speaker (会派), house + meeting +
// issue, the meeting date and the speech's page. The speech text is not
// kept. Order 0 is the meeting's header record (出席者の一覧), not a speech.
func parseDiet(body []byte) ([]Item, error) {
	var r dietResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("diet: %w", err)
	}
	if r.NumberOfRecords == nil {
		// An error answer carries "message" instead of the counts.
		return nil, errors.New("diet: not a search result")
	}
	out := make([]Item, 0, len(r.SpeechRecord))
	for _, s := range r.SpeechRecord {
		speaker := clean(s.Speaker, 40)
		if speaker == "" || s.SpeechOrder == 0 {
			continue
		}
		if g := clean(s.SpeakerGroup, 40); g != "" {
			speaker += "(" + g + ")"
		}
		it := Item{Title: clean(speaker, maxTitleRunes), URL: httpsURL(s.SpeechURL),
			Subtitle: clean(strings.Join([]string{s.NameOfHouse, s.NameOfMeeting, s.Issue}, " "), maxSubtitleRunes)}
		if d, err := time.Parse(time.DateOnly, strings.TrimSpace(s.Date)); err == nil {
			it.Date = d.Format(time.DateOnly)
		}
		out = append(out, it)
	}
	return out, nil
}
