package core

import (
	"net/http"
	"time"
)

const (
	// DefaultAPIVersion is the Alguna-Version this SDK was generated against. Clients send it
	// unless option.WithAPIVersion sets another, on the client or on a single request.
	DefaultAPIVersion = "2026-04-01"

	// DefaultTimeout limits each attempt of a call when the caller passes no
	// option.WithHTTPClient. A ctx deadline limits the whole call, retries included.
	DefaultTimeout = 60 * time.Second

	// SDKVersion is sent in the User-Agent header. The release sets it to the published
	// module version.
	SDKVersion = "0.1.0"
)

// ApplyClientDefaults fills the client options the caller left unset. The client
// constructor calls it once, so per-request options still override the defaults.
func ApplyClientDefaults(options *RequestOptions) {
	if options.APIVersion == "" {
		options.APIVersion = DefaultAPIVersion
	}
	if options.HTTPClient == nil {
		options.HTTPClient = &http.Client{Timeout: DefaultTimeout}
	}
	// The header map is the one in the caller's option.WithHTTPHeader, which every client
	// built with that option shares, so add User-Agent to a copy.
	header := options.HTTPHeader.Clone()
	if header == nil {
		header = make(http.Header)
	}
	if header.Get("User-Agent") == "" {
		header.Set("User-Agent", "alguna-go/"+SDKVersion)
	}
	options.HTTPHeader = header
}
