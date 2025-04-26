// Package pluginmanifest provides utilities for loading, validating, and verifying plugin manifests
// and their associated downloadable components, including OCI image existence checks.
package pluginmanifest

import (
	// Standard library imports
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors" // Import errors package for error handling
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	// Third-party imports
	"github.com/Masterminds/semver/v3"
	_ "github.com/opencontainers/image-spec/specs-go/v1" // OCI spec alias - underscore import as types aren't directly used in this version but good to note dependency
	"gopkg.in/yaml.v3"
	"oras.land/oras-go/v2/registry"        // For parsing reference
	"oras.land/oras-go/v2/registry/remote" // For interacting with remote registries
	// For auth types if needed later
)

// --- Struct Definitions ---

// Component represents a single functional part of the plugin.
type Component struct {
	// URI for downloadable components (e.g., zip, tar.gz).
	URI string `yaml:"uri,omitempty" json:"uri,omitempty"`
	// ImageURI for container image components (e.g., discovery).
	// Must be in digest format (e.g., repo/image@sha256:hash).
	ImageURI string `yaml:"image-uri,omitempty" json:"image-uri,omitempty"`
	// PathInArchive specifies the relative path to the executable or file
	// within a downloaded archive (if URI points to an archive).
	PathInArchive string `yaml:"path-in-archive,omitempty" json:"path-in-archive,omitempty"`
	// Checksum for verifying file integrity (e.g., "sha256:<hex_hash>").
	// For downloadable archives, it's the hash of the archive file.
	// Generally not used for ImageURI as the digest serves this purpose.
	Checksum string `yaml:"checksum,omitempty" json:"checksum,omitempty"`
}

// Metadata holds descriptive information about the plugin.
type Metadata struct {
	// Author of the plugin (Required).
	Author string `yaml:"author" json:"author"`
	// PublishedDate of this plugin version (YYYY-MM-DD format recommended) (Required).
	PublishedDate string `yaml:"published-date" json:"published-date"`
	// Contact information for the author or support (Required).
	Contact string `yaml:"contact" json:"contact"`
	// License identifier (SPDX format recommended, e.g., "Apache-2.0") (Required).
	License string `yaml:"license" json:"license"`
	// Description of the plugin's functionality (Optional).
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	// Website URL for the plugin or author (Optional).
	Website string `yaml:"website,omitempty" json:"website,omitempty"`
}

// Plugin defines the core details of the plugin.
type Plugin struct {
	// Name of the plugin (unique identifier) (Required).
	Name string `yaml:"name" json:"name"`
	// Version of the plugin (SemVer format recommended) (Required).
	Version string `yaml:"version" json:"version"`
	// SupportedPlatformVersions lists the platform versions this plugin is compatible with,
	// using SemVer constraint strings (e.g., ">=1.2.0, <2.0.0") (Required, non-empty).
	SupportedPlatformVersions []string `yaml:"supported-platform-versions" json:"supported-platform-versions"`
	// Metadata about the plugin (Required).
	Metadata Metadata `yaml:"metadata" json:"metadata"`
	// Components defines the functional parts of the plugin (Required).
	Components PluginComponents `yaml:"components" json:"components"`
	// SampleData provides information about optional sample data (Optional).
	SampleData *Component `yaml:"sample-data,omitempty" json:"sample-data,omitempty"`
}

// PluginComponents holds the different component definitions.
type PluginComponents struct {
	// Discovery component, typically a container image (Required).
	Discovery Component `yaml:"discovery" json:"discovery"`
	// PlatformBinary component, typically a downloadable executable or archive (Required).
	PlatformBinary Component `yaml:"platform-binary" json:"platform-binary"`
	// CloudQLBinary component, typically a downloadable executable or archive (Required).
	CloudQLBinary Component `yaml:"cloudql-binary" json:"cloudql-binary"`
}

// PluginManifest is the top-level structure for the manifest file.
type PluginManifest struct {
	// APIVersion of the manifest schema (Required, must be "v1").
	APIVersion string `yaml:"api-version" json:"api-version"`
	// Type of the manifest (Required, must be "plugin").
	Type string `yaml:"type" json:"type"`
	// Plugin definition (Required).
	Plugin Plugin `yaml:"plugin" json:"plugin"`
}

