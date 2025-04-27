package platformspec

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"oras.land/oras-go/v2/registry"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/errcode"
)

// --- Configuration Constants (Duplicated here for clarity, consider centralizing) ---
const (
	MaxRegistryRetries     = 3
	MaxDownloadRetries     = 3
	InitialBackoffDuration = 1 * time.Second
	OverallRequestTimeout  = 60 * time.Second
	MaxDownloadSizeBytes   = 1 * 1024 * 1024 * 1024 // 1 GiB
)

// validateImageManifestExists checks if an image manifest exists in the remote registry using ORAS libraries.
// It performs retries with exponential backoff for transient network or server errors.
func (v *defaultValidator) validateImageManifestExists(imageURI string) error {
	if !isNonEmpty(imageURI) {
		return errors.New("image URI cannot be empty for existence check")
	}
	if !imageDigestRegex.MatchString(imageURI) {
		return fmt.Errorf("image URI ('%s') must be in digest format (e.g., repo/image@sha256:...) for existence check", imageURI)
	}

	log.Printf("--- Checking Image Manifest Existence (using ORAS): %s ---", imageURI)
	var lastErr error
	backoff := InitialBackoffDuration

	for attempt := 0; attempt <= MaxRegistryRetries; attempt++ {
		if attempt > 0 {
			jitter := time.Duration(rand.Int63n(int64(backoff) / 2)) // Add jitter
			waitTime := backoff + jitter
			log.Printf("Image resolve attempt %d for '%s' failed. Retrying in %v...", attempt, imageURI, waitTime)
			time.Sleep(waitTime)
			backoff *= 2 // Exponential backoff
		}

		log.Printf("Image resolve attempt %d/%d for %s...", attempt+1, MaxRegistryRetries+1, imageURI)
		ctx, cancel := context.WithTimeout(context.Background(), OverallRequestTimeout) // Apply overall timeout

		var err error // Declare err here for the scope

		// 1. Parse the image reference
		var ref registry.Reference
		ref, err = registry.ParseReference(imageURI)
		if err != nil {
			cancel() // Release context resources
			return fmt.Errorf("failed to parse image reference '%s': %w", imageURI, err)
		}

		// 2. Create a remote repository client
		var repo registry.Repository
		repo, err = remote.NewRepository(ref.Repository) // Pass just the repository part (e.g., "library/alpine")
		if err != nil {
			lastErr = fmt.Errorf("attempt %d: failed to create ORAS repository client for '%s': %w", attempt+1, ref.Repository, err)
			cancel()
			continue // Retry might not help, but let's follow the loop structure
		}

		// 3. Resolve the manifest by digest
		log.Printf("Attempting to resolve digest '%s' in repository '%s'...", ref.Reference, ref.Repository)
		_, err = repo.Resolve(ctx, ref.Reference) // ref.Reference contains the digest
		cancel()                                  // Release context resources after the operation

		// 4. Handle results
		if err == nil {
			log.Printf("Successfully resolved image manifest for '%s'.", imageURI)
			return nil // Success! Manifest exists.
		}

		// --- Error Handling ---
		lastErr = fmt.Errorf("attempt %d: failed to resolve image manifest for '%s': %w", attempt+1, imageURI, err)
		log.Printf("ORAS resolve error details: %v", err)

		var errResp *errcode.ErrorResponse
		if errors.As(err, &errResp) {
			log.Printf("Registry returned HTTP status %d: %s", errResp.StatusCode, errResp.Error())
			if errResp.StatusCode >= 400 && errResp.StatusCode < 500 {
				log.Printf("Attempt %d: Received client error %d. Aborting retries.", attempt+1, errResp.StatusCode)
				return lastErr // Return the specific error, don't retry
			}
		} else if errors.Is(err, context.DeadlineExceeded) {
			log.Printf("Attempt %d: Operation timed out.", attempt+1)
		} else if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			log.Printf("Attempt %d: Network timeout detected.", attempt+1)
		} else {
			log.Printf("Attempt %d: Encountered non-HTTP or unknown error type. Retrying allowed.", attempt+1)
		}
	} // End retry loop

	return fmt.Errorf("failed to resolve image manifest '%s' after %d attempts: %w", imageURI, MaxRegistryRetries+1, lastErr)
}

