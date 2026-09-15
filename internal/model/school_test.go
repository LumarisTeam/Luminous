package model

import "testing"

func TestIsValidEduSystemURL(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want bool
	}{
		{"empty is allowed (optional field)", "", true},
		{"public https", "https://jwc.xauat.edu.cn", true},
		{"public http", "http://jwc.xauat.edu.cn/jwglxt", true},
		{"private address allowed", "http://10.0.0.5/jwglxt", true},
		{"loopback allowed", "http://127.0.0.1:8080", true},
		{"ftp scheme", "ftp://jwc.xauat.edu.cn", false},
		{"javascript scheme", "javascript:alert(1)", false},
		{"no scheme", "jwc.xauat.edu.cn", false},
		{"missing host", "https://", false},
		{"credentials in url", "https://user:pass@jwc.xauat.edu.cn", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsValidEduSystemURL(tc.raw); got != tc.want {
				t.Errorf("IsValidEduSystemURL(%q) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}

// Guards the refactor that extracted isValidHTTPURL: IsValidURL must keep
// rejecting private addresses, since a school's website is fetched by the
// MCP proxy.
func TestIsValidURLStillRejectsPrivate(t *testing.T) {
	if IsValidURL("http://10.0.0.5") {
		t.Error("IsValidURL should reject private addresses")
	}
	if IsValidURL("http://127.0.0.1:8080") {
		t.Error("IsValidURL should reject loopback addresses")
	}
	if !IsValidURL("https://xauatapi.xauat.site/v1") {
		t.Error("IsValidURL should accept a public https address")
	}
}