// --- Configuration Constants ---
const (
	// MaxRegistryRetries defines the maximum number of times to retry resolving an image manifest.
	MaxRegistryRetries = 3
	// MaxDownloadRetries defines the maximum number of times to retry downloading an artifact archive.
	MaxDownloadRetries = 3
	// InitialBackoffDuration defines the starting wait time for retries.
	InitialBackoffDuration = 1 * time.Second
	// ConnectTimeout is the maximum time to wait for establishing a TCP connection.
	ConnectTimeout = 5 * time.Second
	// TLSHandshakeTimeout is the maximum time allowed for the TLS handshake.
	TLSHandshakeTimeout = 5 * time.Second
	// ResponseHeaderTimeout is the maximum time to wait for the server's response headers.
	ResponseHeaderTimeout = 10 * time.Second
	// OverallRequestTimeout defines the maximum time allowed for a single HTTP request attempt.
	OverallRequestTimeout = 60 * time.Second
	// MaxDownloadSizeBytes limits the maximum size of a downloadable artifact archive file.
	MaxDownloadSizeBytes = 1 * 1024 * 1024 * 1024 // 1 GiB limit

	// ArtifactTypeDiscovery identifies the discovery image component.
	ArtifactTypeDiscovery = "discovery"
	// ArtifactTypePlatformBinary identifies the platform-binary component.
	ArtifactTypePlatformBinary = "platform-binary" // Added constant
	// ArtifactTypeCloudQLBinary identifies the cloudql-binary component.
	ArtifactTypeCloudQLBinary = "cloudql-binary" // Added constant
	// ArtifactTypeAll indicates validation for all relevant components.
	ArtifactTypeAll = "all"
)

// --- Global HTTP Client ---
var httpClient *http.Client

// --- Regular Expression for Image Digest ---
var imageDigestRegex = regexp.MustCompile(`^.+@sha256:[a-fA-F0-9]{64}$`)

// init initializes the package-level resources.
func init() {
	rand.Seed(time.Now().UnixNano())
	httpClient = &http.Client{
		Timeout: OverallRequestTimeout,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: ConnectTimeout, KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2: true, MaxIdleConns: 100, IdleConnTimeout: 90 * time.Second,
			TLSHandshakeTimeout: TLSHandshakeTimeout, ResponseHeaderTimeout: ResponseHeaderTimeout,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
	log.Println("Initialized shared HTTP client for plugin manifest validation.")
}

// --- Interface Definition ---

// PluginValidator defines the interface for loading and validating plugin manifests and artifacts.
type PluginValidator interface {
	// LoadManifest reads and parses a plugin manifest from the given file path.
	LoadManifest(filePath string) (*PluginManifest, error)
	// ValidateManifestStructure performs structural and metadata checks on a loaded manifest.
	ValidateManifestStructure(manifest *PluginManifest) error
	// CheckPlatformSupport checks if the manifest supports a given platform version.
	CheckPlatformSupport(manifest *PluginManifest, platformVersion string) (bool, error)
	// ValidateArtifact downloads/verifies specific artifacts based on artifactType.
	// Valid types: "discovery", "platform-binary", "cloudql-binary", "all" (or empty).
	ValidateArtifact(manifest *PluginManifest, artifactType string) error
}

// --- Concrete Implementation ---

// defaultValidator implements the PluginValidator interface.
type defaultValidator struct{}

// NewDefaultValidator creates a new instance of the default validator.
func NewDefaultValidator() PluginValidator {
	return &defaultValidator{}
}

// --- Helper Function ---
func isNonEmpty(s string) bool {
	return strings.TrimSpace(s) != ""
}

// --- Interface Method Implementations ---

// LoadManifest reads and parses the manifest file from the given path.
func (v *defaultValidator) LoadManifest(filePath string) (*PluginManifest, error) {
	log.Printf("Loading manifest from: %s", filePath)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file '%s': %w", filePath, err)
	}
	var manifest PluginManifest
	err = yaml.Unmarshal(data, &manifest)
	if err != nil {
		return nil, fmt.Errorf("failed to parse manifest file '%s' (check syntax): %w", filePath, err)
	}
	return &manifest, nil
}

