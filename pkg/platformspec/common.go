package platformspec

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/Masterminds/semver/v3"
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

// CheckPlatformSupport checks platform compatibility using an already validated PluginSpecification.
func (v *defaultValidator) CheckPlatformSupport(pluginSpec *PluginSpecification, platformVersion string) (bool, error) {
	if pluginSpec == nil {
		return false, errors.New("plugin specification cannot be nil for platform support check")
	}
	// Assume pluginSpec is already structurally validated by ProcessSpecification
	if !isNonEmpty(platformVersion) {
		return false, errors.New("platformVersion cannot be empty for platform support check")
	}

	// Parse the current platform version
	currentV, err := semver.NewVersion(platformVersion)
	if err != nil {
		return false, fmt.Errorf("invalid platform version format '%s': %w", platformVersion, err)
	}

	supportedVersions := pluginSpec.SupportedPlatformVersions // Access directly from spec
	// Structure validation already ensured this is not empty.

	// Check against each constraint defined in the manifest
	for _, constraintStr := range supportedVersions {
		// Structure validation already ensured constraints are non-empty and valid syntax.
		constraints, err := semver.NewConstraint(constraintStr)
		if err != nil {
			// This should ideally not happen if structure validation passed, but handle defensively.
			log.Printf("Internal Warning: Re-parsing constraint '%s' failed during support check: %v", constraintStr, err)
			return false, fmt.Errorf("internal error: failed to re-parse constraint '%s': %w", constraintStr, err)
		}
		// Check if the current platform version satisfies the constraint
		if constraints.Check(currentV) {
			log.Printf("Platform version '%s' matches constraint '%s' for plugin '%s'.", platformVersion, constraintStr, pluginSpec.Name) // Use spec.Name
			return true, nil                                                                                                              // Found a matching constraint
		}
	}

	// If no constraint matched
	log.Printf("Platform version '%s' does not satisfy any supported-platform-versions constraints %v for plugin '%s'.",
		platformVersion, supportedVersions, pluginSpec.Name) // Use spec.Name
	return false, nil
}

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
