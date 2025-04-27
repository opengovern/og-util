// Package platformspec provides utilities for loading, validating, and verifying
// various specification types (plugin, task, query, control, etc.).
package platformspec

import (
	"errors" // Import for sentinel error
	"fmt"
	"log"
	"net/http" // Needed for init placeholder/actual
	"os"
	"regexp" // Needed for init
	"strings"

	// Needed for init
	"gopkg.in/yaml.v3"
	// NOTE: Do not import packages solely used by implementations in other files
	// e.g., remove "math/rand" if not used directly *in this file*.
	// e.g., remove "github.com/Masterminds/semver/v3" if CheckPlatformSupport is not implemented here.
)

// --- Configuration Constants ---
const (
	// Standard Specification Types
	SpecTypePlugin  = "plugin"
	SpecTypeTask    = "task"
	SpecTypeQuery   = "query"
	SpecTypeControl = "control"

	// Standard API Version
	APIVersionV1 = "v1"

	// Date format for PublishedDate (used in metadata_validation.go)
	PublishedDateFormat = "2006-01-02" // Go's reference date format

	// Constants for artifact validation types used in ProcessSpecification
	ArtifactTypeDiscovery      = "discovery"
	ArtifactTypePlatformBinary = "platform-binary"
	ArtifactTypeCloudQLBinary  = "cloudql-binary"
	ArtifactTypeAll            = "all"

	// Output Formats for GetEmbeddedTaskSpecification
	FormatYAML = "yaml"
	FormatJSON = "json"
)

// --- Exported Sentinel Error ---
var ErrMissingTypeField = errors.New("specification file is missing required top-level 'type' field")

// --- Global Resources (Initialized in init) ---
var httpClient *http.Client
var imageDigestRegex *regexp.Regexp

// init initializes package-level resources.
// Assumes initializeHTTPClient and initializeSPDX are defined elsewhere (e.g., common.go).
func init() {
	// rand.Seed() is deprecated and not needed for Go 1.20+ global rand
	initializeHTTPClient() // Assumes definition exists elsewhere
	imageDigestRegex = regexp.MustCompile(`^.+@sha256:[a-fA-F0-9]{64}$`)
	initializeSPDX() // Assumes definition exists elsewhere
	log.Println("Platform specification validator package initialized.")
}

// --- Interface Definition ---

// Validator defines the interface for processing, validating, and retrieving information from specifications.
type Validator interface {
	ProcessSpecification(filePath string, platformVersion string, artifactValidationType string, skipArtifactValidation bool) (interface{}, error)
	GetTaskDefinition(filePath string) (*TaskSpecification, error)
	GetTaskDetailsFromPluginSpecification(pluginSpec *PluginSpecification) (*TaskDetails, error)
	CheckPlatformSupport(pluginSpec *PluginSpecification, platformVersion string) (bool, error)
	IdentifySpecificationTypes(filePath string) (*SpecificationTypeInfo, error)
	GetEmbeddedTaskSpecification(pluginSpec *PluginSpecification, format string) (string, error)
}

// --- Type Identification ---

// SpecificationTypeInfo holds the results of type identification.
type SpecificationTypeInfo struct {
	PrimaryType   string
	EmbeddedTypes map[string]int // Key: type name (e.g., "task"), Value: count
}

// Minimal struct to check for embedded discovery task in plugins.
type pluginDiscoveryCheck struct {
	Components struct {
		Discovery struct {
			TaskSpec interface{} `yaml:"task-spec"`
		} `yaml:"discovery"`
	} `yaml:"components"`
}

// IdentifySpecificationTypes reads a specification file and quickly identifies the primary type.
// Returns ErrMissingTypeField if the 'type' field is missing.
// Assumes isNonEmpty is defined elsewhere (e.g., common.go).
func (v *defaultValidator) IdentifySpecificationTypes(filePath string) (*SpecificationTypeInfo, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file '%s' for type identification: %w", filePath, err)
	}

	var base BaseSpecification
	if err := yaml.Unmarshal(data, &base); err != nil {
		return nil, fmt.Errorf("failed to parse base specification fields from '%s': %w", filePath, err)
	}

	if !isNonEmpty(base.Type) {
		return nil, ErrMissingTypeField // Return specific error
	}
	primaryType := strings.ToLower(base.Type)

	info := &SpecificationTypeInfo{
		PrimaryType:   primaryType,
		EmbeddedTypes: make(map[string]int),
	}

	// Check for Known Embeddings
	switch primaryType {
	case SpecTypePlugin:
		var pluginCheck pluginDiscoveryCheck
		// Ignore unmarshal error here, only care if TaskSpec was present
		if yaml.Unmarshal(data, &pluginCheck) == nil && pluginCheck.Components.Discovery.TaskSpec != nil {
			// Consider logging only if verbose logging is enabled
			// log.Printf("Found embedded 'discovery' component (type: %s)", SpecTypeTask)
			info.EmbeddedTypes[SpecTypeTask] = 1
		}
	}
	return info, nil // Success
}