// ValidateManifestStructure performs structural, metadata, and format checks on the manifest.
func (v *defaultValidator) ValidateManifestStructure(manifest *PluginManifest) error {
	if manifest == nil {
		return fmt.Errorf("manifest cannot be nil")
	}
	if !isNonEmpty(manifest.APIVersion) || manifest.APIVersion != "v1" {
		return fmt.Errorf("api-version must be 'v1'")
	}
	if !isNonEmpty(manifest.Type) || manifest.Type != "plugin" {
		return fmt.Errorf("type must be 'plugin'")
	}
	if !isNonEmpty(manifest.Plugin.Name) {
		return fmt.Errorf("plugin.name is required")
	}
	if !isNonEmpty(manifest.Plugin.Version) {
		return fmt.Errorf("plugin.version is required")
	}
	if _, err := semver.NewVersion(manifest.Plugin.Version); err != nil {
		return fmt.Errorf("invalid plugin.version format '%s': %w", manifest.Plugin.Version, err)
	}
	if len(manifest.Plugin.SupportedPlatformVersions) == 0 {
		return fmt.Errorf("plugin.supported-platform-versions requires at least one entry")
	}
	for i, constraintStr := range manifest.Plugin.SupportedPlatformVersions {
		if !isNonEmpty(constraintStr) {
			return fmt.Errorf("plugin.supported-platform-versions entry %d cannot be empty", i)
		}
		if _, err := semver.NewConstraint(constraintStr); err != nil {
			return fmt.Errorf("invalid constraint string '%s': %w", constraintStr, err)
		}
	}
	if !isNonEmpty(manifest.Plugin.Metadata.Author) {
		return fmt.Errorf("plugin.metadata.author is required")
	}
	if !isNonEmpty(manifest.Plugin.Metadata.PublishedDate) {
		return fmt.Errorf("plugin.metadata.published-date is required")
	}
	if !isNonEmpty(manifest.Plugin.Metadata.Contact) {
		return fmt.Errorf("plugin.metadata.contact is required")
	}
	if !isNonEmpty(manifest.Plugin.Metadata.License) {
		return fmt.Errorf("plugin.metadata.license is required")
	}
	discoveryURI := manifest.Plugin.Components.Discovery.ImageURI
	if !isNonEmpty(discoveryURI) {
		return fmt.Errorf("plugin.components.discovery.image-uri is required")
	}
	if !imageDigestRegex.MatchString(discoveryURI) {
		return fmt.Errorf("plugin.components.discovery.image-uri ('%s') must be in digest format (e.g., repository/image@sha256:hash)", discoveryURI)
	}
	platformComp := manifest.Plugin.Components.PlatformBinary
	cloudqlComp := manifest.Plugin.Components.CloudQLBinary
	if !isNonEmpty(platformComp.URI) {
		return fmt.Errorf("plugin.components.platform-binary.uri is required")
	}
	if !isNonEmpty(cloudqlComp.URI) {
		return fmt.Errorf("plugin.components.cloudql-binary.uri is required")
	}
	if platformComp.URI == cloudqlComp.URI {
		if !isNonEmpty(platformComp.PathInArchive) {
			return fmt.Errorf("plugin.components.platform-binary.path-in-archive required when URIs match ('%s')", platformComp.URI)
		}
		if !isNonEmpty(cloudqlComp.PathInArchive) {
			return fmt.Errorf("plugin.components.cloudql-binary.path-in-archive required when URIs match ('%s')", cloudqlComp.URI)
		}
	}
	if manifest.Plugin.SampleData != nil && !isNonEmpty(manifest.Plugin.SampleData.URI) {
		return fmt.Errorf("plugin.sample-data.uri required when sample-data section present")
	}
	return nil
}

// CheckPlatformSupport checks if the manifest supports a given platform version.
func (v *defaultValidator) CheckPlatformSupport(manifest *PluginManifest, platformVersion string) (bool, error) {
	if manifest == nil {
		return false, fmt.Errorf("manifest cannot be nil")
	}
	if !isNonEmpty(platformVersion) {
		return false, fmt.Errorf("platformVersion cannot be empty")
	}
	currentV, err := semver.NewVersion(platformVersion)
	if err != nil {
		return false, fmt.Errorf("invalid platform version format '%s': %w", platformVersion, err)
	}
	if len(manifest.Plugin.SupportedPlatformVersions) == 0 {
		log.Printf("Warning: Checking support for platform %s against plugin %s with no defined supported versions.", platformVersion, manifest.Plugin.Name)
		return false, nil
	}
	for _, constraintStr := range manifest.Plugin.SupportedPlatformVersions {
		constraints, err := semver.NewConstraint(constraintStr)
		if err != nil {
			log.Printf("Warning: Skipping invalid constraint '%s' during support check.", constraintStr)
			continue
		}
		if constraints.Check(currentV) {
			return true, nil
		}
	}
	return false, nil
}

