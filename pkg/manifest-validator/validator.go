// Package manifestvalidator provides utilities for loading, validating, and verifying plugin manifests
// (both 'plugin' and 'task' types) and their associated downloadable components or container images.
// It follows a "one call" pattern: use ProcessManifest once to get a validated manifest object,
// then use that object with other functions like CheckPlatformSupport or GetTaskDetailsFromPluginManifest.
package manifestvalidator

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
	"errors"
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
	// Corrected import path for SPDX license validation using github/go-spdx
	"github.com/github/go-spdx/v2/spdxexp"
	"gopkg.in/yaml.v3"
	"oras.land/oras-go/v2/registry"
	"oras.land/oras-go/v2/registry/remote" // Keep for error type checking
	"oras.land/oras-go/v2/registry/remote/errcode"
	// _ "github.com/opencontainers/image-spec/specs-go/v1" // OCI spec alias - uncomment if needed elsewhere
)

// --- Struct Definitions ---

// Component represents a downloadable artifact or an image reference.
type Component struct {
	URI           string `yaml:"uri,omitempty" json:"uri,omitempty"`                         // URI for downloadable artifacts (e.g., binaries, sample data)
	ImageURI      string `yaml:"image-uri,omitempty" json:"image-uri,omitempty"`             // Deprecated: TaskManifest.ImageURL is used for discovery task image. Prefer ImageURL in TaskManifest.
	PathInArchive string `yaml:"path-in-archive,omitempty" json:"path-in-archive,omitempty"` // Path to the specific file within a downloaded archive (if URI points to an archive)
	Checksum      string `yaml:"checksum,omitempty" json:"checksum,omitempty"`               // Checksum for verifying downloaded artifact integrity (e.g., "sha256:...")
}

// Metadata holds descriptive information about the plugin or standalone task.
type Metadata struct {
	Author        string `yaml:"author" json:"author"`                               // Required: Author of the plugin/task.
	PublishedDate string `yaml:"published-date" json:"published-date"`               // Required: Date the version was published in YYYY-MM-DD format.
	Contact       string `yaml:"contact" json:"contact"`                             // Required: Contact information (e.g., email, website).
	License       string `yaml:"license" json:"license"`                             // Required: Valid SPDX license identifier (e.g., "Apache-2.0", "MIT"). See https://spdx.org/licenses/
	Description   string `yaml:"description,omitempty" json:"description,omitempty"` // Optional: Brief description.
	Website       string `yaml:"website,omitempty" json:"website,omitempty"`         // Optional: URL to the website or repository.
}

// Plugin defines the core details of the plugin manifest type.
type Plugin struct {
	Name                      string           `yaml:"name" json:"name"`                                               // Required: Name of the plugin.
	Version                   string           `yaml:"version" json:"version"`                                         // Required: Semantic version of the plugin (e.g., "1.2.3").
	SupportedPlatformVersions []string         `yaml:"supported-platform-versions" json:"supported-platform-versions"` // Required: List of platform version constraints (e.g., ">=1.0.0, <2.0.0").
	Metadata                  Metadata         `yaml:"metadata" json:"metadata"`                                       // Required: Metadata about the plugin.
	Components                PluginComponents `yaml:"components" json:"components"`                                   // Required: Defines the core functional parts of the plugin.
	SampleData                *Component       `yaml:"sample-data,omitempty" json:"sample-data,omitempty"`             // Optional: Reference to downloadable sample data.
}

// PluginComponents holds the different component definitions for a 'plugin' manifest.
type PluginComponents struct {
	// Discovery component IS the embedded task definition used for discovering resources.
	// It inherits metadata implicitly from the parent Plugin.
	Discovery TaskManifest `yaml:"discovery" json:"discovery"`
	// PlatformBinary component is a downloadable artifact (e.g., the main plugin executable).
	PlatformBinary Component `yaml:"platform-binary" json:"platform-binary"`
	// CloudQLBinary component is a downloadable artifact (e.g., a specific query language binary).
	CloudQLBinary Component `yaml:"cloudql-binary" json:"cloudql-binary"`
}

// PluginManifest is the top-level structure for the 'plugin' type manifest file.
// This is the object returned by ProcessManifest for plugin types.
type PluginManifest struct {
	APIVersion string `yaml:"api-version" json:"api-version"` // Required: Must be "v1".
	Type       string `yaml:"type" json:"type"`               // Required: Must be "plugin".
	Plugin     Plugin `yaml:"plugin" json:"plugin"`           // Required: Contains the plugin details.
}

// ScaleConfig defines the scaling parameters for a task.
type ScaleConfig struct {
	LagThreshold string `yaml:"lag_threshold" json:"lag_threshold"` // Required: String representing a positive integer threshold for scaling.
	MinReplica   int    `yaml:"min_replica" json:"min_replica"`     // Required: Minimum number of task replicas (>= 0).
	MaxReplica   int    `yaml:"max_replica" json:"max_replica"`     // Required: Maximum number of task replicas (>= MinReplica).
}

// RunScheduleEntry defines a single scheduled run configuration for a task.
type RunScheduleEntry struct {
	ID        string            `yaml:"id" json:"id"`               // Required: Unique identifier for the schedule entry (e.g., "daily-report", "default").
	Params    map[string]string `yaml:"params" json:"params"`       // Required: Parameters specific to this scheduled run. Must cover required top-level params if ID is "default".
	Frequency string            `yaml:"frequency" json:"frequency"` // Required: How often the task should run (format depends on scheduler implementation, e.g., cron string, interval).
}

