package extapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Google Photos Picker API: the person picks photos in Google Photos and
// the app reads what was picked. Free (one `runs` a call), OAuth with PKCE.
// docs/decisions/external-apis.md candidate 24: since 2025-03-31 an app
// reads only the items the person chose in the Picker (no library scope).
//
//	startPicking(maxItems?)        Item: the session id / the Picker's link
//	pickedItems(sessionId, limit?) Item: the session's state; Rows: the items
//
// startPicking makes a Picker session; the app opens its link
// (openUrlButton) and keeps the id (Value) in a field. pickedItems answers
// 「まだ選んでいません」 until the person is done, then one row an item (title
// = the file name, subtitle = taken at (JST)・写真 / 動画・size, value = the
// media item id). The bytes' baseUrl needs the person's token, so it never
// goes back to the device. Nothing is cached (OAuth).
//
// Scope photospicker.mediaitems.readonly (sensitive: Google's verification
// is a human step). The OAuth client is Google Calendar's unless
// EXT_GOOGLE_PHOTOS_CLIENT_ID / _CLIENT_SECRET are set (ClientFrom).
//
// Env: EXT_GOOGLE_PHOTOS_ENABLED=1. Off by default; not in the review build.
const GooglePhotos = "googlePhotos"

const (
	gphotosScope        = "https://www.googleapis.com/auth/photospicker.mediaitems.readonly"
	gphotosPickerHost   = "photos.google.com"
	gphotosDefaultLimit = 25
	gphotosMaxLimit     = 100
	gphotosMaxItems     = 2000 // the Picker's own cap
)

var googlePhotosWire = gcalWire{
	AuthURL:   "https://accounts.google.com/o/oauth2/v2/auth",
	TokenURL:  "https://oauth2.googleapis.com/token",
	RevokeURL: "https://oauth2.googleapis.com/revoke",
	APIBase:   "https://photospicker.googleapis.com/v1",
}

var googlePhotosHosts = []string{"https://oauth2.googleapis.com", "https://photospicker.googleapis.com"}

var gphotosIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{8,128}$`)

func init() {
	register(Provider{ID: GooglePhotos, Billing: BillingFree, NeedsOAuth: true, NameJa: "Google フォト", NameEn: "Google Photos",
		Ops: []OpSpec{
			{Name: "startPicking", Scopes: []string{gphotosScope}, Args: []string{"maxItems"}, Check: checkGphotosStart},
			{Name: "pickedItems", Scopes: []string{gphotosScope}, Args: []string{"sessionId", "limit"}, Check: checkGphotosPicked},
		},
		Hosts:      googlePhotosHosts,
		ClientFrom: GoogleCalendar,
		New: func(sec Secrets) (map[string]Operation, error) {
			p, err := newGooglePhotos(sec, googlePhotosWire)
			if err != nil {
				return nil, err
			}
			return map[string]Operation{"startPicking": OperationFunc(p.startPicking), "pickedItems": OperationFunc(p.pickedItems)}, nil
		},
		NewOAuth: func(sec Secrets) (OAuthClient, error) {
			p, err := newGooglePhotos(sec, googlePhotosWire)
			if err != nil {
				return nil, err
			}
			return p.g, nil
		},
	})
}

// googlePhotos is the provider's ops; the OAuth client and do are Google
// Calendar's code on the Picker wire.
type googlePhotos struct {
	g *googleCalendar
}

func newGooglePhotos(sec Secrets, w gcalWire) (*googlePhotos, error) {
	g, err := newGoogleClient(GooglePhotos, sec, w, googlePhotosHosts)
	if err != nil {
		return nil, err
	}
	return &googlePhotos{g: g}, nil
}

func checkGphotosStart(a map[string]string) error {
	_, err := argInt(a, "maxItems", 0, 1, gphotosMaxItems)
	return err
}

func checkGphotosPicked(a map[string]string) error {
	if !gphotosIDRe.MatchString(strings.TrimSpace(a["sessionId"])) {
		return errors.New("sessionId must be a Picker session id")
	}
	_, err := argInt(a, "limit", gphotosDefaultLimit, 1, gphotosMaxLimit)
	return err
}

// gphotosCanRead: the granted scopes cover the ops (unknown scopes pass;
// Google then answers 403 and the person connects again).
func gphotosCanRead(t *Token) bool {
	return t == nil || len(t.Scopes) == 0 || strings.Contains(" "+strings.Join(t.Scopes, " ")+" ", " "+gphotosScope+" ")
}

type gphotosSession struct {
	ID            string `json:"id"`
	PickerURI     string `json:"pickerUri"`
	MediaItemsSet bool   `json:"mediaItemsSet"`
	ExpireTime    string `json:"expireTime"`
}

// item: the session id, and the Picker's link while it is Google Photos'.
func (s gphotosSession) item(title string) *Row {
	r := &Row{Title: title, Value: s.ID}
	if u, err := url.Parse(s.PickerURI); err == nil && u.Scheme == "https" && u.Host == gphotosPickerHost {
		r.URL = s.PickerURI
	}
	if t, err := time.Parse(time.RFC3339, s.ExpireTime); err == nil {
		r.Subtitle = t.In(gcalJST).Format("01/02 15:04") + " まで"
	}
	return r
}

func (p *googlePhotos) startPicking(ctx context.Context, in Input) (Output, error) {
	if err := checkGphotosStart(in.Args); err != nil {
		return Output{}, fmt.Errorf("%w: %v", ErrBadArgs, err)
	}
	if !gphotosCanRead(in.Token) {
		return Output{}, ErrNotConnected
	}
	body := []byte(`{}`)
	if n, _ := argInt(in.Args, "maxItems", 0, 1, gphotosMaxItems); n > 0 {
		b, err := json.Marshal(map[string]any{"pickingConfig": map[string]string{"maxItemCount": strconv.Itoa(n)}})
		if err != nil {
			return Output{}, errors.New("googlePhotos: building the request failed")
		}
		body = b
	}
	var s gphotosSession
	if err := p.do(ctx, in.Token, http.MethodPost, "/sessions", body, &s); err != nil {
		return Output{}, err
	}
	if !gphotosIDRe.MatchString(s.ID) {
		return Output{}, &ProviderError{Status: http.StatusOK, Err: errors.New("googlePhotos: no session id")}
	}
	return Output{Item: s.item("Google フォトで写真を選ぶ")}, nil
}

type gphotosItem struct {
	ID         string `json:"id"`
	CreateTime string `json:"createTime"`
	Type       string `json:"type"`
	MediaFile  struct {
		MimeType string `json:"mimeType"`
		Filename string `json:"filename"`
		Metadata struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"mediaFileMetadata"`
	} `json:"mediaFile"`
}

