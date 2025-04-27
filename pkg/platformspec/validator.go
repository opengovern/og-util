// Package platformspec provides utilities for loading, validating, and verifying
// various specification types (plugin, task, query, control, etc.).
package platformspec

import (
	"fmt"
	"log"
	"math/rand" // Needed for init
	"net/http"  // Needed for init
	"os"
	"regexp" // Needed for init
	"strings"
	"time" // Needed for init

	// Import other necessary packages used only by functions implemented *in this file*
	"gopkg.in/yaml.v3"
	// NOTE: Do not import packages solely used by implementations in other files
	// e.g., remove "github.com/Masterminds/semver/v3" if CheckPlatformSupport is not implemented here.
)

// --- Configuration Constants ---
const (
	// Standard Specification Types
	SpecTypePlugin  = "plugin"
	SpecTypeTask    = "task"
	SpecTypeQuery   = "query"   // Example future type
	SpecTypeControl = "control" // Example future type

	// Standard API Version
	APIVersionV1 = "v1"

	// Date format for PublishedDate (used in metadata_validation.go)
	PublishedDateFormat = "2006-01-02" // Go's reference date format

	// Constants for artifact validation types used in ProcessSpecification
	ArtifactTypeDiscovery      = "discovery"       // Validate only the discovery task's image.
	ArtifactTypePlatformBinary = "platform-binary" // Validate only the platform binary artifact.
	ArtifactTypeCloudQLBinary  = "cloudql-binary"  // Validate only the CloudQL binary artifact.
	ArtifactTypeAll            = "all"             // Validate all artifacts (default).

	// Output Formats for GetEmbeddedTaskSpecification
	FormatYAML = "yaml"
	FormatJSON = "json"
)

// --- Global Resources (Initialized in init) ---
// Shared HTTP client optimized for potentially frequent requests to registries and artifact servers.
var httpClient *http.Client

// Regex to validate that an image URL uses the digest format (e.g., image@sha256:...).
var imageDigestRegex *regexp.Regexp

// init initializes package-level resources.
// Assumes initializeHTTPClient and initializeSPDX are defined (e.g., in common.go, metadata_validation.go)
func init() {
	// Seed random number generator (used in artifact_validation.go)
	rand.Seed(time.Now().UnixNano())
	// Initialize HTTP client (assumes definition in http_client.go or common.go)
	initializeHTTPClient()
	// Compile regex (used in task_spec.go and artifact_validation.go)
	imageDigestRegex = regexp.MustCompile(`^.+@sha256:[a-fA-F0-9]{64}$`)
	// Initialize SPDX (assumes definition in metadata_validation.go or common.go)
	initializeSPDX()

	log.Println("Platform specification validator package initialized.")
}

// --- Interface Definition ---