// validateSingleDownloadableComponent downloads, verifies checksum, and checks path (if applicable) for one component.
// Returns the downloaded data on success. Retries are handled by downloadWithRetry.
func (v *defaultValidator) validateSingleDownloadableComponent(component Component, componentName string) ([]byte, error) {
	log.Printf("--- Validating Downloadable Component: %s ---", componentName)
	if !isNonEmpty(component.URI) {
		return nil, fmt.Errorf("%s validation failed: component URI is missing", componentName)
	}
	log.Printf("Component URI: %s", component.URI)
	log.Printf("Checksum provided: %s", component.Checksum)            // Log if checksum is expected
	log.Printf("PathInArchive specified: %s", component.PathInArchive) // Log if path check is needed

	// 1. Download the artifact with retries
	downloadedData, err := v.downloadWithRetry(component.URI)
	if err != nil {
		return nil, fmt.Errorf("%s download failed from URI '%s': %w", componentName, component.URI, err)
	}
	if len(downloadedData) == 0 {
		return nil, fmt.Errorf("%s validation failed: downloaded file from '%s' is unexpectedly empty", componentName, component.URI)
	}
	log.Printf("Successfully downloaded %d bytes for %s from %s.", len(downloadedData), componentName, component.URI)

	// 2. Verify Checksum (if provided)
	err = v.verifyChecksum(downloadedData, component.Checksum)
	if err != nil {
		return nil, fmt.Errorf("%s checksum verification failed for URI '%s': %w", componentName, component.URI, err)
	}

	// 3. Validate Path in Archive (if specified)
	if isNonEmpty(component.PathInArchive) {
		log.Printf("Checking for path '%s' within downloaded archive for %s...", component.PathInArchive, componentName)
		err := v.validateArchivePathExists(downloadedData, component.PathInArchive, component.URI)
		if err != nil {
			return nil, fmt.Errorf("%s archive path check failed for URI '%s': %w", componentName, component.URI, err)
		}
		log.Printf("Successfully verified path '%s' exists within archive for %s.", component.PathInArchive, componentName)
	} else {
		log.Printf("Component %s validated (no path-in-archive specified).", componentName)
	}

	log.Printf("--- Downloadable Component Validation Successful: %s ---", componentName)
	return downloadedData, nil
}

