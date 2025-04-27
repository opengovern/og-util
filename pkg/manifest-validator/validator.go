// Package pluginmanifest provides utilities for loading, validating, and verifying plugin manifests
// (both 'plugin' and 'task' types) and their associated downloadable components.
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
	_ "github.com/opencontainers/image-spec/specs-go/v1" // OCI spec alias
	"gopkg.in/yaml.v3"
	"oras.land/oras-go/v2/registry"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/errcode"
)

// --- Struct Definitions ---
// (Struct definitions for PluginManifest, TaskManifest, Component, Metadata, etc. remain the same)

// Component represents a single functional part of the plugin or task.
type Component struct {
	URI           string `yaml:"uri,omitempty" json:"uri,omitempty"`
	ImageURI      string `yaml:"image-uri,omitempty" json:"image-uri,omitempty"` // Used by Plugin:Discovery
	PathInArchive string `yaml:"path-in-archive,omitempty" json:"path-in-archive,omitempty"`
	Checksum      string `yaml:"checksum,omitempty" json:"checksum,omitempty"`
}

// Metadata holds descriptive information about the plugin.
type Metadata struct {
	Author        string `yaml:"author" json:"author"`
	PublishedDate string `yaml:"published-date" json:"published-date"`
	Contact       string `yaml:"contact" json:"contact"`
	License       string `yaml:"license" json:"license"`
	Description   string `yaml:"description,omitempty" json:"description,omitempty"`
	Website       string `yaml:"website,omitempty" json:"website,omitempty"`
}

// Plugin defines the core details of the plugin.
type Plugin struct {
	Name                      string           `yaml:"name" json:"name"`
	Version                   string           `yaml:"version" json:"version"`
	SupportedPlatformVersions []string         `yaml:"supported-platform-versions" json:"supported-platform-versions"`
	Metadata                  Metadata         `yaml:"metadata" json:"metadata"`
	Components                PluginComponents `yaml:"components" json:"components"`
	SampleData                *Component       `yaml:"sample-data,omitempty" json:"sample-data,omitempty"`
}

// PluginComponents holds the different component definitions.
type PluginComponents struct {
	Discovery      Component `yaml:"discovery" json:"discovery"`
	PlatformBinary Component `yaml:"platform-binary" json:"platform-binary"`
	CloudQLBinary  Component `yaml:"cloudql-binary" json:"cloudql-binary"`
}

// PluginManifest is the top-level structure for the 'plugin' type manifest file.
type PluginManifest struct {
	APIVersion string `yaml:"api-version" json:"api-version"`
	Type       string `yaml:"type" json:"type"` // Should be "plugin"
	Plugin     Plugin `yaml:"plugin" json:"plugin"`
}

// ScaleConfig defines the scaling parameters for a task.
type ScaleConfig struct {
	LagThreshold string `yaml:"lag_threshold" json:"lag_threshold"`
	MinReplica   int    `yaml:"min_replica" json:"min_replica"`
	MaxReplica   int    `yaml:"max_replica" json:"max_replica"`
}

// RunScheduleEntry defines a single scheduled run configuration for a task.
type RunScheduleEntry struct {
	ID        string            `yaml:"id" json:"id"`
	Params    map[string]string `yaml:"params" json:"params"`
	Frequency string            `yaml:"frequency" json:"frequency"`
}

// TaskManifest is the top-level structure for the 'task' type manifest file.
type TaskManifest struct {
	ID          string             `yaml:"id" json:"id"`
	Name        string             `yaml:"name" json:"name"`
	Description string             `yaml:"description" json:"description"`
	IsEnabled   bool               `yaml:"is_enabled" json:"is_enabled"`
	Type        string             `yaml:"type" json:"type"` // Should be "task"
	ImageURL    string             `yaml:"image_url" json:"image_url"`
	Command     string             `yaml:"command" json:"command"`
	Timeout     string             `yaml:"timeout" json:"timeout"`
	ScaleConfig ScaleConfig        `yaml:"scale_config" json:"scale_config"`
	Params      []string           `yaml:"params" json:"params"`
	Configs     []interface{}      `yaml:"configs" json:"configs"`
	RunSchedule []RunScheduleEntry `yaml:"run_schedule" json:"run_schedule"`
}