// Validator defines the interface for processing, validating, and retrieving information from specifications.
// It promotes a "one call" pattern: call ProcessSpecification once, then use the returned validated
// specification object (*PluginSpecification, *TaskSpecification, etc.) with other functions.
type Validator interface {
	// ProcessSpecification is the primary entry point. It reads a specification file, determines its type,
	// performs full structural validation according to that type (including specific metadata, date, SPDX license checks where applicable),
	// checks platform compatibility (for plugins), and optionally validates artifacts based on the flags.
	// On success, it returns the fully parsed and validated specification struct (e.g., *PluginSpecification, *TaskSpecification,
	// *QuerySpecification - type-assert the returned interface{} to access specific fields) and a nil error.
	// Call this function ONCE per specification file.
	//
	// Parameters:
	//   filePath: Path to the specification YAML file.
	//   platformVersion: The current platform version string (e.g., "1.5.2") to check compatibility against (only for plugin specifications). Leave empty to skip check.
	//   artifactValidationType: Specifies which artifacts to validate ("discovery", "platform-binary", "cloudql-binary", "all"). Default is "all". Applies only to plugin specs.
	//   skipArtifactValidation: If true, completely skips all artifact/image download and validation checks. Applies only to plugin and standalone task specs.
	//
	// Returns:
	//   interface{}: A pointer to the specific validated specification struct (e.g., *PluginSpecification) if validation succeeds. Use type assertion.
	//   error: An error if reading, parsing, or validation fails.
	ProcessSpecification(filePath string, platformVersion string, artifactValidationType string, skipArtifactValidation bool) (interface{}, error)

	// GetTaskDefinition reads a specification file specifically expecting a *standalone* 'task' type,
	// parses it, validates its structure (including metadata), and returns the TaskSpecification struct or an error.
	// Consider using ProcessSpecification instead for a unified approach.
	GetTaskDefinition(filePath string) (*TaskSpecification, error)

	// GetTaskDetailsFromPluginSpecification extracts the details of the embedded 'discovery' task from an *already validated* PluginSpecification.
	// It includes inherited fields (APIVersion, SupportedPlatformVersions, Metadata) from the parent plugin for complete context.
	// It performs an additional validation step to ensure the task's image exists in the registry.
	// Use this function *after* successfully calling ProcessSpecification for a plugin.
	//
	// Parameters:
	//   pluginSpec: A pointer to a validated PluginSpecification struct (obtained from ProcessSpecification).
	//
	// Returns:
	//   *TaskDetails: A struct containing details of the discovery task, including inherited fields and its validated image URI.
	//   error: An error if the input specification is nil, if the discovery component used a task-id reference, or if the image existence check fails.
	GetTaskDetailsFromPluginSpecification(pluginSpec *PluginSpecification) (*TaskDetails, error)

	// CheckPlatformSupport checks if a given PluginSpecification supports a specific platform version string
	// based on the specification's `supported-platform-versions` constraints.
	// Use this function *after* successfully calling ProcessSpecification for a plugin.
	//
	// Parameters:
	//   pluginSpec: A pointer to a validated PluginSpecification struct (obtained from ProcessSpecification).
	//   platformVersion: The platform version string to check (e.g., "1.5.2").
	//
	// Returns:
	//   bool: True if the platform version is supported, false otherwise.
	//   error: An error if the specification is nil, platformVersion is empty, or if version/constraint parsing fails.
	CheckPlatformSupport(pluginSpec *PluginSpecification, platformVersion string) (bool, error)

	// IdentifySpecificationTypes reads a specification file and quickly identifies the primary type
	// and counts known embedded types (like the discovery task in a plugin) without full validation.
	//
	// Parameters:
	//   filePath: Path to the specification YAML file.
	//
	// Returns:
	//   *SpecificationTypeInfo: A struct containing the primary type and a map of embedded type counts.
	//   error: An error if reading or basic parsing fails.
	IdentifySpecificationTypes(filePath string) (*SpecificationTypeInfo, error)

	// GetEmbeddedTaskSpecification generates a standalone TaskSpecification representation (YAML or JSON string)
	// from the embedded discovery task within a validated PluginSpecification.
	// It includes inherited metadata and platform support details.
	// Returns an error if the plugin specification uses a task-id reference instead of embedding the task spec.
	// Use this function *after* successfully calling ProcessSpecification for a plugin.
	//
	// Parameters:
	//   pluginSpec: A pointer to a validated PluginSpecification struct (obtained from ProcessSpecification).
	//   format: The desired output format ("yaml" or "json"). Defaults to "yaml" if empty or invalid.
	//
	// Returns:
	//   string: The generated specification string in the requested format.
	//   error: An error if the input specification is nil, uses task-id, or marshaling fails.
	GetEmbeddedTaskSpecification(pluginSpec *PluginSpecification, format string) (string, error)

	// NOTE: GetFlattenedTags is NO LONGER part of the interface.
	// Use the package-level function GetFlattenedTags(spec interface{}) instead.
}

// --- Type Identification ---

// SpecificationTypeInfo holds the results of type identification.
type SpecificationTypeInfo struct {
	PrimaryType   string
	EmbeddedTypes map[string]int // Key: type name (e.g., "task"), Value: count
}

// Minimal struct to check for embedded discovery task in plugins (flattened structure).
// Checks for the presence of the task-spec key.
type pluginDiscoveryCheck struct {
	Components struct {
		Discovery struct {
			// We only need to know if task-spec exists, value can be anything here.
			TaskSpec interface{} `yaml:"task-spec"`
		} `yaml:"discovery"`
	} `yaml:"components"`
}

// IdentifySpecificationTypes reads a specification file and quickly identifies the primary type
// and counts known embedded types (like the discovery task in a plugin) without full validation.
// Assumes isNonEmpty is defined elsewhere (e.g., common.go)
func (v *defaultValidator) IdentifySpecificationTypes(filePath string) (*SpecificationTypeInfo, error) {
	log.Printf("Identifying specification types in file: %s", filePath)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file '%s' for type identification: %w", filePath, err)
	}

	// 1. Base Parse for Primary Type
	var base BaseSpecification
	if err := yaml.Unmarshal(data, &base); err != nil {
		return nil, fmt.Errorf("failed to parse base specification fields (type) from '%s': %w", filePath, err)
	}

	if !isNonEmpty(base.Type) {
		return nil, fmt.Errorf("specification file '%s' is missing required top-level 'type' field", filePath)
	}
	primaryType := strings.ToLower(base.Type)
	log.Printf("Identified primary type: '%s'", primaryType)

	// Initialize result
	info := &SpecificationTypeInfo{
		PrimaryType:   primaryType,
		EmbeddedTypes: make(map[string]int),
	}

	// 2. Check for Known Embeddings based on Primary Type
	switch primaryType {
	case SpecTypePlugin:
		var pluginCheck pluginDiscoveryCheck
		if err := yaml.Unmarshal(data, &pluginCheck); err == nil {
			if pluginCheck.Components.Discovery.TaskSpec != nil {
				log.Printf("Found embedded 'discovery' component (type: %s)", SpecTypeTask)
				info.EmbeddedTypes[SpecTypeTask] = 1
			} else {
				log.Printf("Plugin discovery component does not contain an embedded 'task-spec'. It might be a 'task-id' reference.")
			}
		} else {
			log.Printf("Warning: Could not perform minimal parse for embedded discovery check in '%s': %v", filePath, err)
		}
	default:
		log.Printf("No known embedded types defined for primary type '%s'.", primaryType)
	}

	return info, nil
}

