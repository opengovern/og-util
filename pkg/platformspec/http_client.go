package platformspec

import (
	"net"
	"net/http"
	"time"
)

// HTTP Client Configuration Constants
const (
	ConnectTimeout        = 10 * time.Second // Timeout for establishing a network connection.
	TLSHandshakeTimeout   = 10 * time.Second // Timeout for the TLS handshake.
	ResponseHeaderTimeout = 15 * time.Second // Timeout for receiving response headers.
	ClientOverallTimeout  = 60 * time.Second // Overall timeout for a single HTTP request.
	KeepAliveDuration     = 30 * time.Second // Keep-alive duration for TCP connections.
	MaxIdleConns          = 100              // Max idle connections across all hosts.
	MaxIdleConnsPerHost   = 10               // Max idle connections per host.
	IdleConnTimeout       = 90 * time.Second // Timeout for idle connections before closing.
	ExpectContinueTimeout = 1 * time.Second  // Timeout waiting for a 100 Continue response.
)

// initializeHTTPClient creates and configures the shared HTTP client.
// It is called by the package's init function in validator.go.
func initializeHTTPClient() {
	httpClient = &http.Client{
		Timeout: ClientOverallTimeout, // Overall timeout for the entire request lifecycle.
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment, // Respect standard proxy environment variables.
			DialContext: (&net.Dialer{
				Timeout:   ConnectTimeout,
				KeepAlive: KeepAliveDuration,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          MaxIdleConns,
			MaxIdleConnsPerHost:   MaxIdleConnsPerHost,
			IdleConnTimeout:       IdleConnTimeout,
			TLSHandshakeTimeout:   TLSHandshakeTimeout,
			ResponseHeaderTimeout: ResponseHeaderTimeout,
			ExpectContinueTimeout: ExpectContinueTimeout,
		},
	}
}