// TaskManifest defines the structure for a task, used standalone or embedded in PluginComponents.Discovery.
// This is the object returned by ProcessManifest for task types.
type TaskManifest struct {
	APIVersion                string             `yaml:"api-version,omitempty" json:"api-version,omitempty"`                                 // Required ONLY for standalone tasks (must be "v1"). Must be ABSENT for embedded discovery task.
	Metadata                  *Metadata          `yaml:"metadata,omitempty" json:"metadata,omitempty"`                                       // Required ONLY for standalone tasks. Must be ABSENT for embedded discovery task.
	ID                        string             `yaml:"id" json:"id"`                                                                       // Required: Unique identifier for the task within its scope (e.g., plugin name for discovery).
	Name                      string             `yaml:"name" json:"name"`                                                                   // Required: Human-readable name for the task.
	Description               string             `yaml:"description" json:"description"`                                                     // Required: Description of what the task does (distinct from Metadata.Description).
	IsEnabled                 bool               `yaml:"is_enabled" json:"is_enabled"`                                                       // Required: Whether the task is enabled by default.
	Type                      string             `yaml:"type" json:"type"`                                                                   // Required: Must be "task".
	ImageURL                  string             `yaml:"image_url" json:"image_url"`                                                         // Required: Container image URL in digest format (e.g., "registry/repo/image@sha256:hash").
	Command                   string             `yaml:"command" json:"command"`                                                             // Required: Command to execute within the container.
	Timeout                   string             `yaml:"timeout" json:"timeout"`                                                             // Required: Maximum execution time (e.g., "5m", "1h"), must be < 24h.
	ScaleConfig               ScaleConfig        `yaml:"scale_config" json:"scale_config"`                                                   // Required: Scaling configuration for the task.
	Params                    []string           `yaml:"params" json:"params"`                                                               // Required: List of parameter names the task expects (can be empty []).
	Configs                   []interface{}      `yaml:"configs" json:"configs"`                                                             // Required: List of configuration items needed by the task (can be empty []). Structure depends on task needs.
	RunSchedule               []RunScheduleEntry `yaml:"run_schedule" json:"run_schedule"`                                                   // Required: List of scheduled runs (min 1 entry). Needs 'default' or 'describe-all' ID if top-level Params exist.
	SupportedPlatformVersions []string           `yaml:"supported-platform-versions,omitempty" json:"supported-platform-versions,omitempty"` // Required ONLY for standalone tasks. Must be ABSENT for embedded discovery task.
}

// TaskDetails holds extracted and validated details for a specific task, typically retrieved via GetTaskDetailsFromPluginManifest.
type TaskDetails struct {
	PluginName        string             // Name of the plugin this task belongs to.
	TaskID            string             // The unique ID of the task.
	TaskName          string             // The human-readable name of the task.
	ValidatedImageURI string             // The container image URI, validated for format and registry existence.
	Command           string             // The command executed by the task container.
	Timeout           string             // The execution timeout duration string.
	ScaleConfig       ScaleConfig        // The task's scaling configuration.
	Params            []string           // List of expected parameter names.
	Configs           []interface{}      // List of configuration items.
	RunSchedule       []RunScheduleEntry // List of scheduled runs.
	// Note: Metadata is not included here as it's assumed to be the same as the parent plugin's metadata.
	// If needed, it can be accessed from the original PluginManifest object.
}

// manifestBase is used for initial YAML parsing to determine the manifest type ("plugin" or "task").
type manifestBase struct {
	Type string `yaml:"type"`
}

// --- Configuration Constants ---
const (
	MaxRegistryRetries     = 3                      // Maximum number of retries for registry operations (e.g., image manifest resolution).
	MaxDownloadRetries     = 3                      // Maximum number of retries for downloading artifacts.
	InitialBackoffDuration = 1 * time.Second        // Starting duration for exponential backoff between retries.
	ConnectTimeout         = 10 * time.Second       // Timeout for establishing a network connection.
	TLSHandshakeTimeout    = 10 * time.Second       // Timeout for the TLS handshake.
	ResponseHeaderTimeout  = 15 * time.Second       // Timeout for receiving response headers.
	OverallRequestTimeout  = 60 * time.Second       // Overall timeout for a single HTTP request or registry operation.
	MaxDownloadSizeBytes   = 1 * 1024 * 1024 * 1024 // 1 GiB limit for downloadable artifacts.

	// Constants for artifact validation types used in ProcessManifest
	ArtifactTypeDiscovery      = "discovery"       // Validate only the discovery task's image.
	ArtifactTypePlatformBinary = "platform-binary" // Validate only the platform binary artifact.
	ArtifactTypeCloudQLBinary  = "cloudql-binary"  // Validate only the CloudQL binary artifact.
	ArtifactTypeAll            = "all"             // Validate all artifacts (default).

	// Standard manifest types
	ManifestTypePlugin = "plugin"
	ManifestTypeTask   = "task"

	// Standard API Version
	APIVersionV1 = "v1"

	// Date format for PublishedDate (YYYY-MM-DD)
	PublishedDateFormat = "2006-01-02" // Go's reference date format
)

// --- Global HTTP Client ---
// Shared HTTP client optimized for potentially frequent requests to registries and artifact servers.
var httpClient *http.Client

// --- Regular Expressions ---
// Regex to validate that an image URL uses the digest format (e.g., image@sha256:...).
var imageDigestRegex = regexp.MustCompile(`^.+@sha256:[a-fA-F0-9]{64}$`)

// init initializes the package-level resources like the shared HTTP client and random seed.
func init() {
	// Seed the random number generator for jitter calculation in retries.
	rand.Seed(time.Now().UnixNano())

	// Configure the shared HTTP client with appropriate timeouts and connection pooling.
	httpClient = &http.Client{
		Timeout: OverallRequestTimeout, // Overall timeout for the entire request lifecycle.
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment, // Respect standard proxy environment variables.
			DialContext: (&net.Dialer{
				Timeout:   ConnectTimeout,   // Timeout for establishing the TCP connection.
				KeepAlive: 30 * time.Second, // Keep-alive duration for TCP connections.
			}).DialContext,
			ForceAttemptHTTP2:     true,                  // Prefer HTTP/2.
			MaxIdleConns:          100,                   // Max idle connections across all hosts.
			MaxIdleConnsPerHost:   10,                    // Max idle connections per host.
			IdleConnTimeout:       90 * time.Second,      // Timeout for idle connections before closing.
			TLSHandshakeTimeout:   TLSHandshakeTimeout,   // Timeout for the TLS handshake phase.
			ResponseHeaderTimeout: ResponseHeaderTimeout, // Timeout waiting for response headers after sending request.
			ExpectContinueTimeout: 1 * time.Second,       // Timeout waiting for a 100 Continue response.
		},
	}
	log.Println("Initialized shared HTTP client for manifest validation.")
	// No SPDX init check needed here, the library handles it implicitly.
}