// downloadWithRetry attempts to download a file from a URL with exponential backoff, jitter, size limits, and status checks.
func (v *defaultValidator) downloadWithRetry(url string) ([]byte, error) {
	var lastErr error
	backoff := InitialBackoffDuration

	for attempt := 0; attempt <= MaxDownloadRetries; attempt++ {
		if attempt > 0 {
			jitter := time.Duration(rand.Int63n(int64(backoff) / 2))
			waitTime := backoff + jitter
			log.Printf("Download attempt %d for '%s' failed. Retrying in %v...", attempt, url, waitTime)
			time.Sleep(waitTime)
			backoff *= 2 // Exponential backoff
		}

		log.Printf("Download attempt %d/%d for %s...", attempt+1, MaxDownloadRetries+1, url)
		ctx, cancel := context.WithTimeout(context.Background(), OverallRequestTimeout) // Timeout for the whole attempt

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			lastErr = fmt.Errorf("attempt %d: failed to create HTTP request for '%s': %w", attempt+1, url, err)
			cancel()
			continue
		}
		// Consider adding User-Agent?
		// req.Header.Set("User-Agent", "platformspec-validator/1.0")

		resp, err := httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("attempt %d: HTTP request failed for '%s': %w", attempt+1, url, err)
			if errors.Is(err, context.DeadlineExceeded) {
				log.Printf("Attempt %d: Request timed out for '%s'.", attempt+1, url)
			} else if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				log.Printf("Attempt %d: Network timeout detected for '%s'.", attempt+1, url)
			}
			cancel()
			continue
		}

		// Check HTTP Status Code
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			bodyPreview := make([]byte, 512)
			n, _ := io.ReadFull(resp.Body, bodyPreview)
			resp.Body.Close()
			cancel()

			errMsg := fmt.Sprintf("attempt %d: received non-success HTTP status %d (%s) for '%s'. Body preview: %s",
				attempt+1, resp.StatusCode, http.StatusText(resp.StatusCode), url, string(bodyPreview[:n]))
			lastErr = errors.New(errMsg)

			if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusRequestTimeout && resp.StatusCode != http.StatusTooManyRequests {
				log.Printf("Attempt %d: Received client error %d. Aborting retries for '%s'.", attempt+1, resp.StatusCode, url)
				return nil, lastErr
			}
			log.Printf("Attempt %d: Received status %d. Allowing retry for '%s'.", attempt+1, resp.StatusCode, url)
			continue
		}

		// Read Response Body with Size Limit
		var expectedSize int64 = -1
		contentLengthHeader := resp.Header.Get("Content-Length")
		if contentLengthHeader != "" {
			if parsedSize, parseErr := strconv.ParseInt(contentLengthHeader, 10, 64); parseErr == nil && parsedSize >= 0 {
				expectedSize = parsedSize
				if expectedSize > MaxDownloadSizeBytes {
					resp.Body.Close()
					cancel()
					return nil, fmt.Errorf("attempt %d: declared content length %d bytes exceeds maximum allowed %d bytes for '%s'", attempt+1, expectedSize, MaxDownloadSizeBytes, url)
				}
				log.Printf("Attempt %d: Content-Length header indicates %d bytes for '%s'.", attempt+1, expectedSize, url)
			} else {
				log.Printf("Attempt %d: Warning - Could not parse Content-Length header '%s' for '%s'.", attempt+1, contentLengthHeader, url)
			}
		} else {
			log.Printf("Attempt %d: Warning - Content-Length header missing for '%s'. Proceeding with download limit.", attempt+1, url)
		}

		limitedReader := io.LimitedReader{R: resp.Body, N: MaxDownloadSizeBytes + 1}
		bodyBytes, err := io.ReadAll(&limitedReader)
		readErr := err
		closeErr := resp.Body.Close()
		cancel()

		if readErr != nil {
			lastErr = fmt.Errorf("attempt %d: failed to read response body from '%s': %w", attempt+1, url, readErr)
			continue
		}
		if closeErr != nil {
			log.Printf("Warning: Error closing response body for '%s' on attempt %d: %v", url, attempt+1, closeErr)
		}
		if limitedReader.N == 0 {
			return nil, fmt.Errorf("attempt %d: downloaded file from '%s' exceeds maximum allowed size of %d bytes", attempt+1, url, MaxDownloadSizeBytes)
		}

		// Verify Size Against Content-Length (if available)
		actualSize := int64(len(bodyBytes))
		if expectedSize != -1 && actualSize != expectedSize {
			lastErr = fmt.Errorf("attempt %d: downloaded size %d bytes does not match Content-Length header %d bytes for '%s'", attempt+1, actualSize, expectedSize, url)
			continue
		}

		log.Printf("Download successful for '%s' (%d bytes) on attempt %d.", url, actualSize, attempt+1)
		return bodyBytes, nil // Success

	} // End retry loop

	return nil, fmt.Errorf("download failed for '%s' after %d attempts: %w", url, MaxDownloadRetries+1, lastErr)
}