// --- Generic Manifest Type for initial parsing ---
type manifestBase struct {
	Type string `yaml:"type"`
}

// --- Configuration Constants ---
const (
	MaxRegistryRetries     = 3
	MaxDownloadRetries     = 3
	InitialBackoffDuration = 1 * time.Second
	ConnectTimeout         = 10 * time.Second
	TLSHandshakeTimeout    = 10 * time.Second
	ResponseHeaderTimeout  = 15 * time.Second
	OverallRequestTimeout  = 60 * time.Second
	MaxDownloadSizeBytes   = 1 * 1024 * 1024 * 1024 // 1 GiB limit

	// ArtifactTypeDiscovery identifies the discovery image component (for plugin manifests).
	ArtifactTypeDiscovery = "discovery"
	// ArtifactTypePlatformBinary identifies the platform-binary component (for plugin manifests).
	ArtifactTypePlatformBinary = "platform-binary"
	// ArtifactTypeCloudQLBinary identifies the cloudql-binary component (for plugin manifests).
	ArtifactTypeCloudQLBinary = "cloudql-binary"
	// ArtifactTypeAll indicates validation for all relevant components (for plugin manifests).
	ArtifactTypeAll = "all"
)

// --- Global HTTP Client ---
var httpClient *http.Client

// --- Regular Expressions ---
var imageDigestRegex = regexp.MustCompile(`^.+@sha256:[a-fA-F0-9]{64}$`)
var dockerURIRegex = regexp.MustCompile(`^([a-zA-Z0-9.-]+(:\d+)?\/)?([a-zA-Z0-9._-]+)(\/[a-zA-Z0-9._-]+)*(:[a-zA-Z0-9._-]+|@sha256:[a-fA-F0-9]{64})?$`)

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

// Validator defines the interface for processing and validating manifests.
type Validator interface {
	// ProcessManifest reads a manifest file, determines its type ("plugin" or "task"),
	// validates its structure, and optionally validates its artifacts/image.
	// artifactValidationType applies only to "plugin" manifests. Valid types are
	// "discovery", "platform-binary", "cloudql-binary", "all", or "" (same as "all").
	// Set skipArtifactValidation to true to bypass artifact/image checks completely.
	ProcessManifest(filePath string, platformVersion string, artifactValidationType string, skipArtifactValidation bool) (interface{}, error)
}

// --- Concrete Implementation ---

// defaultValidator implements the Validator interface.
type defaultValidator struct{}

// NewDefaultValidator creates a new instance of the default validator.
func NewDefaultValidator() Validator { // Return the new interface type
	return &defaultValidator{}
}

// --- Helper Function ---
func isNonEmpty(s string) bool {
	return strings.TrimSpace(s) != ""
}

// --- Interface Method Implementations ---