// --- Interface Definition ---

// Validator defines the interface for processing, validating, and retrieving information from manifests.
// It promotes a "one call" pattern: call ProcessManifest once, then use the returned validated
// manifest object (*PluginManifest or *TaskManifest) with other functions.
type Validator interface {
	// ProcessManifest is the primary entry point. It reads a manifest file, determines its type ("plugin" or "task"),
	// performs full structural validation (including date and SPDX license checks), checks platform compatibility (for plugins),
	// and optionally validates artifacts (downloadable components or container images) based on the flags.
	// On success, it returns the fully parsed and validated manifest struct (either *PluginManifest or *TaskManifest,
	// type-assert the returned interface{} to access specific fields) and a nil error.
	// Call this function ONCE per manifest file.
	//
	// Parameters:
	//   filePath: Path to the manifest YAML file.
	//   platformVersion: The current platform version string (e.g., "1.5.2") to check compatibility against (only for plugin manifests). Leave empty to skip check.
	//   artifactValidationType: Specifies which artifacts to validate ("discovery", "platform-binary", "cloudql-binary", "all"). Default is "all".
	//   skipArtifactValidation: If true, completely skips all artifact/image download and validation checks.
	//
	// Returns:
	//   interface{}: Either a *PluginManifest or *TaskManifest pointer if validation succeeds. Use type assertion to get the specific type.
	//   error: An error if reading, parsing, or validation fails.
	ProcessManifest(filePath string, platformVersion string, artifactValidationType string, skipArtifactValidation bool) (interface{}, error)

	// GetTaskDefinition reads a manifest file specifically expecting a *standalone* 'task' type,
	// parses it, validates its structure (including metadata), and returns the TaskManifest struct or an error.
	// Consider using ProcessManifest instead for a unified approach.
	GetTaskDefinition(filePath string) (*TaskManifest, error)

	// GetTaskDetailsFromPluginManifest extracts the details of the embedded 'discovery' task from an *already validated* PluginManifest.
	// It performs an additional validation step to ensure the task's image exists in the registry.
	// Use this function *after* successfully calling ProcessManifest for a plugin.
	//
	// Parameters:
	//   pluginManifest: A pointer to a validated PluginManifest struct (obtained from ProcessManifest).
	//
	// Returns:
	//   *TaskDetails: A struct containing details of the discovery task, including its validated image URI.
	//   error: An error if the input manifest is nil or if the image existence check fails.
	GetTaskDetailsFromPluginManifest(pluginManifest *PluginManifest) (*TaskDetails, error)

	// CheckPlatformSupport checks if a given PluginManifest supports a specific platform version string
	// based on the manifest's `supported-platform-versions` constraints.
	// Use this function *after* successfully calling ProcessManifest for a plugin.
	//
	// Parameters:
	//   manifest: A pointer to a validated PluginManifest struct (obtained from ProcessManifest).
	//   platformVersion: The platform version string to check (e.g., "1.5.2").
	//
	// Returns:
	//   bool: True if the platform version is supported, false otherwise.
	//   error: An error if the manifest is nil, platformVersion is empty, or if version/constraint parsing fails.
	CheckPlatformSupport(manifest *PluginManifest, platformVersion string) (bool, error)
}

// --- Concrete Implementation ---

// defaultValidator implements the Validator interface using the defined structs and helper methods.
type defaultValidator struct{}

// NewDefaultValidator creates a new instance of the default validator.
func NewDefaultValidator() Validator {
	return &defaultValidator{}
}

// --- Helper Functions ---

// isNonEmpty checks if a string is non-empty after trimming whitespace.
func isNonEmpty(s string) bool {
	return strings.TrimSpace(s) != ""
}

// validateMetadata performs structural, date format, and SPDX license validation on a Metadata object.
func (v *defaultValidator) validateMetadata(meta *Metadata, context string) error {
	if meta == nil {
		// This case should be caught before calling (e.g., in validateTaskManifestStructure), but handle defensively.
		return fmt.Errorf("%s: metadata section cannot be nil", context)
	}
	if !isNonEmpty(meta.Author) {
		return fmt.Errorf("%s: metadata.author is required", context)
	}
	if !isNonEmpty(meta.PublishedDate) {
		return fmt.Errorf("%s: metadata.published-date is required", context)
	}
	// Validate PublishedDate Format (YYYY-MM-DD)
	if _, err := time.Parse(PublishedDateFormat, meta.PublishedDate); err != nil {
		return fmt.Errorf("%s: invalid metadata.published-date format '%s' (expected '%s'): %w", context, meta.PublishedDate, PublishedDateFormat, err)
	}
	if !isNonEmpty(meta.Contact) {
		return fmt.Errorf("%s: metadata.contact is required", context)
	}
	if !isNonEmpty(meta.License) {
		return fmt.Errorf("%s: metadata.license is required", context)
	}
	// Validate License against SPDX list using github/go-spdx library
	// spdxexp.ValidateLicenses checks if the provided string(s) are valid license identifiers or expression parts.
	// For a single ID, valid should be true and invalidList should be empty.
	valid, invalidList := spdxexp.ValidateLicenses([]string{meta.License})
	if !valid {
		// Provide helpful error message including link to SPDX website and the invalid part found
		return fmt.Errorf("%s: metadata.license '%s' is not a valid SPDX license identifier (invalid parts: %v). See https://spdx.org/licenses/", context, meta.License, invalidList)
	}
	// Optional fields (Description, Website) don't need presence checks.
	return nil
}

// --- Interface Method Implementations ---