// verifyChecksum compares the SHA256 hash of data against an expected checksum string (e.g., "sha256:abc...").
func (v *defaultValidator) verifyChecksum(data []byte, expectedChecksum string) error {
	if !isNonEmpty(expectedChecksum) {
		log.Println("Checksum verification skipped: No checksum provided in the specification.")
		return nil
	}

	parts := strings.SplitN(expectedChecksum, ":", 2)
	if len(parts) != 2 || !isNonEmpty(parts[0]) || !isNonEmpty(parts[1]) {
		return fmt.Errorf("invalid checksum format '%s', expected format 'algorithm:hash' (e.g., 'sha256:...')", expectedChecksum)
	}

	algo, expectedHash := strings.ToLower(parts[0]), strings.ToLower(parts[1])

	if algo != "sha256" {
		return fmt.Errorf("unsupported checksum algorithm '%s', only 'sha256' is supported", algo)
	}

	if len(expectedHash) != 64 || !isHex(expectedHash) {
		return fmt.Errorf("invalid expected sha256 hash format '%s', must be 64 hexadecimal characters", expectedHash)
	}

	hasher := sha256.New()
	if _, err := io.Copy(hasher, bytes.NewReader(data)); err != nil {
		return fmt.Errorf("failed to calculate sha256 hash: %w", err)
	}
	actualHash := hex.EncodeToString(hasher.Sum(nil))

	if actualHash != expectedHash {
		return fmt.Errorf("checksum mismatch: expected sha256:%s, but calculated sha256:%s", expectedHash, actualHash)
	}

	log.Printf("Checksum verified successfully (sha256: %s)", actualHash)
	return nil
}