// ValidateArtifact downloads/verifies specific artifacts based on artifactType.
// Valid types: "discovery", "platform-binary", "cloudql-binary", "all" (or empty).
func (v *defaultValidator) ValidateArtifact(manifest *PluginManifest, artifactType string) error {
	if manifest == nil {
		return fmt.Errorf("manifest cannot be nil for artifact validation")
	}
	normalizedType := strings.ToLower(artifactType)
	if !isNonEmpty(artifactType) {
		normalizedType = ArtifactTypeAll
	}
	logMsgType := normalizedType
	log.Printf("--- Starting Artifact Validation (Type: %s) ---", logMsgType)

	validateDiscovery := false
	validatePlatform := false
	validateCloudQL := false
	switch normalizedType {
	case ArtifactTypeAll:
		validateDiscovery = true
		validatePlatform = true
		validateCloudQL = true
		log.Println("Validating Discovery, PlatformBinary, and CloudQLBinary artifacts.")
	case ArtifactTypeDiscovery:
		validateDiscovery = true
		log.Println("Validating only Discovery artifact (image existence).")
	case ArtifactTypePlatformBinary:
		validatePlatform = true
		log.Println("Validating only PlatformBinary artifact.")
	case ArtifactTypeCloudQLBinary:
		validateCloudQL = true
		log.Println("Validating only CloudQLBinary artifact.")
	default:
		return fmt.Errorf("invalid artifactType '%s'. Must be '%s', '%s', '%s', or empty/all", artifactType, ArtifactTypeDiscovery, ArtifactTypePlatformBinary, ArtifactTypeCloudQLBinary)
	}

	var wg sync.WaitGroup
	var discoveryErr, platformErr, cloudqlErr error
	var platformData []byte
	platformComp := manifest.Plugin.Components.PlatformBinary
	cloudqlComp := manifest.Plugin.Components.CloudQLBinary

	if validateDiscovery {
		log.Println("Initiating Discovery image validation...")
		discoveryErr = v.validateImageManifestExists(manifest.Plugin.Components.Discovery.ImageURI) // Pass URI directly
		if discoveryErr != nil {
			log.Printf("Discovery image validation failed: %v", discoveryErr)
		} else {
			log.Println("Discovery image validation successful.")
		}
	}

	if validatePlatform {
		wg.Add(1)
		go func() {
			defer wg.Done()
			log.Println("Initiating PlatformBinary artifact validation...")
			platformData, platformErr = v.validateSingleDownloadableComponent(platformComp, ArtifactTypePlatformBinary)
			if platformErr == nil {
				log.Println("PlatformBinary artifact validation successful.")
			}
		}()
	}
	if validateCloudQL && platformComp.URI != cloudqlComp.URI {
		wg.Add(1)
		go func() {
			defer wg.Done()
			log.Println("Initiating CloudQLBinary artifact validation (separate URI)...")
			_, cloudqlErr = v.validateSingleDownloadableComponent(cloudqlComp, ArtifactTypeCloudQLBinary)
			if cloudqlErr == nil {
				log.Println("CloudQLBinary artifact validation successful.")
			}
		}()
	}
	wg.Wait() // Wait for downloads

	if validateCloudQL && platformComp.URI == cloudqlComp.URI {
		log.Println("Initiating CloudQLBinary artifact validation (shared URI)...")
		if platformErr != nil {
			cloudqlErr = fmt.Errorf("cannot validate cloudql-binary path in shared archive because platform-binary validation failed: %w", platformErr)
		} else if platformData == nil {
			cloudqlErr = fmt.Errorf("internal logic error: platform data not available for shared URI validation")
		} else {
			log.Printf("Validating cloudql path '%s' within shared archive from %s...", cloudqlComp.PathInArchive, platformComp.URI)
			err := v.validateArchivePathExists(platformData, cloudqlComp.PathInArchive, cloudqlComp.URI)
			if err != nil {
				cloudqlErr = fmt.Errorf("cloudql-binary artifact validation failed: archive/path check failed for shared URI %s: %w", cloudqlComp.URI, err)
			} else {
				log.Println("CloudQLBinary artifact validation successful (shared URI path check).")
			}
		}
	}

	var combinedErrors []string
	if discoveryErr != nil {
		combinedErrors = append(combinedErrors, fmt.Sprintf("discovery image validation failed: %v", discoveryErr))
	}
	if platformErr != nil {
		combinedErrors = append(combinedErrors, fmt.Sprintf("platform-binary artifact validation failed: %w", platformErr))
	}
	// Avoid duplicating error if it was already reported via platformErr in shared URI case
	if cloudqlErr != nil && !(platformComp.URI == cloudqlComp.URI && platformErr != nil) {
		combinedErrors = append(combinedErrors, fmt.Sprintf("cloudql-binary artifact validation failed: %w", cloudqlErr))
	}
	if len(combinedErrors) > 0 {
		return errors.New(strings.Join(combinedErrors, "; "))
	}

	log.Println("--- All requested artifact validations successful ---")
	return nil
}