// ProcessManifest reads, identifies, validates structure (incl. date/SPDX license), checks platform (if applicable),
// and validates artifacts (if applicable and requested). This is the main entry point.
func (v *defaultValidator) ProcessManifest(filePath string, platformVersion string, artifactValidationType string, skipArtifactValidation bool) (interface{}, error) {
	log.Printf("Processing manifest file: %s (Platform Version: %s, Artifact Validation: %s, Skip Artifacts: %t)",
		filePath, platformVersion, artifactValidationType, skipArtifactValidation)

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file '%s': %w", filePath, err)
	}

	// 1. Determine Manifest Type
	var base manifestBase
	if err := yaml.Unmarshal(data, &base); err != nil {
		var yamlErr *yaml.TypeError
		if errors.As(err, &yamlErr) {
			return nil, fmt.Errorf("failed to parse manifest type from '%s' (YAML error on line %d: %s): %w", filePath, yamlErr.Line, yamlErr.Errors, err)
		}
		return nil, fmt.Errorf("failed to parse manifest type from '%s' (invalid YAML structure?): %w", filePath, err)
	}
	log.Printf("Detected manifest type: '%s'", base.Type)

	// 2. Process based on Type
	switch strings.ToLower(base.Type) {
	case ManifestTypePlugin:
		var manifest PluginManifest
		if err := yaml.Unmarshal(data, &manifest); err != nil {
			return nil, fmt.Errorf("failed to parse manifest file '%s' as plugin: %w", filePath, err)
		}

		log.Println("Validating plugin manifest structure (including metadata, date, license, embedded task)...")
		if err := v.validatePluginManifestStructure(&manifest); err != nil {
			return nil, fmt.Errorf("plugin manifest structure validation failed: %w", err)
		}
		log.Println("Plugin manifest structure validation successful.")

		// 2a. Check Platform Support (if version provided)
		if isNonEmpty(platformVersion) {
			log.Printf("Checking platform support for version: %s", platformVersion)
			supported, supportErr := v.CheckPlatformSupport(&manifest, platformVersion) // Use interface method on validated manifest
			if supportErr != nil {
				log.Printf("Warning: Error checking platform support for plugin '%s': %v", manifest.Plugin.Name, supportErr)
			} else {
				status := "IS NOT"
				if supported {
					status = "IS"
				}
				log.Printf("Platform version %s %s supported by plugin '%s' version '%s'.", platformVersion, status, manifest.Plugin.Name, manifest.Plugin.Version)
			}
		} else {
			log.Println("Skipping platform support check (no platform version provided).")
		}

		// 2b. Validate Artifacts (if requested)
		if !skipArtifactValidation {
			log.Println("Starting plugin artifact validation...")
			if err := v.validatePluginArtifacts(&manifest, artifactValidationType); err != nil {
				return nil, fmt.Errorf("plugin artifact validation failed: %w", err)
			}
			log.Println("Plugin artifact validation successful.")
		} else {
			log.Println("Skipping plugin artifact validation as requested.")
		}
		return &manifest, nil // Return the fully validated plugin manifest object

	case ManifestTypeTask:
		var manifest TaskManifest
		if err := yaml.Unmarshal(data, &manifest); err != nil {
			return nil, fmt.Errorf("failed to parse manifest file '%s' as task: %w", filePath, err)
		}

		log.Println("Validating task manifest structure (standalone, including metadata)...")
		// Pass true for isStandalone check
		if err := v.validateTaskManifestStructure(&manifest, true); err != nil {
			return nil, fmt.Errorf("standalone task manifest structure validation failed: %w", err)
		}
		log.Println("Standalone task manifest structure validation successful.")

		// 2c. Validate Task Image (if requested)
		if !skipArtifactValidation && isNonEmpty(manifest.ImageURL) {
			log.Println("Initiating standalone task image validation...")
			err = v.validateImageManifestExists(manifest.ImageURL) // Use internal helper
			if err != nil {
				return nil, fmt.Errorf("standalone task image validation failed for '%s': %w", manifest.ImageURL, err)
			}
			log.Println("Standalone task image validation successful.")
		} else {
			log.Println("Skipping standalone task image validation (image_url empty or validation skipped).")
		}
		return &manifest, nil // Return the fully validated task manifest object

	default:
		return nil, fmt.Errorf("unknown or unsupported manifest type '%s' in file '%s'", base.Type, filePath)
	}
}

// GetTaskDefinition reads a manifest file specifically expecting a 'task' type and parses it.
// Prefer ProcessManifest for a unified approach.
func (v *defaultValidator) GetTaskDefinition(filePath string) (*TaskManifest, error) {
	log.Printf("Loading standalone task definition from: %s", filePath)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file '%s': %w", filePath, err)
	}

	var base manifestBase
	if err := yaml.Unmarshal(data, &base); err != nil {
		return nil, fmt.Errorf("failed to parse manifest type from '%s' (invalid YAML?): %w", filePath, err)
	}
	if strings.ToLower(base.Type) != ManifestTypeTask {
		return nil, fmt.Errorf("expected manifest type '%s' but got '%s' in file '%s'", ManifestTypeTask, base.Type, filePath)
	}

	var manifest TaskManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest file '%s' as task (check syntax): %w", filePath, err)
	}

	if strings.ToLower(manifest.Type) != ManifestTypeTask {
		return nil, fmt.Errorf("consistency check failed: expected manifest type '%s' but parsed '%s' in file '%s'", ManifestTypeTask, manifest.Type, filePath)
	}

	// Perform structure validation for standalone task (includes metadata checks)
	log.Println("Validating standalone task manifest structure...")
	if err := v.validateTaskManifestStructure(&manifest, true); err != nil {
		return nil, fmt.Errorf("standalone task structure validation failed: %w", err)
	}
	log.Printf("Successfully loaded and validated standalone task definition for ID: %s", manifest.ID)
	return &manifest, nil
}