// --- Concrete Implementation ---

// defaultValidator implements the Validator interface.
type defaultValidator struct{}

// NewDefaultValidator creates a new instance of the default validator.
func NewDefaultValidator() Validator {
	return &defaultValidator{}
}

// --- Interface Method Implementations (Wrappers) ---

// ProcessSpecification reads, identifies, validates structure, checks platform, and validates artifacts.
// It dispatches to internal type-specific processor methods (process*Spec).
// Assumes isNonEmpty and process*Spec methods are defined elsewhere on *defaultValidator.
func (v *defaultValidator) ProcessSpecification(filePath string, platformVersion string, artifactValidationType string, skipArtifactValidation bool) (interface{}, error) {
	// log.Printf("Processing specification file: %s ...", filePath) // Optional: Keep logs minimal
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file '%s': %w", filePath, err)
	}

	var base BaseSpecification
	if err := yaml.Unmarshal(data, &base); err != nil {
		return nil, fmt.Errorf("failed to parse base fields from '%s': %w", filePath, err)
	}

	if !isNonEmpty(base.Type) {
		// This case should ideally be caught by IdentifySpecificationTypes first,
		// but return the specific error here too for robustness.
		return nil, ErrMissingTypeField
	}
	specType := strings.ToLower(base.Type)

	originalAPIVersion := base.APIVersion
	defaultedAPIVersion := base.APIVersion
	if !isNonEmpty(base.APIVersion) {
		if specType != SpecTypePlugin {
			defaultedAPIVersion = APIVersionV1
		} else {
			return nil, fmt.Errorf("plugin specification '%s' missing required 'api-version'", filePath)
		}
	}

	// Dispatch to specific processors implemented elsewhere
	switch specType {
	case SpecTypePlugin:
		return v.processPluginSpec(data, filePath, platformVersion, artifactValidationType, skipArtifactValidation)
	case SpecTypeTask:
		return v.processTaskSpec(data, filePath, skipArtifactValidation, defaultedAPIVersion, originalAPIVersion)
	case SpecTypeQuery:
		return v.processQuerySpec(data, filePath, defaultedAPIVersion, originalAPIVersion)
	case SpecTypeControl:
		// Example handling for a future type
		var spec ControlSpecification
		if err := yaml.Unmarshal(data, &spec); err != nil {
			return nil, fmt.Errorf("failed parse '%s' as control: %w", filePath, err)
		}
		if !isNonEmpty(spec.APIVersion) {
			spec.APIVersion = defaultedAPIVersion
		}
		spec.Type = specType
		if spec.APIVersion != APIVersionV1 {
			return nil, fmt.Errorf("control '%s': invalid api-version '%s'", filePath, originalAPIVersion)
		}
		if !isNonEmpty(spec.ID) {
			return nil, fmt.Errorf("control '%s': id is required", filePath)
		}
		// TODO: Add call to v.validateControlStructure(&spec) when implemented
		log.Printf("Control specification '%s' validated (Placeholder).", filePath)
		return &spec, nil
	default:
		return nil, fmt.Errorf("unknown specification type '%s' in file '%s'", base.Type, filePath)
	}
}

// GetTaskDetailsFromPluginSpecification implements the Validator interface by calling the internal logic.
// Assumes getTaskDetailsFromPluginSpecificationImpl is defined on *defaultValidator in plugin_spec.go.
func (v *defaultValidator) GetTaskDetailsFromPluginSpecification(pluginSpec *PluginSpecification) (*TaskDetails, error) {
	return v.getTaskDetailsFromPluginSpecificationImpl(pluginSpec)
}

// CheckPlatformSupport implements the Validator interface by calling the internal logic.
// Assumes checkPlatformSupportImpl is defined on *defaultValidator elsewhere (e.g., common.go).
func (v *defaultValidator) CheckPlatformSupport(pluginSpec *PluginSpecification, platformVersion string) (bool, error) {
	return v.checkPlatformSupportImpl(pluginSpec, platformVersion)
}

// GetEmbeddedTaskSpecification implements the Validator interface by calling the internal logic.
// Assumes getEmbeddedTaskSpecificationImpl is defined on *defaultValidator in plugin_spec.go.
func (v *defaultValidator) GetEmbeddedTaskSpecification(pluginSpec *PluginSpecification, format string) (string, error) {
	return v.getEmbeddedTaskSpecificationImpl(pluginSpec, format)
}

// --- Exported Helper Functions ---