// --- Internal Validation Helpers ---

// validateImageManifestExists checks if an image manifest exists in the registry using retries.
func (v *defaultValidator) validateImageManifestExists(imageURI string) error {
	if !isNonEmpty(imageURI) {
		return fmt.Errorf("image URI is empty")
	}
	if !imageDigestRegex.MatchString(imageURI) {
		return fmt.Errorf("image URI ('%s') must be in digest format", imageURI)
	}

	log.Printf("--- Checking Image Manifest Existence for: %s ---", imageURI)
	var lastErr error
	backoff := InitialBackoffDuration

	for attempt := 0; attempt <= MaxRegistryRetries; attempt++ {
		if attempt > 0 {
			jitter := time.Duration(rand.Int63n(int64(backoff) / 2))
			waitTime := backoff + jitter
			log.Printf("Image resolve attempt %d for %s failed. Retrying in %v...", attempt+1, imageURI, waitTime)
			time.Sleep(waitTime)
			backoff *= 2
		}
		log.Printf("Image resolve attempt %d/%d for %s...", attempt+1, MaxRegistryRetries+1, imageURI)
		ctx, cancel := context.WithTimeout(context.Background(), OverallRequestTimeout)
		defer cancel()

		ref, err := registry.ParseReference(imageURI)
		if err != nil {
			return fmt.Errorf("attempt %d: failed to parse image reference '%s': %w", attempt+1, imageURI, err)
		} // No retry
		// Use ref.Repository directly as it's the repository part of the reference
		repo, err := remote.NewRepository(ref.Repository)
		if err != nil {
			lastErr = fmt.Errorf("attempt %d: failed create repository client for '%s': %w", attempt+1, ref.Repository, err)
			continue
		} // Corrected: Use ref.Repository
		// Assign the global httpClient. Authentication can be added here if needed via auth.Client or repo options.
		repo.Client = httpClient // Corrected: Assign http client directly for default/anonymous access

		// Resolve attempts to fetch manifest metadata (HEAD or GET) using the digest
		_, err = repo.Resolve(ctx, ref.Reference) // ref.Reference is the digest

		if err == nil {
			log.Printf("Successfully resolved image manifest for %s.", imageURI)
			return nil
		} // Success

		lastErr = fmt.Errorf("attempt %d: failed resolve image manifest for '%s': %w", attempt+1, imageURI, err)
		log.Printf("Error details: %v", err)
		var httpErr *remote.Error // Correct type for checking registry errors
		if errors.As(err, &httpErr) {
			if httpErr.StatusCode >= 400 && httpErr.StatusCode < 500 {
				log.Printf("Attempt %d: Client error %d (%s), not retrying.", attempt+1, httpErr.StatusCode, http.StatusText(httpErr.StatusCode))
				return lastErr
			}
		} else if errors.Is(err, context.DeadlineExceeded) {
			log.Printf("Attempt %d: Request timed out.", attempt+1)
		}
	}
	return fmt.Errorf("failed to resolve image %s after %d attempts: %w", imageURI, MaxRegistryRetries+1, lastErr)
}