// GetTaskDetailsFromPluginManifest extracts discovery task details from an already validated PluginManifest.
// It performs an additional image existence check.
func (v *defaultValidator) GetTaskDetailsFromPluginManifest(pluginManifest *PluginManifest) (*TaskDetails, error) {
	if pluginManifest == nil {
		return nil, errors.New("input PluginManifest cannot be nil")
	}
	// Assume pluginManifest is already structurally validated by ProcessManifest

	log.Printf("Getting task details from pre-validated plugin manifest: %s (Version: %s)", pluginManifest.Plugin.Name, pluginManifest.Plugin.Version)

	// 1. Access the embedded discovery task
	discoveryTask := pluginManifest.Plugin.Components.Discovery

	// 2. Validate the Image URL existence (Format check was done during ProcessManifest)
	log.Printf("Validating image existence for discovery task (ID: %s, Image: %s)...", discoveryTask.ID, discoveryTask.ImageURL)
	if err := v.validateImageManifestExists(discoveryTask.ImageURL); err != nil {
		// Wrap error for better context
		return nil, fmt.Errorf("discovery task image URI existence check failed for '%s' (plugin: %s): %w",
			discoveryTask.ImageURL, pluginManifest.Plugin.Name, err)
	}
	log.Printf("Image existence validated successfully for: %s", discoveryTask.ImageURL)

	// 3. Populate the TaskDetails struct
	details := &TaskDetails{
		PluginName:        pluginManifest.Plugin.Name,
		TaskID:            discoveryTask.ID,
		TaskName:          discoveryTask.Name,
		ValidatedImageURI: discoveryTask.ImageURL, // Use the validated URL
		Command:           discoveryTask.Command,
		Timeout:           discoveryTask.Timeout,
		ScaleConfig:       discoveryTask.ScaleConfig,
		Params:            discoveryTask.Params,
		Configs:           discoveryTask.Configs,
		RunSchedule:       discoveryTask.RunSchedule,
	}

	log.Printf("Successfully retrieved and validated task details for task ID '%s' from plugin '%s'", details.TaskID, details.PluginName)
	return details, nil
}

// CheckPlatformSupport checks platform compatibility using an already validated PluginManifest.
func (v *defaultValidator) CheckPlatformSupport(manifest *PluginManifest, platformVersion string) (bool, error) {
	if manifest == nil {
		return false, errors.New("plugin manifest cannot be nil for platform support check")
	}
	// Assume manifest is already structurally validated by ProcessManifest
	if !isNonEmpty(platformVersion) {
		return false, errors.New("platformVersion cannot be empty for platform support check")
	}

	// Parse the current platform version
	currentV, err := semver.NewVersion(platformVersion)
	if err != nil {
		return false, fmt.Errorf("invalid platform version format '%s': %w", platformVersion, err)
	}

	supportedVersions := manifest.Plugin.SupportedPlatformVersions
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
			log.Printf("Platform version '%s' matches constraint '%s' for plugin '%s'.", platformVersion, constraintStr, manifest.Plugin.Name)
			return true, nil // Found a matching constraint
		}
	}

	// If no constraint matched
	log.Printf("Platform version '%s' does not satisfy any supported-platform-versions constraints %v for plugin '%s'.",
		platformVersion, supportedVersions, manifest.Plugin.Name)
	return false, nil
}

// --- Internal Validation Logic ---

// validatePluginManifestStructure performs structural checks specific to 'plugin' manifests,
// including metadata (date, license) and the embedded discovery task.
func (v *defaultValidator) validatePluginManifestStructure(manifest *PluginManifest) error {
	if manifest == nil {
		return errors.New("plugin manifest cannot be nil")
	}
	// --- Top Level Plugin Fields ---
	if !isNonEmpty(manifest.APIVersion) || manifest.APIVersion != APIVersionV1 {
		return fmt.Errorf("top level: api-version is required and must be '%s', got: '%s'", APIVersionV1, manifest.APIVersion)
	}
	if manifest.Type != ManifestTypePlugin {
		return fmt.Errorf("top level: type is required and must be '%s', got: '%s'", ManifestTypePlugin, manifest.Type)
	}

	// --- Plugin Block Fields ---
	plugin := manifest.Plugin // Alias for readability
	if !isNonEmpty(plugin.Name) {
		return errors.New("plugin.name is required")
	}
	if !isNonEmpty(plugin.Version) {
		return errors.New("plugin.version is required")
	}
	if _, err := semver.NewVersion(plugin.Version); err != nil {
		return fmt.Errorf("plugin.version: invalid semantic version format '%s': %w", plugin.Version, err)
	}
	if len(plugin.SupportedPlatformVersions) == 0 {
		return errors.New("plugin.supported-platform-versions requires at least one constraint entry")
	}
	for i, constraintStr := range plugin.SupportedPlatformVersions {
		if !isNonEmpty(constraintStr) {
			return fmt.Errorf("plugin.supported-platform-versions entry %d: cannot be empty", i)
		}
		if _, err := semver.NewConstraint(constraintStr); err != nil {
			return fmt.Errorf("plugin.supported-platform-versions entry %d ('%s'): is not a valid semantic version constraint: %w", i, constraintStr, err)
		}
	}

	// --- Metadata Block Fields ---
	// Use the helper function for metadata validation
	if err := v.validateMetadata(&plugin.Metadata, fmt.Sprintf("plugin '%s' metadata", plugin.Name)); err != nil {
		return err // Error from validateMetadata is already contextualized
	}

	// --- Components Block Fields ---
	components := plugin.Components // Alias for readability

	// *** Validate Embedded Discovery Task Structure ***
	// Pass 'false' for isStandalone. Metadata and APIVersion checks happen inside.
	if err := v.validateTaskManifestStructure(&components.Discovery, false); err != nil { // false = embedded
		// Add context to the error from the task validation
		return fmt.Errorf("plugin.components.discovery task validation failed: %w", err)
	}

	// *** Validate Downloadable Component References ***
	platformComp := components.PlatformBinary
	cloudqlComp := components.CloudQLBinary

	if !isNonEmpty(platformComp.URI) {
		return errors.New("plugin.components.platform-binary.uri is required")
	}
	if !isNonEmpty(cloudqlComp.URI) {
		return errors.New("plugin.components.cloudql-binary.uri is required")
	}

	// If URIs are the same, both components MUST specify a PathInArchive.
	if platformComp.URI == cloudqlComp.URI {
		log.Printf("Info: PlatformBinary and CloudQLBinary share the same URI: %s. PathInArchive is required for both.", platformComp.URI)
		if !isNonEmpty(platformComp.PathInArchive) {
			return fmt.Errorf("plugin.components.platform-binary.path-in-archive is required when its URI ('%s') matches cloudql-binary.uri", platformComp.URI)
		}
		if !isNonEmpty(cloudqlComp.PathInArchive) {
			return fmt.Errorf("plugin.components.cloudql-binary.path-in-archive is required when its URI ('%s') matches platform-binary.uri", cloudqlComp.URI)
		}
		if platformComp.PathInArchive == cloudqlComp.PathInArchive {
			return fmt.Errorf("plugin.components.platform-binary.path-in-archive ('%s') cannot be the same as cloudql-binary.path-in-archive ('%s') when URIs match", platformComp.PathInArchive, cloudqlComp.PathInArchive)
		}
	}

	// --- Sample Data (Optional) ---
	if plugin.SampleData != nil {
		if !isNonEmpty(plugin.SampleData.URI) {
			return errors.New("plugin.sample-data.uri is required when the sample-data section is present")
		}
	}

	return nil
}