// isHex checks if a string contains only hexadecimal characters.
func isHex(s string) bool {
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

// validateArchivePathExists checks if a specific file path exists within various archive formats (zip, tar.gz, tar.bz2).
// It reads the archive from the provided byte slice.
func (v *defaultValidator) validateArchivePathExists(archiveData []byte, pathInArchive string, archiveURI string) error {
	if len(archiveData) == 0 {
		return errors.New("cannot check path in empty archive data")
	}
	if !isNonEmpty(pathInArchive) {
		return errors.New("path-in-archive cannot be empty when checking archive")
	}
	cleanedPath := filepath.Clean(strings.Trim(pathInArchive, "/"))
	if !isNonEmpty(cleanedPath) || cleanedPath == "." {
		return fmt.Errorf("invalid path-in-archive specified: '%s'", pathInArchive)
	}

	log.Printf("Attempting to detect archive type for URI: %s", archiveURI)
	archiveType := ""
	lowerURI := strings.ToLower(archiveURI)
	if strings.HasSuffix(lowerURI, ".tar.gz") || strings.HasSuffix(lowerURI, ".tgz") {
		archiveType = "tar.gz"
	} else if strings.HasSuffix(lowerURI, ".tar.bz2") || strings.HasSuffix(lowerURI, ".tbz2") {
		archiveType = "tar.bz2"
	} else if strings.HasSuffix(lowerURI, ".zip") {
		archiveType = "zip"
	} else {
		return fmt.Errorf("unsupported or unrecognized archive extension for URI '%s'. Supported: .zip, .tar.gz, .tgz, .tar.bz2, .tbz2", archiveURI)
	}
	log.Printf("Detected archive type: %s. Searching for path: '%s'", archiveType, cleanedPath)

	var err error
	found := false
	byteReader := bytes.NewReader(archiveData) // Use a reader for archive libraries

	switch archiveType {
	case "zip":
		var zipReader *zip.Reader
		zipReader, err = zip.NewReader(byteReader, int64(len(archiveData)))
		if err != nil {
			return fmt.Errorf("failed to create zip reader for '%s': %w", archiveURI, err)
		}
		for _, file := range zipReader.File {
			fileNameCleaned := filepath.Clean(strings.Trim(file.Name, "/"))
			if fileNameCleaned == cleanedPath {
				if file.FileInfo().IsDir() {
					return fmt.Errorf("path '%s' in zip archive '%s' is a directory, not a file", cleanedPath, archiveURI)
				}
				rc, openErr := file.Open()
				if openErr != nil {
					return fmt.Errorf("found path '%s' in zip '%s', but failed to open it: %w", cleanedPath, archiveURI, openErr)
				}
				oneByte := make([]byte, 1)
				_, readErr := rc.Read(oneByte)
				rc.Close()
				if readErr != nil && readErr != io.EOF {
					return fmt.Errorf("found path '%s' in zip '%s', but failed to read from it (corrupt?): %w", cleanedPath, archiveURI, readErr)
				}
				log.Printf("Successfully found and opened file path '%s' in zip archive.", cleanedPath)
				found = true
				break
			}
		}

	case "tar.gz":
		var gzipReader *gzip.Reader
		gzipReader, err = gzip.NewReader(byteReader)
		if err != nil {
			return fmt.Errorf("failed to create gzip reader for '%s': %w", archiveURI, err)
		}
		defer gzipReader.Close()
		tarReader := tar.NewReader(gzipReader)
		found, err = v.checkTarArchive(tarReader, cleanedPath, archiveURI, "tar.gz")
		if err != nil {
			return err
		}

	case "tar.bz2":
		bz2Reader := bzip2.NewReader(byteReader)
		tarReader := tar.NewReader(bz2Reader)
		found, err = v.checkTarArchive(tarReader, cleanedPath, archiveURI, "tar.bz2")
		if err != nil {
			return err
		}

	default:
		return fmt.Errorf("internal error: unexpected archive type '%s'", archiveType)
	}

	if !found {
		return fmt.Errorf("path '%s' was not found as a file within the %s archive '%s'", cleanedPath, archiveType, archiveURI)
	}

	return nil
}

// checkTarArchive iterates through a tar reader to find and validate a specific file path.
func (v *defaultValidator) checkTarArchive(tarReader *tar.Reader, cleanedPath string, archiveURI string, archiveType string) (bool, error) {
	filesChecked := 0
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			return false, fmt.Errorf("failed to read next tar header in %s archive '%s' (checked %d files): %w", archiveType, archiveURI, filesChecked, err)
		}
		filesChecked++

		headerNameCleaned := filepath.Clean(strings.Trim(header.Name, "/"))

		if headerNameCleaned == cleanedPath {
			if header.Typeflag == tar.TypeReg || header.Typeflag == tar.TypeRegA || header.Typeflag == 0 {
				log.Printf("Found matching file path '%s' in %s archive. Type: %v, Size: %d.", cleanedPath, archiveType, header.Typeflag, header.Size)
				if header.Size > 0 {
					written, copyErr := io.Copy(io.Discard, tarReader)
					if copyErr != nil {
						return false, fmt.Errorf("found path '%s' in %s archive '%s', but failed to read its content (corrupt?): %w", cleanedPath, archiveType, archiveURI, copyErr)
					}
					if written != header.Size {
						return false, fmt.Errorf("found path '%s' in %s archive '%s', but read %d bytes instead of expected header size %d (corrupt?)", cleanedPath, archiveType, archiveURI, written, header.Size)
					}
					log.Printf("Successfully read %d bytes for file path '%s' in %s archive.", written, cleanedPath, archiveType)
				} else {
					log.Printf("File path '%s' in %s archive has size 0.", cleanedPath, archiveType)
				}
				return true, nil // Found the file
			} else {
				return false, fmt.Errorf("path '%s' in %s archive '%s' exists but is not a regular file (typeflag: %v)", cleanedPath, archiveType, archiveURI, header.Typeflag)
			}
		}
	}
	log.Printf("Checked %d files in %s archive '%s', path '%s' not found.", filesChecked, archiveType, archiveURI, cleanedPath)
	return false, nil // Not found
}
