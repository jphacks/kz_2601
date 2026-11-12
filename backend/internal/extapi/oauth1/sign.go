// Package oauth1 computes OAuth 1.0a HMAC-SHA1 signatures (RFC 5849 3.4).
//
// Nothing calls it yet and it is not in the extapi registry: it only shows the
// signing is small and correct while #1953 decides whether the runner gets
// OAuth 1.0a (Trello). No nonce, timestamp, header building or token flow here.
package oauth1

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"net/url"
	"slices"
	"strings"
)

// Param is one name/value pair; names may repeat.
type Param struct{ Key, Value string }

// Encode is the RFC 5849 3.6 percent-encoding: everything but ALPHA, DIGIT,
// "-", ".", "_", "~" becomes %XX (upper-case hex) of its UTF-8 bytes.
func Encode(s string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if 'A' <= c && c <= 'Z' || 'a' <= c && c <= 'z' || '0' <= c && c <= '9' || c == '-' || c == '.' || c == '_' || c == '~' {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&15])
	}
	return b.String()
}

// BaseURI is the base string URI (3.4.1.2): lower-case scheme and host, the
// port only when not the scheme's default, the path, no query or fragment.
func BaseURI(u *url.URL) string {
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	if p := u.Port(); p != "" && !(scheme == "http" && p == "80") && !(scheme == "https" && p == "443") {
		host += ":" + p
	}
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	return scheme + "://" + host + path
}

// Collect gathers the request parameters (3.4.1.3.1): the query, a
// form-encoded body (empty when the body is not form-encoded) and the
// protocol parameters. oauth_signature and realm are left out.
func Collect(u *url.URL, formBody string, protocol []Param) ([]Param, error) {
	var out []Param
	for _, raw := range []string{u.RawQuery, formBody} {
		q, err := url.ParseQuery(raw)
		if err != nil {
			return nil, err
		}
		for k, vs := range q {
			for _, v := range vs {
				out = append(out, Param{k, v})
			}
		}
	}
	for _, p := range protocol {
		if p.Key != "oauth_signature" && p.Key != "realm" {
			out = append(out, p)
		}
	}
	return out, nil
}

// NormalizeParams encodes each name and value, sorts by name then value, and
// joins them as name=value with "&" (3.4.1.3.2).
func NormalizeParams(params []Param) string {
	enc := make([]Param, len(params))
	for i, p := range params {
		enc[i] = Param{Encode(p.Key), Encode(p.Value)}
	}
	slices.SortFunc(enc, func(a, b Param) int {
		if c := strings.Compare(a.Key, b.Key); c != 0 {
			return c
		}
		return strings.Compare(a.Value, b.Value)
	})
	pairs := make([]string, len(enc))
	for i, p := range enc {
		pairs[i] = p.Key + "=" + p.Value
	}
	return strings.Join(pairs, "&")
}

// BaseString is the signature base string (3.4.1.1).
func BaseString(method string, u *url.URL, params []Param) string {
	return Encode(strings.ToUpper(method)) + "&" + Encode(BaseURI(u)) + "&" + Encode(NormalizeParams(params))
}

// SignHMACSHA1 is the HMAC-SHA1 signature (3.4.2), base64; the caller
// percent-encodes it when it goes into the Authorization header.
func SignHMACSHA1(base, consumerSecret, tokenSecret string) string {
	mac := hmac.New(sha1.New, []byte(Encode(consumerSecret)+"&"+Encode(tokenSecret)))
	mac.Write([]byte(base))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
