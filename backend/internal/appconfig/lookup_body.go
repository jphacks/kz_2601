package appconfig

// POST variants of the lookups that carry user-entered values (the ISBN, the
// diet search term, the public-apps search): the value goes in a small JSON
// body so it stays out of the URL. The GET variants stay for older iOS
// builds. Neither path logs the values.

import (
	"net/http"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

// lookupBodyMaxBytes caps the POST lookup bodies (one short object).
const lookupBodyMaxBytes = 4 << 10

// decodeLookupBody reads one JSON object (unknown field 400, over the cap
// 413). A query string is refused so a value is not sent in the URL anyway.
func decodeLookupBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if r.URL.RawQuery != "" {
		fail(w, apiErr(http.StatusBadRequest, httpx.CodeBadRequest, "send the parameters in the JSON body, not the query"))
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, lookupBodyMaxBytes)
	return httpx.DecodeBody(w, r, v)
}