// validateSingleDownloadableComponent downloads and validates a specific downloadable binary component.
func (v *defaultValidator) validateSingleDownloadableComponent(component Component, componentName string) ([]byte, error) {
	log.Printf("--- Validating Downloadable Component: %s ---", componentName)
	if !isNonEmpty(component.URI) {
		return nil, fmt.Errorf("%s validation failed: URI is missing", componentName)
	}
	downloadedData, err := v.downloadWithRetry(component.URI)
	if err != nil {
		return nil, fmt.Errorf("%s download failed: %w", componentName, err)
	}
	if len(downloadedData) == 0 {
		return nil, fmt.Errorf("%s validation failed: downloaded file from %s is empty", componentName, component.URI)
	}
	err = v.verifyChecksum(downloadedData, component.Checksum)
	if err != nil {
		return nil, fmt.Errorf("%s validation failed: checksum error for URI %s: %w", componentName, component.URI, err)
	}
	if isNonEmpty(component.PathInArchive) {
		err := v.validateArchivePathExists(downloadedData, component.PathInArchive, component.URI)
		if err != nil {
			return nil, fmt.Errorf("%s validation failed: archive/path check failed for URI %s: %w", componentName, component.URI, err)
		}
	} else {
		log.Printf("Component %s downloaded and checksum verified (no pathInArchive specified, assuming direct download). Size: %d bytes.", componentName, len(downloadedData))
	}
	return downloadedData, nil
}

// downloadWithRetry attempts to download a file from a URL with exponential backoff and checks.
func (v *defaultValidator) downloadWithRetry(url string) ([]byte, error) {
	var lastErr error
	backoff := InitialBackoffDuration
	for attempt := 0; attempt <= MaxDownloadRetries; attempt++ {
		if attempt > 0 {
			jitter := time.Duration(rand.Int63n(int64(backoff) / 2))
			waitTime := backoff + jitter
			log.Printf("Download attempt %d for %s failed. Retrying in %v...", attempt+1, url, waitTime)
			time.Sleep(waitTime)
			backoff *= 2
		}
		log.Printf("Download attempt %d/%d for %s...", attempt+1, MaxDownloadRetries+1, url)
		ctx, cancel := context.WithTimeout(context.Background(), OverallRequestTimeout)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			lastErr = fmt.Errorf("attempt %d: failed create request: %w", attempt+1, err)
			continue
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("attempt %d: request failed: %w", attempt+1, err)
			if ctx.Err() == context.DeadlineExceeded {
				log.Printf("Attempt %d: Timeout", attempt+1)
			}
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			resp.Body.Close()
			lastErr = fmt.Errorf("attempt %d: status code %d. Body: %s", attempt+1, resp.StatusCode, string(bodyBytes))
			if resp.StatusCode >= 400 && resp.StatusCode < 500 {
				return nil, lastErr
			}
			continue
		}
		var expectedSize int64 = -1
		contentLengthHeader := resp.Header.Get("Content-Length")
		if contentLengthHeader != "" {
			if parsedSize, err := strconv.ParseInt(contentLengthHeader, 10, 64); err == nil && parsedSize >= 0 {
				expectedSize = parsedSize
				if expectedSize > MaxDownloadSizeBytes {
					resp.Body.Close()
					return nil, fmt.Errorf("attempt %d: content length %d > max %d", attempt+1, expectedSize, MaxDownloadSizeBytes)
				}
			} else {
				log.Printf("Attempt %d: Warning - invalid Content-Length '%s'", attempt+1, contentLengthHeader)
			}
		} else {
			log.Printf("Attempt %d: Warning - Content-Length missing", attempt+1)
		}
		limitedReader := io.LimitedReader{R: resp.Body, N: MaxDownloadSizeBytes + 1}
		bodyBytes, err := io.ReadAll(&limitedReader)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("attempt %d: read body failed: %w", attempt+1, err)
			continue
		}
		if limitedReader.N == 0 {
			return nil, fmt.Errorf("attempt %d: file > max %d bytes", attempt+1, MaxDownloadSizeBytes)
		}
		actualSize := int64(len(bodyBytes))
		if expectedSize != -1 && actualSize != expectedSize {
			lastErr = fmt.Errorf("attempt %d: size %d != Content-Length %d", attempt+1, actualSize, expectedSize)
			continue
		}
		log.Printf("Download successful for %s (%d bytes)", url, actualSize)
		return bodyBytes, nil
	}
	return nil, fmt.Errorf("download failed after %d attempts: %w", MaxDownloadRetries+1, lastErr)
}