// validateTaskManifestStructure performs structural checks specific to 'task' manifests.
// The isStandalone flag determines if APIVersion, Metadata, and SupportedPlatformVersions are required (true)
// or must be absent (false, for embedded discovery tasks).
func (v *defaultValidator) validateTaskManifestStructure(manifest *TaskManifest, isStandalone bool) error {
	if manifest == nil {
		return errors.New("task manifest cannot be nil")
	}

	taskDesc := fmt.Sprintf("task (ID: %s)", manifest.ID)
	if !isNonEmpty(manifest.ID) {
		taskDesc = "task (ID missing)"
	}

	// --- Standalone vs Embedded Checks ---
	if isStandalone {
		// These fields are REQUIRED for standalone tasks.
		if !isNonEmpty(manifest.APIVersion) || manifest.APIVersion != APIVersionV1 {
			return fmt.Errorf("%s: api-version is required and must be '%s' for standalone task, got: '%s'", taskDesc, APIVersionV1, manifest.APIVersion)
		}
		if manifest.Metadata == nil {
			return fmt.Errorf("%s: metadata section is required for standalone task", taskDesc)
		}
		// Validate the metadata content using the helper
		if err := v.validateMetadata(manifest.Metadata, fmt.Sprintf("%s standalone metadata", taskDesc)); err != nil {
			return err // Error already contextualized
		}
		if len(manifest.SupportedPlatformVersions) == 0 {
			return fmt.Errorf("%s: supported-platform-versions requires at least one constraint entry for standalone task", taskDesc)
		}
		for i, constraintStr := range manifest.SupportedPlatformVersions {
			if !isNonEmpty(constraintStr) {
				return fmt.Errorf("%s: supported-platform-versions entry %d cannot be empty for standalone task", taskDesc, i)
			}
			if _, err := semver.NewConstraint(constraintStr); err != nil {
				return fmt.Errorf("%s: supported-platform-versions entry %d ('%s') is not a valid semantic version constraint for standalone task: %w", taskDesc, i, constraintStr, err)
			}
		}
	} else {
		// These fields MUST NOT be present for embedded discovery tasks (they are inherited from the plugin).
		if isNonEmpty(manifest.APIVersion) {
			return fmt.Errorf("%s: embedded discovery task must not contain api-version (it's inherited from plugin), but found: '%s'", taskDesc, manifest.APIVersion)
		}
		if manifest.Metadata != nil {
			return fmt.Errorf("%s: embedded discovery task must not contain metadata section (it's inherited from plugin)", taskDesc)
		}
		if len(manifest.SupportedPlatformVersions) > 0 {
			return fmt.Errorf("%s: embedded discovery task must not contain supported-platform-versions (it's inherited from plugin), but found: %v", taskDesc, manifest.SupportedPlatformVersions)
		}
	}

	// --- Common Task Field Checks (Required for both Standalone and Embedded) ---
	if !isNonEmpty(manifest.ID) {
		return errors.New("task: id is required") // Use generic 'task' as ID might be missing
	}
	if !isNonEmpty(manifest.Name) {
		return fmt.Errorf("%s: name is required", taskDesc)
	}
	// Note: Task has its own Description field, separate from Metadata.Description
	if !isNonEmpty(manifest.Description) {
		return fmt.Errorf("%s: description is required", taskDesc)
	}
	// IsEnabled is a boolean, always present.
	if manifest.Type != ManifestTypeTask {
		return fmt.Errorf("%s: type is required and must be '%s', got: '%s'", taskDesc, ManifestTypeTask, manifest.Type)
	}
	if !isNonEmpty(manifest.ImageURL) {
		return fmt.Errorf("%s: image_url is required", taskDesc)
	}
	// ** Enforce Digest Format for ImageURL **
	if !imageDigestRegex.MatchString(manifest.ImageURL) {
		return fmt.Errorf("%s: image_url ('%s') must be in digest format (e.g., registry/repository/image@sha256:hash)", taskDesc, manifest.ImageURL)
	}
	if !isNonEmpty(manifest.Command) {
		return fmt.Errorf("%s: command is required", taskDesc)
	}
	if !isNonEmpty(manifest.Timeout) {
		return fmt.Errorf("%s: timeout is required", taskDesc)
	}
	timeoutDuration, err := time.ParseDuration(manifest.Timeout)
	if err != nil {
		return fmt.Errorf("%s: invalid timeout format '%s', requires format like '5m', '1h30s': %w", taskDesc, manifest.Timeout, err)
	}
	twentyFourHours := 24 * time.Hour
	if timeoutDuration >= twentyFourHours {
		return fmt.Errorf("%s: timeout '%s' must be less than 24 hours (%s)", taskDesc, manifest.Timeout, twentyFourHours)
	}
	if timeoutDuration <= 0 {
		return fmt.Errorf("%s: timeout '%s' must be a positive duration", taskDesc, manifest.Timeout)
	}

	// --- Scale Config Check ---
	sc := manifest.ScaleConfig // Alias for readability
	if !isNonEmpty(sc.LagThreshold) {
		return fmt.Errorf("%s: scale_config.lag_threshold is required and must be a non-empty string representing a positive integer", taskDesc)
	}
	lagInt, err := strconv.Atoi(sc.LagThreshold)
	if err != nil {
		return fmt.Errorf("%s: scale_config.lag_threshold ('%s') must be a valid integer string: %w", taskDesc, sc.LagThreshold, err)
	}
	if lagInt <= 0 {
		return fmt.Errorf("%s: scale_config.lag_threshold ('%s' -> %d) must represent a positive integer > 0", taskDesc, sc.LagThreshold, lagInt)
	}
	if sc.MinReplica < 0 {
		return fmt.Errorf("%s: scale_config.min_replica (%d) cannot be negative", taskDesc, sc.MinReplica)
	}
	if sc.MaxReplica < sc.MinReplica {
		return fmt.Errorf("%s: scale_config.max_replica (%d) must be greater than or equal to min_replica (%d)", taskDesc, sc.MaxReplica, sc.MinReplica)
	}

	// --- Params & Configs Check (Presence) ---
	if manifest.Params == nil {
		return fmt.Errorf("%s: params field is required (use an empty list [] if no parameters)", taskDesc)
	}
	if manifest.Configs == nil {
		return fmt.Errorf("%s: configs field is required (use an empty list [] if no configs)", taskDesc)
	}

	// --- Run Schedule Check ---
	if manifest.RunSchedule == nil {
		return fmt.Errorf("%s: run_schedule field is required (must contain at least one entry)", taskDesc)
	}
	if len(manifest.RunSchedule) < 1 {
		return fmt.Errorf("%s: run_schedule must contain at least one schedule entry", taskDesc)
	}

	// Check schedule entries and ensure default exists if params are defined
	defaultScheduleFound := false
	paramSet := make(map[string]struct{})
	for _, p := range manifest.Params {
		if !isNonEmpty(p) {
			return fmt.Errorf("%s: parameter names in top-level 'params' list cannot be empty", taskDesc)
		}
		if _, exists := paramSet[p]; exists {
			return fmt.Errorf("%s: duplicate parameter name '%s' found in top-level 'params' list", taskDesc, p)
		}
		paramSet[p] = struct{}{}
	}

	scheduleIDs := make(map[string]struct{})
	for i, schedule := range manifest.RunSchedule {
		entryContext := fmt.Sprintf("%s run_schedule entry %d", taskDesc, i)
		if isNonEmpty(schedule.ID) {
			entryContext = fmt.Sprintf("%s run_schedule entry %d (id: '%s')", taskDesc, i, schedule.ID)
			if _, exists := scheduleIDs[schedule.ID]; exists {
				return fmt.Errorf("%s: duplicate schedule ID '%s' found", entryContext, schedule.ID)
			}
			scheduleIDs[schedule.ID] = struct{}{}
		} else {
			return fmt.Errorf("%s: id field is required and cannot be empty", entryContext)
		}

		if schedule.Params == nil {
			return fmt.Errorf("%s: params map field is required (use {} for no parameters specific to this schedule)", entryContext)
		}
		if !isNonEmpty(schedule.Frequency) {
			return fmt.Errorf("%s: frequency field is required", entryContext)
		}

		// Check if this is a default schedule and if it covers all required top-level params
		if schedule.ID == "describe-all" || schedule.ID == "default" {
			defaultScheduleFound = true
			log.Printf("Info: Found default schedule entry: %s", entryContext)
			for requiredParam := range paramSet {
				if _, ok := schedule.Params[requiredParam]; !ok {
					return fmt.Errorf("%s: this default schedule is missing required parameter '%s' (defined in top-level params list)", entryContext, requiredParam)
				}
			}
		} else {
			// For non-default schedules, ensure they don't define params *not* in the top-level list.
			for definedParam := range schedule.Params {
				if _, ok := paramSet[definedParam]; !ok {
					return fmt.Errorf("%s: defines parameter '%s' which is not declared in the task's top-level 'params' list %v", entryContext, definedParam, manifest.Params)
				}
			}
		}
	}

	// If the task defines parameters, a default schedule covering them is mandatory.
	if !defaultScheduleFound && len(paramSet) > 0 {
		return fmt.Errorf("%s: task defines parameters (%v), but no run_schedule entry with id 'describe-all' or 'default' was found to provide default values", taskDesc, manifest.Params)
	}

	return nil
}

