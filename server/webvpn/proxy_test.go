package webvpn

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestScrubBackendCORSHeaders(t *testing.T) {
	header := make(http.Header)
	header.Set("Access-Control-Allow-Origin", "*")
	header.Set("Access-Control-Allow-Credentials", "true")
	header.Set("Access-Control-Allow-Headers", "Authorization")
	header.Set("Access-Control-Allow-Methods", "GET")
	header.Set("Access-Control-Expose-Headers", "X-Internal")
	header.Set("Access-Control-Max-Age", "3600")
	header.Set("X-Backend", "kept")

	scrubBackendCORSHeaders(header)

	for _, name := range []string{
		"Access-Control-Allow-Origin",
		"Access-Control-Allow-Credentials",
		"Access-Control-Allow-Headers",
		"Access-Control-Allow-Methods",
		"Access-Control-Expose-Headers",
		"Access-Control-Max-Age",
	} {
		assert.Empty(t, header.Values(name), name)
	}
	assert.Equal(t, "kept", header.Get("X-Backend"))
}

func TestStripRemLinkCookiesIsCaseInsensitive(t *testing.T) {
	cookies := []*http.Cookie{
		{Name: "WEBVPN_SESSION", Value: "spoofed"},
		{Name: "Portal_Session", Value: "spoofed"},
		{Name: "JSESSIONID", Value: "backend"},
	}

	assert.Equal(t, "JSESSIONID=backend", StripRemLinkCookies(cookies))
}

func TestScrubSetCookieDomain(t *testing.T) {
	resp := &http.Response{Header: make(http.Header)}
	resp.Header.Add("Set-Cookie", "webvpn_session=spoofed; Path=/")
	resp.Header.Add("Set-Cookie", "PORTAL_SESSION=spoofed; Path=/")
	resp.Header.Add("Set-Cookie", "backend=value; DOMAIN=.backend.example; Path=/; HttpOnly")
	resp.Header.Add("Set-Cookie", "other=value; Domain=public.example; Path=/")
	resp.Header.Add("Set-Cookie", "plain=value; Path=/")

	scrubSetCookieDomain(resp, "backend.example:443")

	assert.Equal(t, []string{
		"backend=value; Path=/; HttpOnly",
		"other=value; Domain=public.example; Path=/",
		"plain=value; Path=/",
	}, resp.Header.Values("Set-Cookie"))
}
