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