// verifyChecksum compares the SHA256 hash of data against an expected checksum string.
func (v *defaultValidator) verifyChecksum(data []byte, expectedChecksum string) error {
	if !isNonEmpty(expectedChecksum) {
		log.Println("Warning: No checksum provided.")
		return nil
	}
	parts := strings.SplitN(expectedChecksum, ":", 2)
	if len(parts) != 2 || !isNonEmpty(parts[0]) || !isNonEmpty(parts[1]) {
		return fmt.Errorf("invalid checksum format '%s'", expectedChecksum)
	}
	algo, expectedHash := strings.ToLower(parts[0]), strings.ToLower(parts[1])
	if algo != "sha256" {
		return fmt.Errorf("unsupported checksum algorithm '%s'", algo)
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, bytes.NewReader(data)); err != nil {
		return fmt.Errorf("failed to calculate sha256: %w", err)
	}
	actualHash := hex.EncodeToString(hasher.Sum(nil))
	if actualHash != expectedHash {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedHash, actualHash)
	}
	log.Printf("Checksum verified (sha256: %s)", actualHash)
	return nil
}

// validateArchivePathExists checks various archive formats for a specific file path.
func (v *defaultValidator) validateArchivePathExists(archiveData []byte, pathInArchive string, archiveURI string) error {
	if len(archiveData) == 0 {
		return fmt.Errorf("archive data empty")
	}
	if !isNonEmpty(pathInArchive) {
		return fmt.Errorf("pathInArchive empty")
	}
	ext := strings.ToLower(filepath.Ext(archiveURI))
	archiveType := ""
	if strings.HasSuffix(archiveURI, ".tar.gz") || strings.HasSuffix(archiveURI, ".tgz") {
		archiveType = "tar.gz"
	} else if strings.HasSuffix(archiveURI, ".tar.bz2") || strings.HasSuffix(archiveURI, ".tbz2") {
		archiveType = "tar.bz2"
	} else if ext == ".zip" {
		archiveType = "zip"
	} else {
		return fmt.Errorf("unsupported archive extension for URI '%s'. Supported: .zip, .tar.gz, .tgz, .tar.bz2, .tbz2", archiveURI)
	}
	var err error
	found := false
	byteReader := bytes.NewReader(archiveData)
	switch archiveType {
	case "zip":
		zipReader, zipErr := zip.NewReader(byteReader, int64(len(archiveData)))
		if zipErr != nil {
			return fmt.Errorf("read zip failed: %w", zipErr)
		}
		for _, file := range zipReader.File {
			if file.Name == pathInArchive {
				if !file.FileInfo().IsDir() {
					rc, openErr := file.Open()
					if openErr != nil {
						return fmt.Errorf("zip path '%s' open failed: %w", pathInArchive, openErr)
					}
					_, copyErr := io.Copy(io.Discard, rc)
					rc.Close()
					if copyErr != nil {
						return fmt.Errorf("zip path '%s' read failed: %w", pathInArchive, copyErr)
					}
					found = true
				} else {
					return fmt.Errorf("zip path '%s' is directory", pathInArchive)
				}
				break
			}
		}
	case "tar.gz":
		gzipReader, gzErr := gzip.NewReader(byteReader)
		if gzErr != nil {
			return fmt.Errorf("gzip reader failed: %w", gzErr)
		}
		defer gzipReader.Close()
		tarReader := tar.NewReader(gzipReader)
		found, err = v.checkTarArchive(tarReader, pathInArchive)
		if err != nil {
			return err
		}
	case "tar.bz2":
		bz2Reader := bzip2.NewReader(byteReader)
		tarReader := tar.NewReader(bz2Reader)
		found, err = v.checkTarArchive(tarReader, pathInArchive)
		if err != nil {
			return err
		}
	}
	if !found {
		return fmt.Errorf("path '%s' not found in %s archive '%s'", pathInArchive, archiveType, archiveURI)
	}
	return nil
}

// checkTarArchive iterates through a tar reader to find and validate a path.
func (v *defaultValidator) checkTarArchive(tarReader *tar.Reader, pathInArchive string) (bool, error) {
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return false, fmt.Errorf("read tar header failed: %w", err)
		}
		if header.Name == pathInArchive {
			if header.Typeflag == tar.TypeReg || header.Typeflag == tar.TypeRegA || header.Typeflag == 0 {
				if _, copyErr := io.Copy(io.Discard, tarReader); copyErr != nil {
					return false, fmt.Errorf("tar path '%s' read failed (corrupt?): %w", pathInArchive, copyErr)
				}
				return true, nil
			} else {
				return false, fmt.Errorf("tar path '%s' not regular file (typeflag %v)", pathInArchive, header.Typeflag)
			}
		}
	}
	return false, nil
}
