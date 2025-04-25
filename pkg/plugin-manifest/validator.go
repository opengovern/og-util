package utils

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
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	// Third-party imports
	"github.com/Masterminds/semver/v3"
	"gopkg.in/yaml.v3"
)

// --- Configuration Constants ---
// (Consider moving these to a config struct or loading from env/file)
const (
	MaxDownloadRetries     = 3
	InitialBackoffDuration = 1 * time.Second
	ConnectTimeout         = 5 * time.Second
	TLSHandshakeTimeout    = 5 * time.Second
	ResponseHeaderTimeout  = 10 * time.Second
	OverallDownloadTimeout = 60 * time.Second
	MaxDownloadSizeBytes   = 1 * 1024 * 1024 * 1024 // 1 GiB limit
)

// --- Global HTTP Client ---
var httpClient *http.Client

// Initialize the shared HTTP client once.
func init() {
	// Seed random number generator once at startup for jitter
	rand.Seed(time.Now().UnixNano())

	httpClient = &http.Client{
		Timeout: OverallDownloadTimeout,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   ConnectTimeout,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   TLSHandshakeTimeout,
			ResponseHeaderTimeout: ResponseHeaderTimeout,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
	log.Println("Initialized shared HTTP client.")
}

// --- Interface Definition ---

// PluginValidator defines the interface for loading and validating plugin manifests.
type PluginValidator interface {
	// LoadAndValidateManifest reads, parses, and validates a plugin manifest from the given file path.
	// It performs structural checks, metadata validation, and downloads/verifies binary components.
	LoadAndValidateManifest(filePath string) (*PluginManifest, error)
}

// --- Concrete Implementation ---

// defaultValidator implements the PluginValidator interface.
type defaultValidator struct {
	// Potentially add configuration fields here if not using constants/env vars
}

// NewDefaultValidator creates a new instance of the default validator.
func NewDefaultValidator() PluginValidator {
	return &defaultValidator{}
}

// --- Helper Function ---
func isNonEmpty(s string) bool {
	return strings.TrimSpace(s) != ""
}

// --- Core Logic Methods (associated with defaultValidator) ---

// LoadAndParseManifest implements the PluginValidator interface method.
func (v *defaultValidator) LoadAndParseManifest(filePath string) (*PluginManifest, error) {
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

	// Perform validation after successful parsing
	err = v.validateManifestStructure(&manifest)
	if err != nil {
		return nil, err // Return validation error
	}

	// Perform download/validation checks
	err = v.validateBinaryComponents(&manifest)
	if err != nil {
		return nil, err // Return download/validation error
	}

	log.Println("--- Manifest Validation Fully Successful ---")
	return &manifest, nil
}

// validateManifestStructure performs basic structural and metadata checks.
func (v *defaultValidator) validateManifestStructure(manifest *PluginManifest) error {
	log.Println("--- Starting Manifest Structure Validation ---")
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
		// Use stricter constraint format checking compatible with Masterminds/semver
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
	if !isNonEmpty(manifest.Plugin.Components.Discovery.ImageURI) {
		return fmt.Errorf("plugin.components.discovery.image-uri is required")
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

	log.Println("Basic manifest structure and metadata validation successful.")
	return nil
}

// validateBinaryComponents handles the download and validation logic.
func (v *defaultValidator) validateBinaryComponents(manifest *PluginManifest) error {
	log.Println("--- Starting Binary Component Validation ---")
	var wg sync.WaitGroup
	var platformErr, cloudqlErr error
	var platformData []byte // To store downloaded data if URIs match

	platformComp := manifest.Plugin.Components.PlatformBinary
	cloudqlComp := manifest.Plugin.Components.CloudQLBinary

	// Validate Platform Binary (always run)
	wg.Add(1)
	go func() {
		defer wg.Done()
		platformData, platformErr = v.validateSingleBinaryComponent(platformComp, "platform-binary")
	}()

	// Validate CloudQL Binary (conditionally run concurrently)
	if platformComp.URI != cloudqlComp.URI {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, cloudqlErr = v.validateSingleBinaryComponent(cloudqlComp, "cloudql-binary")
		}()
	}

	// Wait for downloads/validations to complete
	wg.Wait()

	// Check for errors from concurrent operations
	if platformErr != nil {
		return fmt.Errorf("platform-binary validation failed: %w", platformErr)
	}
	if cloudqlErr != nil {
		return fmt.Errorf("cloudql-binary validation failed: %w", cloudqlErr)
	}

	// If URIs matched, perform the CloudQL path check using the already downloaded data
	if platformComp.URI == cloudqlComp.URI {
		log.Printf("Validating cloudql path '%s' within shared archive from %s...", cloudqlComp.PathInArchive, platformComp.URI)
		err := v.validateArchivePathExists(platformData, cloudqlComp.PathInArchive, cloudqlComp.URI)
		if err != nil {
			return fmt.Errorf("cloudql-binary validation failed: archive/path check failed for URI %s: %w", cloudqlComp.URI, err)
		}
	}
	return nil
}

// validateSingleBinaryComponent downloads and validates a specific binary component.
func (v *defaultValidator) validateSingleBinaryComponent(component Component, componentName string) ([]byte, error) {
	log.Printf("--- Validating Component: %s ---", componentName)
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
		log.Printf("Validating archive from %s for path '%s'...", component.URI, component.PathInArchive)
		err := v.validateArchivePathExists(downloadedData, component.PathInArchive, component.URI)
		if err != nil {
			return nil, fmt.Errorf("%s validation failed: archive/path check failed for URI %s: %w", componentName, component.URI, err)
		}
	} else {
		log.Printf("Component %s downloaded and checksum verified (no pathInArchive specified). Size: %d bytes.", componentName, len(downloadedData))
	}

	log.Printf("--- Component %s Validation Successful ---", componentName)
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
			log.Printf("Download attempt %d for %s failed. Retrying in %v...", attempt, url, waitTime)
			time.Sleep(waitTime)
			backoff *= 2
		}

		log.Printf("Download attempt %d/%d for %s...", attempt+1, MaxDownloadRetries+1, url)
		ctx, cancel := context.WithTimeout(context.Background(), OverallDownloadTimeout)
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
			} // Don't retry client errors
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
				log.Printf("Attempt %d: Expecting %d bytes", attempt+1, expectedSize)
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

		log.Printf("Download successful (%d bytes)", actualSize)
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
	var err error
	found := false
	byteReader := bytes.NewReader(archiveData)

	switch {
	case ext == ".zip":
		archiveType = "zip"
		zipReader, err := zip.NewReader(byteReader, int64(len(archiveData)))
		if err != nil {
			return fmt.Errorf("read zip failed: %w", err)
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
	case strings.HasSuffix(archiveURI, ".tar.gz") || strings.HasSuffix(archiveURI, ".tgz"):
		archiveType = "tar.gz"
		gzipReader, err := gzip.NewReader(byteReader)
		if err != nil {
			return fmt.Errorf("gzip reader failed: %w", err)
		}
		defer gzipReader.Close()
		tarReader := tar.NewReader(gzipReader)
		found, err = v.checkTarArchive(tarReader, pathInArchive)
		if err != nil {
			return err
		}
	case strings.HasSuffix(archiveURI, ".tar.bz2") || strings.HasSuffix(archiveURI, ".tbz2"):
		archiveType = "tar.bz2"
		bz2Reader := bzip2.NewReader(byteReader)
		tarReader := tar.NewReader(bz2Reader)
		found, err = v.checkTarArchive(tarReader, pathInArchive)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported archive extension '%s'", ext)
	}

	if !found {
		return fmt.Errorf("path '%s' not found in %s archive '%s'", pathInArchive, archiveType, archiveURI)
	}
	log.Printf("Validated path '%s' exists in %s archive.", pathInArchive, archiveType)
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
					return false, fmt.Errorf("tar path '%s' read failed: %w", pathInArchive, copyErr)
				}
				return true, nil
			} else {
				return false, fmt.Errorf("tar path '%s' not regular file (type %v)", pathInArchive, header.Typeflag)
			}
		}
	}
	return false, nil // Not found
}
