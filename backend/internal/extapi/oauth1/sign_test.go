package oauth1

import (
	"net/url"
	"testing"
)

func mustURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestEncode(t *testing.T) {
	tests := map[string]string{
		"":         "",
		"abcXYZ09": "abcXYZ09",
		"-._~":     "-._~",
		"r b":      "r%20b",
		"=%3D":     "%3D%253D",
		"c@":       "c%40",
		"a+b/c":    "a%2Bb%2Fc",
		"*":        "%2A",
		"é":        "%C3%A9",
	}
	for in, want := range tests {
		if got := Encode(in); got != want {
			t.Errorf("Encode(%q) = %q, want %q", in, got, want)
		}
	}
}

// RFC 5849 3.4.1.2.
func TestBaseURI(t *testing.T) {
	tests := map[string]string{
		"HTTP://EXAMPLE.COM:80/r%20v/X?id=123": "http://example.com/r%20v/X",
		"https://www.example.net:8080/?q=1":    "https://www.example.net:8080/",
		"https://example.com:443":              "https://example.com/",
		"http://example.com:443/a":             "http://example.com:443/a",
	}
	for in, want := range tests {
		if got := BaseURI(mustURL(t, in)); got != want {
			t.Errorf("BaseURI(%q) = %q, want %q", in, got, want)
		}
	}
}

// RFC 5849 3.4.1: the request
//
//	POST /request?b5=%3D%253D&a3=a&c%40=&a2=r%20b HTTP/1.1
//	Host: example.com
//	Content-Type: application/x-www-form-urlencoded
//	Authorization: OAuth realm="Example", oauth_consumer_key="9djdj82h48djs9d2", ...
//
//	c2&a3=2+q
func TestBaseStringRFC5849Example(t *testing.T) {
	u := mustURL(t, "http://example.com/request?b5=%3D%253D&a3=a&c%40=&a2=r%20b")
	params, err := Collect(u, "c2&a3=2+q", []Param{
		{"realm", "Example"},
		{"oauth_consumer_key", "9djdj82h48djs9d2"},
		{"oauth_token", "kkk9d7dh3k39sjv7"},
		{"oauth_signature_method", "HMAC-SHA1"},
		{"oauth_timestamp", "137131201"},
		{"oauth_nonce", "7d8f3e4a"},
		{"oauth_signature", "djosJKDKJSD8743243/jdk33klY="},
	})
	if err != nil {
		t.Fatal(err)
	}

	// 3.4.1.3.2
	wantNorm := "a2=r%20b&a3=2%20q&a3=a&b5=%3D%253D&c%40=&c2=&oauth_consumer_key=9djdj82h48djs9d2" +
		"&oauth_nonce=7d8f3e4a&oauth_signature_method=HMAC-SHA1&oauth_timestamp=137131201&oauth_token=kkk9d7dh3k39sjv7"
	if got := NormalizeParams(params); got != wantNorm {
		t.Errorf("normalized\ngot:  %s\nwant: %s", got, wantNorm)
	}

	// 3.4.1.1
	wantBase := "POST&http%3A%2F%2Fexample.com%2Frequest&a2%3Dr%2520b%26a3%3D2%2520q" +
		"%26a3%3Da%26b5%3D%253D%25253D%26c%2540%3D%26c2%3D%26oauth_consumer_" +
		"key%3D9djdj82h48djs9d2%26oauth_nonce%3D7d8f3e4a%26oauth_signature_m" +
		"ethod%3DHMAC-SHA1%26oauth_timestamp%3D137131201%26oauth_token%3Dkkk" +
		"9d7dh3k39sjv7"
	if got := BaseString("post", u, params); got != wantBase {
		t.Errorf("base string\ngot:  %s\nwant: %s", got, wantBase)
	}
}

// RFC 5849 1.2, the protected resource request (the 3.4.1 example has no
// secrets, so its signature cannot be checked).
func TestSignHMACSHA1RFC5849Examples(t *testing.T) {
	const consumerSecret = "kd94hf93k423kf44"
	tests := []struct {
		name, method, url, tokenSecret, want string
		protocol                             []Param
	}{
		{
			name: "photos", method: "GET", url: "http://photos.example.net/photos?file=vacation.jpg&size=original", tokenSecret: "pfkkdhi9sl3r4s00",
			protocol: []Param{
				{"oauth_consumer_key", "dpf43f3p2l4k3l03"},
				{"oauth_token", "nnch734d00sl2jdk"},
				{"oauth_signature_method", "HMAC-SHA1"},
				{"oauth_timestamp", "137131202"},
				{"oauth_nonce", "chapoH"},
			},
			want: "MdpQcU8iPSUjWoN/UDMsK2sui9I=",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := mustURL(t, tt.url)
			params, err := Collect(u, "", tt.protocol)
			if err != nil {
				t.Fatal(err)
			}
			if got := SignHMACSHA1(BaseString(tt.method, u, params), consumerSecret, tt.tokenSecret); got != tt.want {
				t.Fatalf("signature %s, want %s\nbase: %s", got, tt.want, BaseString(tt.method, u, params))
			}
		})
	}
}

// OAuth Core 1.0 Appendix A.5 (with oauth_version), the vector most libraries test.
func TestSignHMACSHA1CoreAppendix(t *testing.T) {
	u := mustURL(t, "http://photos.example.net/photos?file=vacation.jpg&size=original")
	params, err := Collect(u, "", []Param{
		{"oauth_consumer_key", "dpf43f3p2l4k3l03"},
		{"oauth_token", "nnch734d00sl2jdk"},
		{"oauth_signature_method", "HMAC-SHA1"},
		{"oauth_timestamp", "1191242096"},
		{"oauth_nonce", "kllo9940pd9333jh"},
		{"oauth_version", "1.0"},
	})
	if err != nil {
		t.Fatal(err)
	}
	base := BaseString("GET", u, params)
	wantBase := "GET&http%3A%2F%2Fphotos.example.net%2Fphotos&file%3Dvacation.jpg%26oauth_consumer_key%3Ddpf43f3p2l4k3l03" +
		"%26oauth_nonce%3Dkllo9940pd9333jh%26oauth_signature_method%3DHMAC-SHA1%26oauth_timestamp%3D1191242096" +
		"%26oauth_token%3Dnnch734d00sl2jdk%26oauth_version%3D1.0%26size%3Doriginal"
	if base != wantBase {
		t.Fatalf("base string\ngot:  %s\nwant: %s", base, wantBase)
	}
	if got, want := SignHMACSHA1(base, "kd94hf93k423kf44", "pfkkdhi9sl3r4s00"), "tR3+Ty81lMeYAr/Fid0kMTYa/WM="; got != want {
		t.Fatalf("signature %s, want %s", got, want)
	}
}

func TestSignEncodesSecrets(t *testing.T) {
	// the key is encode(consumer)&encode(token): a "&" in a secret must not split it
	if SignHMACSHA1("x", "a&b", "") == SignHMACSHA1("x", "a", "b") {
		t.Fatal("secrets are not percent-encoded into the key")
	}
}

func TestCollectBadQuery(t *testing.T) {
	if _, err := Collect(&url.URL{RawQuery: "a=%zz"}, "", nil); err == nil {
		t.Fatal("bad escape accepted")
	}
}