// --- Concrete Implementation ---

// defaultValidator implements the Validator interface using the defined structs and helper methods.
type defaultValidator struct{}

// NewDefaultValidator creates a new instance of the default validator.
func NewDefaultValidator() Validator {
	return &defaultValidator{}
}

// --- Interface Method Implementations ---

// ProcessSpecification reads, identifies, validates structure (incl. date/SPDX license), checks platform (if applicable),
// and validates artifacts (if applicable and requested). This is the main entry point.
// Assumes isNonEmpty and type-specific process*Spec methods are defined elsewhere.
func (v *defaultValidator) ProcessSpecification(filePath string, platformVersion string, artifactValidationType string, skipArtifactValidation bool) (interface{}, error) {
	log.Printf("Processing specification file: %s (Platform Version: %s, Artifact Validation: %s, Skip Artifacts: %t)",
		filePath, platformVersion, artifactValidationType, skipArtifactValidation)

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file '%s': %w", filePath, err)
	}

	// 1. Determine Base Type, ID, and API Version
	var base BaseSpecification
	if err := yaml.Unmarshal(data, &base); err != nil {
		return nil, fmt.Errorf("failed to parse base specification fields (type, id, api-version) from '%s': %w", filePath, err)
	}

	// Check required Type field
	if !isNonEmpty(base.Type) {
		return nil, fmt.Errorf("specification file '%s' is missing required top-level 'type' field", filePath)
	}
	specType := strings.ToLower(base.Type)
	log.Printf("Detected specification type: '%s'", specType)

	// Handle API Version Defaulting (defaults to v1 for non-plugin types if missing)
	originalAPIVersion := base.APIVersion
	defaultedAPIVersion := base.APIVersion
	if !isNonEmpty(base.APIVersion) {
		if specType != SpecTypePlugin {
			log.Printf("Info: Specification '%s' (type: %s) missing 'api-version', defaulting to '%s'.", filePath, specType, APIVersionV1)
			defaultedAPIVersion = APIVersionV1
		} else {
			// Plugins MUST specify api-version: v1 explicitly
			return nil, fmt.Errorf("plugin specification '%s' is missing required top-level 'api-version' field (must be '%s')", filePath, APIVersionV1)
		}
	}

	// 2. Process based on Type - dispatch to type-specific processors
	// These methods are defined on defaultValidator but implemented in other files
	switch specType {
	case SpecTypePlugin:
		// Calls processPluginSpec defined in plugin_spec.go
		return v.processPluginSpec(data, filePath, platformVersion, artifactValidationType, skipArtifactValidation)

	case SpecTypeTask:
		// Calls processTaskSpec defined in task_spec.go
		return v.processTaskSpec(data, filePath, skipArtifactValidation, defaultedAPIVersion, originalAPIVersion)

	case SpecTypeQuery:
		// Calls processQuerySpec defined in query_spec.go
		return v.processQuerySpec(data, filePath, defaultedAPIVersion, originalAPIVersion)

	case SpecTypeControl: // Example future type
		var spec ControlSpecification
		if err := yaml.Unmarshal(data, &spec); err != nil {
			return nil, fmt.Errorf("failed to parse specification file '%s' as control: %w", filePath, err)
		}
		if !isNonEmpty(spec.APIVersion) {
			spec.APIVersion = defaultedAPIVersion
		}
		spec.Type = specType
		if spec.APIVersion != APIVersionV1 {
			return nil, fmt.Errorf("control specification '%s': api-version must be '%s' (or omitted to default), got '%s'", filePath, APIVersionV1, originalAPIVersion)
		}
		if !isNonEmpty(spec.ID) {
			return nil, fmt.Errorf("control specification '%s': id is required", filePath)
		}
		log.Println("Validating control specification structure...")
		// TODO: Implement v.validateControlStructure(&spec) in control_spec.go later
		log.Println("Control specification structure validation successful (Placeholder).")
		// TODO: Add artifact validation if needed for controls later
		return &spec, nil

	default:
		return nil, fmt.Errorf("unknown or unsupported specification type '%s' in file '%s'", base.Type, filePath)
	}
}
