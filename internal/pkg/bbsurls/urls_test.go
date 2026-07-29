package bbsurls

import "testing"

func TestIsInternalURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		href    string
		want    bool
	}{
		{name: "anchor", baseURL: "https://bbs.example.com", href: "#comments", want: true},
		{name: "root relative path", baseURL: "https://bbs.example.com", href: "/topic/1", want: true},
		{name: "path relative URL", baseURL: "https://bbs.example.com", href: "topic/1?tab=latest", want: true},
		{name: "query relative URL", baseURL: "https://bbs.example.com", href: "?page=2", want: true},
		{name: "same host", baseURL: "https://bbs.example.com", href: "https://bbs.example.com/topic/1", want: true},
		{name: "same host is case insensitive", baseURL: "https://BBS.example.com", href: "http://bbs.EXAMPLE.com/topic/1", want: true},
		{name: "same host and port", baseURL: "https://bbs.example.com:8443", href: "https://bbs.example.com:8443/topic/1", want: true},
		{name: "different port", baseURL: "https://bbs.example.com:8443", href: "https://bbs.example.com/topic/1", want: false},
		{name: "host suffix confusion", baseURL: "https://bbs.example.com", href: "https://bbs.example.com.evil.test/topic/1", want: false},
		{name: "host prefix confusion", baseURL: "https://bbs.example.com", href: "https://evil-bbs.example.com/topic/1", want: false},
		{name: "host in query", baseURL: "https://bbs.example.com", href: "https://evil.test/?next=bbs.example.com", want: false},
		{name: "host in user info", baseURL: "https://bbs.example.com", href: "https://bbs.example.com@evil.test/topic/1", want: false},
		{name: "protocol relative same host", baseURL: "https://bbs.example.com", href: "//bbs.example.com/topic/1", want: true},
		{name: "protocol relative external host", baseURL: "https://bbs.example.com", href: "//evil.test/topic/1", want: false},
		{name: "extra slash relative same host", baseURL: "https://bbs.example.com", href: "////bbs.example.com/topic/1", want: true},
		{name: "extra slash relative external host", baseURL: "https://bbs.example.com", href: "///evil.test/topic/1", want: false},
		{name: "extra backslash relative same host", baseURL: "https://bbs.example.com", href: `\\\bbs.example.com/topic/1`, want: true},
		{name: "extra backslash relative external host", baseURL: "https://bbs.example.com", href: `\\\evil.test/topic/1`, want: false},
		{name: "empty network path", baseURL: "https://bbs.example.com", href: "////", want: false},
		{name: "backslash relative same host", baseURL: "https://bbs.example.com", href: `\\bbs.example.com/topic/1`, want: true},
		{name: "backslash relative external host", baseURL: "https://bbs.example.com", href: `\\evil.test/topic/1`, want: false},
		{name: "non HTTP scheme", baseURL: "https://bbs.example.com", href: "ftp://bbs.example.com/file", want: false},
		{name: "opaque HTTP URL", baseURL: "https://bbs.example.com", href: "https:evil.test", want: false},
		{name: "absolute URL without configured host", baseURL: "/", href: "https://bbs.example.com/topic/1", want: false},
		{name: "relative URL without configured host", baseURL: "/", href: "/topic/1", want: true},
		{name: "protocol relative URL without configured host", baseURL: "/", href: "//evil.test/topic/1", want: false},
		{name: "blank URL", baseURL: "https://bbs.example.com", href: "  ", want: false},
		{name: "malformed URL", baseURL: "https://bbs.example.com", href: "https://%zz", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := isInternalURL(tt.href, tt.baseURL)
			if err != nil {
				t.Fatalf("isInternalURL() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("isInternalURL(%q, %q) = %v, want %v", tt.href, tt.baseURL, got, tt.want)
			}
		})
	}
}

func TestIsInternalURLRejectsMalformedBaseURL(t *testing.T) {
	got, err := isInternalURL("https://bbs.example.com/topic/1", "https://%zz")
	if err == nil {
		t.Fatal("isInternalURL() error = nil, want malformed base URL error")
	}
	if got {
		t.Fatal("isInternalURL() = true for malformed base URL")
	}
}