func (it gphotosItem) row() Row {
	r := Row{Title: clip(it.MediaFile.Filename, 200)}
	if r.Title == "" {
		r.Title = "(名前なし)"
	}
	var when, kind, size string
	if t, err := time.Parse(time.RFC3339, it.CreateTime); err == nil {
		when = t.In(gcalJST).Format("2006-01-02 15:04")
	}
	switch it.Type {
	case "PHOTO":
		kind = "写真"
	case "VIDEO":
		kind = "動画"
	}
	if m := it.MediaFile.Metadata; m.Width > 0 && m.Height > 0 {
		size = strconv.Itoa(m.Width) + "×" + strconv.Itoa(m.Height)
	}
	r.Subtitle = joinDots(when, kind, size)
	if gphotosIDRe.MatchString(it.ID) {
		r.Value = it.ID
	}
	return r
}

func (p *googlePhotos) pickedItems(ctx context.Context, in Input) (Output, error) {
	if err := checkGphotosPicked(in.Args); err != nil {
		return Output{}, fmt.Errorf("%w: %v", ErrBadArgs, err)
	}
	if !gphotosCanRead(in.Token) {
		return Output{}, ErrNotConnected
	}
	id := strings.TrimSpace(in.Args["sessionId"])
	limit, _ := argInt(in.Args, "limit", gphotosDefaultLimit, 1, gphotosMaxLimit)
	var s gphotosSession
	if err := p.do(ctx, in.Token, http.MethodGet, "/sessions/"+url.PathEscape(id), nil, &s); err != nil {
		return Output{}, err
	}
	if s.ID == "" {
		s.ID = id
	}
	if !s.MediaItemsSet {
		return Output{Item: s.item("まだ選んでいません"), Rows: []Row{}}, nil
	}
	v := url.Values{"sessionId": {id}, "pageSize": {strconv.Itoa(limit)}}
	var r struct {
		MediaItems []gphotosItem `json:"mediaItems"`
	}
	if err := p.do(ctx, in.Token, http.MethodGet, "/mediaItems?"+v.Encode(), nil, &r); err != nil {
		return Output{}, err
	}
	rows := make([]Row, 0, min(limit, len(r.MediaItems)))
	for _, it := range r.MediaItems {
		if len(rows) == limit {
			break
		}
		rows = append(rows, it.row())
	}
	item := &Row{Title: strconv.Itoa(len(rows)) + " 件を選びました", Value: id}
	return Output{Item: item, Rows: rows}, nil
}

// do is Google's do with the Picker's answers: 400 / 404 (a session that
// is not this person's or has expired) are bad_request; any other 403 but
// a disabled API is the connection (connect again, 409).
func (p *googlePhotos) do(ctx context.Context, tok *Token, method, path string, body []byte, out any) error {
	ctype := ""
	if body != nil {
		ctype = "application/json"
	}
	err := p.g.do(ctx, tok, method, path, body, ctype, out)
	var pe *ProviderError
	if errors.As(err, &pe) && !pe.Retryable {
		switch pe.Status {
		case http.StatusForbidden:
			if errors.Is(err, errGoogleAPIDisabled) {
				return err
			}
			return ErrNotConnected
		case http.StatusBadRequest, http.StatusNotFound:
			return &ProviderError{Status: pe.Status, Err: fmt.Errorf("%w: googlePhotos has no such session", ErrUnknown)}
		}
	}
	return err
}