// ProcessManifest reads, identifies, validates structure, checks platform (if applicable),
// and validates artifacts (if applicable and requested).
func (v *defaultValidator) ProcessManifest(filePath string, platformVersion string, artifactValidationType string, skipArtifactValidation bool) (interface{}, error) {
	log.Printf("Processing manifest file: %s", filePath)

	// 1. Read file content
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file '%s': %w", filePath, err)
	}

	// 2. Determine manifest type
	var base manifestBase
	err = yaml.Unmarshal(data, &base)
	if err != nil {
		return nil, fmt.Errorf("failed to parse manifest type from '%s': %w", filePath, err)
	}
	log.Printf("Detected manifest type: %s", base.Type)

	// 3. Process based on type
	switch strings.ToLower(base.Type) {
	case "plugin":
		var manifest PluginManifest
		err = yaml.Unmarshal(data, &manifest)
		if err != nil {
			return nil, fmt.Errorf("failed to parse manifest file '%s' as plugin: %w", filePath, err)
		}

		log.Println("Validating plugin manifest structure...")
		err = v.validatePluginManifestStructure(&manifest) // Use internal helper
		if err != nil {
			return nil, fmt.Errorf("plugin manifest structure validation failed: %w", err)
		}
		log.Println("Plugin manifest structure validation successful.")

		if isNonEmpty(platformVersion) {
			log.Printf("Checking platform support for version: %s", platformVersion)
			supported, err := v.checkPlatformSupport(&manifest, platformVersion) // Use internal helper
			if err != nil {
				log.Printf("Warning: Error checking platform support: %v", err) /* Don't fail */
			} else {
				if supported {
					log.Printf("Platform version %s IS supported.", platformVersion)
				} else {
					log.Printf("Platform version %s is NOT supported.", platformVersion) /* Potentially fail here */
				}
			}
		} else {
			log.Println("Skipping platform support check.")
		}

		if !skipArtifactValidation {
			err = v.validatePluginArtifacts(&manifest, artifactValidationType) // Use internal helper
			if err != nil {
				return nil, fmt.Errorf("plugin artifact validation failed: %w", err)
			}
			log.Println("Plugin artifact validation successful.")
		} else {
			log.Println("Skipping plugin artifact validation.")
		}

		return &manifest, nil // Return the parsed and validated manifest

	case "task":
		var manifest TaskManifest
		err = yaml.Unmarshal(data, &manifest)
		if err != nil {
			return nil, fmt.Errorf("failed to parse manifest file '%s' as task: %w", filePath, err)
		}

		log.Println("Validating task manifest structure...")
		err = v.validateTaskManifestStructure(&manifest) // Use internal helper
		if err != nil {
			return nil, fmt.Errorf("task manifest structure validation failed: %w", err)
		}
		log.Println("Task manifest structure validation successful.")

		// Add task-specific artifact validation if needed in the future (e.g., image check)
		if !skipArtifactValidation && isNonEmpty(manifest.ImageURL) {
			log.Println("Initiating Task image validation...")
			// Note: Task image validation currently checks digest OR tag format validity,
			// but only performs registry existence check if it's a digest.
			// Modify validateImageManifestExists if tag resolution is needed.
			if imageDigestRegex.MatchString(manifest.ImageURL) {
				err = v.validateImageManifestExists(manifest.ImageURL)
				if err != nil {
					// Combine error reporting
					return nil, fmt.Errorf("task image validation failed: %w", err)
				}
				log.Println("Task image validation successful.")
			} else if dockerURIRegex.MatchString(manifest.ImageURL) {
				log.Printf("Task image URI '%s' uses a tag. Skipping registry existence check (only digest format is checked).", manifest.ImageURL)
			} else {
				// This case should be caught by structure validation, but double-check
				return nil, fmt.Errorf("task image validation failed: invalid image_url format '%s'", manifest.ImageURL)
			}

		} else if !skipArtifactValidation {
			log.Println("Skipping task image validation (image_url empty or validation skipped).")
		}

		return &manifest, nil // Return the parsed and validated manifest

	default:
		return nil, fmt.Errorf("unknown manifest type '%s' in file %s", base.Type, filePath)
	}
}

// --- Internal Helper Methods ---