// --- Artifact Validation Logic (Unchanged from previous version) ---

// validatePluginArtifacts handles the download and validation logic for 'plugin' type artifacts
// based on the requested artifactType ("all", "discovery", "platform-binary", "cloudql-binary").
func (v *defaultValidator) validatePluginArtifacts(manifest *PluginManifest, artifactType string) error {
	if manifest == nil {
		return errors.New("plugin manifest cannot be nil for artifact validation")
	}

	// Normalize and determine which artifacts to validate
	normalizedType := strings.ToLower(strings.TrimSpace(artifactType))
	if !isNonEmpty(normalizedType) {
		normalizedType = ArtifactTypeAll // Default to validating all artifacts
	}
	log.Printf("--- Starting Plugin Artifact Validation (Requested Type: %s) ---", normalizedType)

	validateDiscovery := false
	validatePlatform := false
	validateCloudQL := false

	switch normalizedType {
	case ArtifactTypeAll:
		validateDiscovery = true
		validatePlatform = true
		validateCloudQL = true
		log.Println("Scope: Validating Discovery Image, PlatformBinary, and CloudQLBinary artifacts.")
	case ArtifactTypeDiscovery:
		validateDiscovery = true
		log.Println("Scope: Validating only Discovery Image artifact.")
	case ArtifactTypePlatformBinary:
		validatePlatform = true
		log.Println("Scope: Validating only PlatformBinary artifact.")
	case ArtifactTypeCloudQLBinary:
		validateCloudQL = true
		log.Println("Scope: Validating only CloudQLBinary artifact.")
	default:
		return fmt.Errorf("invalid artifactType '%s'. Must be one of: '%s', '%s', '%s', or '%s' (or empty)",
			artifactType, ArtifactTypeDiscovery, ArtifactTypePlatformBinary, ArtifactTypeCloudQLBinary, ArtifactTypeAll)
	}

	var wg sync.WaitGroup
	errChan := make(chan error, 3) // Buffered channel to collect errors from goroutines
	var platformData []byte        // To store downloaded data for shared URI case

	platformComp := manifest.Plugin.Components.PlatformBinary
	cloudqlComp := manifest.Plugin.Components.CloudQLBinary
	discoveryImageURL := manifest.Plugin.Components.Discovery.ImageURL // Use ImageURL from embedded task

	// --- Validate Discovery Task Image ---
	if validateDiscovery {
		log.Printf("Validating Discovery Image: %s", discoveryImageURL)
		// ImageURL format (digest) is checked during structure validation.
		discoveryErr := v.validateImageManifestExists(discoveryImageURL) // This performs retries internally
		if discoveryErr != nil {
			log.Printf("Error validating Discovery Image '%s': %v", discoveryImageURL, discoveryErr)
			errChan <- fmt.Errorf("discovery image validation failed for '%s': %w", discoveryImageURL, discoveryErr)
		} else {
			log.Printf("Discovery Image validation successful: %s", discoveryImageURL)
		}
	}

	// --- Validate Platform Binary ---
	if validatePlatform {
		wg.Add(1)
		go func(comp Component) {
			defer wg.Done()
			log.Printf("Validating PlatformBinary artifact: %s", comp.URI)
			var err error
			// Store downloaded data in the shared variable if successful
			platformData, err = v.validateSingleDownloadableComponent(comp, ArtifactTypePlatformBinary) // Retries handled inside
			if err != nil {
				log.Printf("Error validating PlatformBinary artifact '%s': %v", comp.URI, err)
				errChan <- fmt.Errorf("platform-binary artifact validation failed for URI '%s': %w", comp.URI, err)
				// Ensure platformData is nil on error to signal failure for shared URI check
				platformData = nil
			} else {
				log.Printf("PlatformBinary artifact validation successful: %s", comp.URI)
			}
		}(platformComp)
	}

	// --- Validate CloudQL Binary (Separate URI) ---
	if validateCloudQL && platformComp.URI != cloudqlComp.URI {
		wg.Add(1)
		go func(comp Component) {
			defer wg.Done()
			log.Printf("Validating CloudQLBinary artifact (separate URI): %s", comp.URI)
			_, err := v.validateSingleDownloadableComponent(comp, ArtifactTypeCloudQLBinary) // Retries handled inside
			if err != nil {
				log.Printf("Error validating CloudQLBinary artifact (separate URI) '%s': %v", comp.URI, err)
				errChan <- fmt.Errorf("cloudql-binary artifact validation failed for URI '%s': %w", comp.URI, err)
			} else {
				log.Printf("CloudQLBinary artifact validation successful (separate URI): %s", comp.URI)
			}
		}(cloudqlComp)
	}

	// Wait for concurrent validations (Discovery is synchronous, Platform/CloudQL(separate) are concurrent)
	wg.Wait()

	// --- Validate CloudQL Binary (Shared URI Case) ---
	// This runs after the platform binary download attempt has finished.
	if validateCloudQL && platformComp.URI == cloudqlComp.URI {
		log.Printf("Validating CloudQLBinary artifact (shared URI %s, path %s)...", cloudqlComp.URI, cloudqlComp.PathInArchive)

		// Check if platform validation was requested and if it succeeded (platformData will be non-nil)
		if validatePlatform {
			if platformData == nil {
				// Platform validation was requested but failed (error already sent to errChan).
				// Cannot proceed with path check. Add a contextual error.
				errMsg := fmt.Errorf("cannot validate cloudql-binary path '%s' because shared archive download/validation failed for URI '%s'",
					cloudqlComp.PathInArchive, cloudqlComp.URI)
				log.Printf("Error validating CloudQLBinary artifact (shared URI): %v", errMsg)
				// Avoid sending duplicate error if platform already failed
				// errChan <- errMsg // This might be redundant
			} else {
				// Platform binary download succeeded, now check the path within the data.
				log.Printf("Checking path '%s' within shared archive data from %s...", cloudqlComp.PathInArchive, cloudqlComp.URI)
				err := v.validateArchivePathExists(platformData, cloudqlComp.PathInArchive, cloudqlComp.URI)
				if err != nil {
					cloudqlErr := fmt.Errorf("cloudql-binary path validation failed: %w", err) // Error from validateArchivePathExists is descriptive
					log.Printf("Error validating CloudQLBinary artifact path (shared URI): %v", cloudqlErr)
					errChan <- cloudqlErr
				} else {
					log.Printf("CloudQLBinary artifact validation successful (shared URI path '%s' exists).", cloudqlComp.PathInArchive)
				}
			}
		} else {
			// CloudQL validation requested for shared URI, but Platform validation was *not* requested.
			// This is an inconsistent state based on the logic, but handle defensively.
			// We need to download the artifact specifically for the CloudQL path check.
			log.Printf("Warning: CloudQL validation requested for shared URI '%s', but PlatformBinary validation was skipped. Downloading artifact again for path check.", platformComp.URI)
			// Handle this synchronously now to avoid complex channel/goroutine management here
			sharedDataForCloudQL, downloadErr := v.validateSingleDownloadableComponent(platformComp, "shared archive for CloudQL path check")
			if downloadErr != nil {
				log.Printf("Error downloading shared archive '%s' for CloudQL path check: %v", platformComp.URI, downloadErr)
				errChan <- fmt.Errorf("failed to download shared archive '%s' for cloudql-binary path check: %w", platformComp.URI, downloadErr)
			} else if sharedDataForCloudQL != nil {
				log.Printf("Checking path '%s' within shared archive data from %s...", cloudqlComp.PathInArchive, cloudqlComp.URI)
				err := v.validateArchivePathExists(sharedDataForCloudQL, cloudqlComp.PathInArchive, cloudqlComp.URI)
				if err != nil {
					cloudqlErr := fmt.Errorf("cloudql-binary path validation failed: %w", err)
					log.Printf("Error validating CloudQLBinary artifact path (shared URI): %v", cloudqlErr)
					errChan <- cloudqlErr
				} else {
					log.Printf("CloudQLBinary artifact validation successful (shared URI path '%s' exists).", cloudqlComp.PathInArchive)
				}
			}
		}
	}

	// Close the error channel and collect errors
	close(errChan)
	var combinedErrors []string
	for err := range errChan {
		combinedErrors = append(combinedErrors, err.Error())
	}

	if len(combinedErrors) > 0 {
		return fmt.Errorf("one or more artifact validations failed: %s", strings.Join(combinedErrors, "; "))
	}

	log.Println("--- Plugin Artifact Validation Completed Successfully ---")
	return nil
}

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
		log.Printf("Attempting to resolve digest '%s' in repository '%s'...", ref.Reference, repo.Reference().Repository)
		_, err = repo.Resolve(ctx, ref.Reference)
		cancel() // Release context resources after the operation

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
		log.Println("Checksum verification skipped: No checksum provided in the manifest.")
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