// validatePluginManifestStructure performs structural, metadata, and format checks specific to 'plugin' manifests.
// Renamed from ValidateManifestStructure to be internal.
func (v *defaultValidator) validatePluginManifestStructure(manifest *PluginManifest) error {
	if manifest == nil {
		return fmt.Errorf("plugin manifest cannot be nil")
	}
	if !isNonEmpty(manifest.APIVersion) || manifest.APIVersion != "v1" {
		return fmt.Errorf("api-version must be 'v1'")
	}
	if manifest.Type != "plugin" {
		return fmt.Errorf("type must be 'plugin'")
	} // Check type consistency
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

// validateTaskManifestStructure performs structural checks specific to 'task' manifests.
// Renamed from ValidateTaskManifestStructure to be internal.
func (v *defaultValidator) validateTaskManifestStructure(manifest *TaskManifest) error {
	if manifest == nil {
		return fmt.Errorf("task manifest cannot be nil")
	}
	if !isNonEmpty(manifest.ID) {
		return fmt.Errorf("id is required")
	}
	if !isNonEmpty(manifest.Name) {
		return fmt.Errorf("name is required")
	}
	if !isNonEmpty(manifest.Description) {
		return fmt.Errorf("description is required")
	}
	if manifest.Type != "task" {
		return fmt.Errorf("type must be 'task'")
	}
	if !isNonEmpty(manifest.ImageURL) {
		return fmt.Errorf("image_url is required")
	}
	if !dockerURIRegex.MatchString(manifest.ImageURL) {
		return fmt.Errorf("image_url ('%s') is not a valid container image URI (e.g., host/repo:tag or host/repo@digest)", manifest.ImageURL)
	}
	if !isNonEmpty(manifest.Command) {
		return fmt.Errorf("command is required")
	}
	if !isNonEmpty(manifest.Timeout) {
		return fmt.Errorf("timeout is required")
	}
	timeoutDuration, err := time.ParseDuration(manifest.Timeout)
	if err != nil {
		return fmt.Errorf("invalid timeout format '%s': %w", manifest.Timeout, err)
	}
	if timeoutDuration >= 24*time.Hour {
		return fmt.Errorf("timeout '%s' must be less than 24 hours", manifest.Timeout)
	}
	if timeoutDuration <= 0 {
		return fmt.Errorf("timeout '%s' must be a positive duration", manifest.Timeout)
	}
	if !isNonEmpty(manifest.ScaleConfig.LagThreshold) {
		return fmt.Errorf("scale_config.lag_threshold is required and must be a non-empty string")
	}
	if manifest.ScaleConfig.MinReplica < 0 {
		return fmt.Errorf("scale_config.min_replica (%d) cannot be negative", manifest.ScaleConfig.MinReplica)
	}
	if manifest.ScaleConfig.MaxReplica < manifest.ScaleConfig.MinReplica {
		return fmt.Errorf("scale_config.max_replica (%d) must be greater than or equal to min_replica (%d)", manifest.ScaleConfig.MaxReplica, manifest.ScaleConfig.MinReplica)
	}
	if manifest.Params == nil {
		return fmt.Errorf("params field is required (can be empty list [])")
	}
	if manifest.Configs == nil {
		return fmt.Errorf("configs field is required (can be empty list [])")
	}
	if manifest.RunSchedule == nil {
		return fmt.Errorf("run_schedule field is required")
	}
	if len(manifest.RunSchedule) < 1 {
		return fmt.Errorf("run_schedule must contain at least one entry")
	}

	defaultScheduleFound := false
	var defaultScheduleParams map[string]string
	paramSet := make(map[string]struct{})
	for _, p := range manifest.Params {
		paramSet[p] = struct{}{}
	}
	for i, schedule := range manifest.RunSchedule {
		entryDesc := fmt.Sprintf("run_schedule entry %d (id: '%s')", i, schedule.ID)
		if !isNonEmpty(schedule.ID) {
			return fmt.Errorf("%s: id is required", entryDesc)
		}
		if schedule.Params == nil {
			return fmt.Errorf("%s: params map is required", entryDesc)
		}
		if !isNonEmpty(schedule.Frequency) {
			return fmt.Errorf("%s: frequency is required", entryDesc)
		}
		if schedule.ID == "describe-all" || schedule.ID == "default" {
			defaultScheduleFound = true
			defaultScheduleParams = schedule.Params
			for requiredParam := range paramSet {
				if _, ok := defaultScheduleParams[requiredParam]; !ok {
					return fmt.Errorf("default schedule (id: '%s') is missing required parameter '%s' defined in top-level params", schedule.ID, requiredParam)
				}
			}
		}
	}
	if !defaultScheduleFound && len(manifest.Params) > 0 {
		return fmt.Errorf("at least one run_schedule entry must have id 'describe-all' or 'default' to cover top-level params")
	}
	return nil
}

// checkPlatformSupport checks if the plugin manifest supports a given platform version.
// Renamed from CheckPlatformSupport to be internal.
func (v *defaultValidator) checkPlatformSupport(manifest *PluginManifest, platformVersion string) (bool, error) {
	if manifest == nil {
		return false, fmt.Errorf("plugin manifest cannot be nil")
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

// validatePluginArtifacts handles the download and validation logic for 'plugin' type artifacts.
// Renamed from ValidateArtifact to be internal and specific to plugins.
func (v *defaultValidator) validatePluginArtifacts(manifest *PluginManifest, artifactType string) error {
	if manifest == nil {
		return fmt.Errorf("plugin manifest cannot be nil for artifact validation")
	}
	normalizedType := strings.ToLower(artifactType)
	if !isNonEmpty(artifactType) {
		normalizedType = ArtifactTypeAll
	}
	logMsgType := normalizedType
	log.Printf("--- Starting Plugin Artifact Validation (Type: %s) ---", logMsgType)

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
		discoveryErr = v.validateImageManifestExists(manifest.Plugin.Components.Discovery.ImageURI)
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
	wg.Wait()

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
		combinedErrors = append(combinedErrors, fmt.Errorf("platform-binary artifact validation failed: %w", platformErr).Error())
	}
	if cloudqlErr != nil && !(platformComp.URI == cloudqlComp.URI && platformErr != nil) {
		combinedErrors = append(combinedErrors, fmt.Errorf("cloudql-binary artifact validation failed: %w", cloudqlErr).Error())
	}
	if len(combinedErrors) > 0 {
		return errors.New(strings.Join(combinedErrors, "; "))
	}

	// log.Println("--- All requested plugin artifact validations successful ---") // Let caller log overall success
	return nil
}

// validateImageManifestExists checks if an image manifest exists in the registry using retries.
func (v *defaultValidator) validateImageManifestExists(imageURI string) error {
	if !isNonEmpty(imageURI) {
		return fmt.Errorf("image URI is empty")
	}
	// Digest format check is done in Validate(Plugin)ManifestStructure
	// if !imageDigestRegex.MatchString(imageURI) { return fmt.Errorf("image URI ('%s') must be in digest format", imageURI) }

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
		}
		fullRepo := fmt.Sprintf("%s/%s", ref.Host(), ref.Repository) // Combine host and repo path
		repo, err := remote.NewRepository(fullRepo)
		if err != nil {
			lastErr = fmt.Errorf("attempt %d: failed create repository client for '%s': %w", attempt+1, fullRepo, err)
			continue
		}
		repo.Client = httpClient // Use global client directly for default/anonymous auth

		// log.Printf("[DEBUG] Attempting to resolve manifest using ORAS default client for host: %s, repository: %s", repo.Reference.Registry, repo.Reference.Repository)
		_, err = repo.Resolve(ctx, ref.Reference) // ref.Reference is the digest

		if err == nil { /* log.Printf("Successfully resolved image manifest for %s.", imageURI); */
			return nil
		} // Success (reduced logging)

		lastErr = fmt.Errorf("attempt %d: failed resolve image manifest for '%s': %w", attempt+1, imageURI, err)
		log.Printf("Error details: %v", err)
		var errResp *errcode.ErrorResponse
		if errors.As(err, &errResp) {
			if errResp.StatusCode >= 400 && errResp.StatusCode < 500 {
				log.Printf("Attempt %d: Received client error %d (%s), not retrying.", attempt+1, errResp.StatusCode, http.StatusText(errResp.StatusCode))
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
	// log.Printf("--- Validating Downloadable Component: %s ---", componentName) // Reduced logging
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
			if errors.Is(err, context.DeadlineExceeded) {
				log.Printf("Attempt %d: Timeout", attempt+1)
			}
			continue
		} // Use errors.Is
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
		closeErr := resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("attempt %d: read body failed: %w", attempt+1, err)
			continue
		}
		if closeErr != nil {
			log.Printf("Warning: error closing response body for %s: %v", url, closeErr)
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
