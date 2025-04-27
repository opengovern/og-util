package platformspec

import (
	"encoding/json" // Added for JSON marshaling
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/Masterminds/semver/v3"
	"gopkg.in/yaml.v3"
)

// processPluginSpec handles the parsing and validation specific to plugin specifications.
// It's called by ProcessSpecification in validator.go.
func (v *defaultValidator) processPluginSpec(data []byte, filePath string, platformVersion string, artifactValidationType string, skipArtifactValidation bool) (*PluginSpecification, error) {
	var spec PluginSpecification
	// Unmarshal directly into the flattened PluginSpecification struct
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("failed to parse specification file '%s' as plugin: %w", filePath, err)
	}

	// --- Core Validation ---
	// Plugins MUST explicitly specify api-version: v1
	if !isNonEmpty(spec.APIVersion) || spec.APIVersion != APIVersionV1 {
		return nil, fmt.Errorf("plugin specification '%s': api-version is required and must be '%s', got '%s'", filePath, APIVersionV1, spec.APIVersion)
	}
	// Type should already be checked by caller (ProcessSpecification), but double-check
	if !isNonEmpty(spec.Type) || spec.Type != SpecTypePlugin {
		return nil, fmt.Errorf("plugin specification '%s': type is required and must be '%s', got '%s'", filePath, SpecTypePlugin, spec.Type)
	}

	log.Println("Validating plugin specification structure...")
	// Defaulting for embedded task ID/Type/Name/Description happens inside validatePluginStructure
	if err := v.validatePluginStructure(&spec); err != nil {
		return nil, fmt.Errorf("plugin specification structure validation failed: %w", err)
	}
	log.Println("Plugin specification structure validation successful.")

	// --- Optional Checks ---
	// Platform Support Check
	if isNonEmpty(platformVersion) {
		log.Printf("Checking platform support for version: %s", platformVersion)
		supported, supportErr := v.CheckPlatformSupport(&spec, platformVersion)
		if supportErr != nil {
			log.Printf("Warning: Error checking platform support for plugin '%s': %v", spec.Name, supportErr) // Use spec.Name
		} else {
			status := "IS NOT"
			if supported {
				status = "IS"
			}
			log.Printf("Platform version %s %s supported by plugin '%s' version '%s'.", platformVersion, status, spec.Name, spec.Version) // Use spec.Name, spec.Version
		}
	} else {
		log.Println("Skipping platform support check (no platform version provided).")
	}

	// Artifact Validation
	if !skipArtifactValidation {
		log.Println("Starting plugin artifact validation...")
		// Assumes validatePluginArtifacts is defined elsewhere (e.g., artifact_validation.go)
		if err := v.validatePluginArtifacts(&spec, artifactValidationType); err != nil {
			return nil, fmt.Errorf("plugin artifact validation failed: %w", err)
		}
		log.Println("Plugin artifact validation successful.")
	} else {
		log.Println("Skipping plugin artifact validation as requested.")
	}

	return &spec, nil
}

// validatePluginStructure performs structural checks specific to 'plugin' specifications,
// including metadata (date, license) and the embedded discovery task. It also handles
// defaulting for the embedded task's ID, Type, Name, and Description.
func (v *defaultValidator) validatePluginStructure(spec *PluginSpecification) error {
	if spec == nil {
		return errors.New("plugin specification cannot be nil")
	}
	// API Version and Type already validated by processPluginSpec

	// --- Top-Level Plugin Fields ---
	if !isNonEmpty(spec.Name) {
		return errors.New("plugin name is required at the top level")
	}
	if !isNonEmpty(spec.Version) {
		return errors.New("plugin version is required at the top level")
	}
	if _, err := semver.NewVersion(spec.Version); err != nil {
		return fmt.Errorf("plugin version: invalid semantic version format '%s': %w", spec.Version, err)
	}
	if len(spec.SupportedPlatformVersions) == 0 {
		return errors.New("plugin supported-platform-versions requires at least one constraint entry at the top level")
	}
	for i, constraintStr := range spec.SupportedPlatformVersions {
		if !isNonEmpty(constraintStr) {
			return fmt.Errorf("plugin supported-platform-versions entry %d: cannot be empty", i)
		}
		if _, err := semver.NewConstraint(constraintStr); err != nil {
			return fmt.Errorf("plugin supported-platform-versions entry %d ('%s'): is not a valid semantic version constraint: %w", i, constraintStr, err)
		}
	}

	// --- Metadata Block Fields ---
	// Use the helper function for metadata validation
	// Assumes validateMetadata exists in metadata_validation.go
	if err := v.validateMetadata(&spec.Metadata, fmt.Sprintf("plugin '%s' metadata", spec.Name)); err != nil {
		return err // Error from validateMetadata is already contextualized
	}

	// --- Components Block Fields ---
	components := &spec.Components // Use pointer for modification

	// *** Validate Embedded Discovery Task Structure ***
	// Pass 'false' for isStandalone. Metadata and APIVersion checks happen inside.
	// ID, Name, Description, Type are optional here.
	// Assumes validateTaskStructure exists in task_spec.go
	if err := v.validateTaskStructure(&components.Discovery, false); err != nil { // false = embedded
		return fmt.Errorf("plugin components.discovery task validation failed: %w", err)
	}

	// *** Default ID, Type, Name, Description for Embedded Discovery Task ***
	discoveryTask := &components.Discovery    // Use pointer to modify
	defaultSuffix := "-task"                  // Suffix for default values
	defaultID := spec.Name + defaultSuffix    // Use top-level Name
	defaultName := spec.Name + defaultSuffix  // Use top-level Name
	defaultDescription := spec.Name + " Task" // Use top-level Name

	if !isNonEmpty(discoveryTask.ID) {
		log.Printf("Info: Embedded discovery task for plugin '%s' is missing 'id'. Defaulting to '%s'.", spec.Name, defaultID)
		discoveryTask.ID = defaultID
	}
	if !isNonEmpty(discoveryTask.Name) {
		log.Printf("Info: Embedded discovery task for plugin '%s' (ID: %s) is missing 'name'. Defaulting to '%s'.", spec.Name, discoveryTask.ID, defaultName)
		discoveryTask.Name = defaultName
	}
	if !isNonEmpty(discoveryTask.Description) {
		log.Printf("Info: Embedded discovery task for plugin '%s' (ID: %s) is missing 'description'. Defaulting to '%s'.", spec.Name, discoveryTask.ID, defaultDescription)
		discoveryTask.Description = defaultDescription
	}
	if !isNonEmpty(discoveryTask.Type) {
		log.Printf("Info: Embedded discovery task for plugin '%s' (ID: %s) is missing 'type'. Defaulting to '%s'.", spec.Name, discoveryTask.ID, SpecTypeTask)
		discoveryTask.Type = SpecTypeTask // Default Type to "task"
	}
	// Post-defaulting check: Ensure type is indeed "task" (covers case where it was specified incorrectly)
	if discoveryTask.Type != SpecTypeTask {
		return fmt.Errorf("plugin components.discovery task (ID: %s): type must be '%s' if specified, got '%s'", discoveryTask.ID, SpecTypeTask, discoveryTask.Type)
	}

	// *** Validate Downloadable Component References ***
	platformComp := components.PlatformBinary
	cloudqlComp := components.CloudQLBinary

	if !isNonEmpty(platformComp.URI) {
		return errors.New("plugin components.platform-binary.uri is required")
	}
	if !isNonEmpty(cloudqlComp.URI) {
		return errors.New("plugin components.cloudql-binary.uri is required")
	}

	// If URIs are the same, both components MUST specify a PathInArchive.
	if platformComp.URI == cloudqlComp.URI {
		log.Printf("Info: PlatformBinary and CloudQLBinary share the same URI: %s. PathInArchive is required for both.", platformComp.URI)
		if !isNonEmpty(platformComp.PathInArchive) {
			return fmt.Errorf("plugin components.platform-binary.path-in-archive is required when its URI ('%s') matches cloudql-binary.uri", platformComp.URI)
		}
		if !isNonEmpty(cloudqlComp.PathInArchive) {
			return fmt.Errorf("plugin components.cloudql-binary.path-in-archive is required when its URI ('%s') matches platform-binary.uri", cloudqlComp.URI)
		}
		if platformComp.PathInArchive == cloudqlComp.PathInArchive {
			return fmt.Errorf("plugin components.platform-binary.path-in-archive ('%s') cannot be the same as cloudql-binary.path-in-archive ('%s') when URIs match", platformComp.PathInArchive, cloudqlComp.PathInArchive)
		}
	}

	// --- Sample Data (Optional) ---
	if spec.SampleData != nil { // Access SampleData directly from spec
		if !isNonEmpty(spec.SampleData.URI) {
			return errors.New("plugin sample-data.uri is required when the sample-data section is present")
		}
	}

	return nil
}

// GetTaskDetailsFromPluginSpecification extracts discovery task details from an already validated PluginSpecification.
// It includes inherited fields and performs an additional image existence check.
func (v *defaultValidator) GetTaskDetailsFromPluginSpecification(pluginSpec *PluginSpecification) (*TaskDetails, error) {
	if pluginSpec == nil {
		return nil, errors.New("input PluginSpecification cannot be nil")
	}
	// Assume pluginSpec is already structurally validated and defaulted by ProcessSpecification

	log.Printf("Getting task details from pre-validated plugin specification: %s (Version: %s)", pluginSpec.Name, pluginSpec.Version) // Use spec.Name, spec.Version

	// 1. Access the embedded discovery task (ID, Name, Description, Type are guaranteed to be set by ProcessSpecification/validatePluginStructure)
	discoveryTask := pluginSpec.Components.Discovery // Access components directly

	// 2. Validate the Image URL existence (Format check was done during ProcessSpecification)
	log.Printf("Validating image existence for discovery task (ID: %s, Image: %s)...", discoveryTask.ID, discoveryTask.ImageURL)
	// Assumes validateImageManifestExists exists in artifact_validation.go
	if err := v.validateImageManifestExists(discoveryTask.ImageURL); err != nil {
		// Wrap error for better context
		return nil, fmt.Errorf("discovery task image URI existence check failed for '%s' (plugin: %s): %w",
			discoveryTask.ImageURL, pluginSpec.Name, err) // Use spec.Name
	}
	log.Printf("Image existence validated successfully for: %s", discoveryTask.ImageURL)

	// 3. Populate the TaskDetails struct, including inherited fields
	details := &TaskDetails{
		// Task-specific fields
		TaskID:            discoveryTask.ID,          // Includes defaults if applied
		TaskName:          discoveryTask.Name,        // Includes defaults if applied
		TaskDescription:   discoveryTask.Description, // Includes defaults if applied
		ValidatedImageURI: discoveryTask.ImageURL,    // Use the validated URL
		Command:           discoveryTask.Command,     // Copy the command slice
		Timeout:           discoveryTask.Timeout,
		ScaleConfig:       discoveryTask.ScaleConfig,
		Params:            discoveryTask.Params,
		Configs:           discoveryTask.Configs,
		RunSchedule:       discoveryTask.RunSchedule,

		// Inherited fields from PluginSpecification
		PluginName:                pluginSpec.Name, // Use spec.Name
		APIVersion:                pluginSpec.APIVersion,
		SupportedPlatformVersions: pluginSpec.SupportedPlatformVersions, // Copy slice from spec
		Metadata:                  pluginSpec.Metadata,                  // Copy struct from spec
	}

	log.Printf("Successfully retrieved and validated task details for task ID '%s' from plugin '%s'", details.TaskID, details.PluginName)
	return details, nil
}

// validatePluginArtifacts handles the download and validation logic for 'plugin' type artifacts
// based on the requested artifactType ("all", "discovery", "platform-binary", "cloudql-binary").
// Assumes dependent functions (validateImageManifestExists, validateSingleDownloadableComponent, etc.)
// are defined in artifact_validation.go
func (v *defaultValidator) validatePluginArtifacts(spec *PluginSpecification, artifactType string) error {
	if spec == nil {
		return errors.New("plugin specification cannot be nil for artifact validation")
	}

	// Normalize and determine which artifacts to validate
	normalizedType := strings.ToLower(strings.TrimSpace(artifactType))
	if !isNonEmpty(normalizedType) {
		normalizedType = ArtifactTypeAll // Use exported constant
	}
	log.Printf("--- Starting Plugin Artifact Validation (Requested Type: %s) ---", normalizedType)

	validateDiscovery := false
	validatePlatform := false
	validateCloudQL := false

	switch normalizedType {
	case ArtifactTypeAll: // Use exported constant
		validateDiscovery = true
		validatePlatform = true
		validateCloudQL = true
		log.Println("Scope: Validating Discovery Image, PlatformBinary, and CloudQLBinary artifacts.")
	case ArtifactTypeDiscovery: // Use exported constant
		validateDiscovery = true
		log.Println("Scope: Validating only Discovery Image artifact.")
	case ArtifactTypePlatformBinary: // Use exported constant
		validatePlatform = true
		log.Println("Scope: Validating only PlatformBinary artifact.")
	case ArtifactTypeCloudQLBinary: // Use exported constant
		validateCloudQL = true
		log.Println("Scope: Validating only CloudQLBinary artifact.")
	default:
		// Use exported constants in error message
		return fmt.Errorf("invalid artifactType '%s'. Must be one of: '%s', '%s', '%s', or '%s' (or empty)",
			artifactType, ArtifactTypeDiscovery, ArtifactTypePlatformBinary, ArtifactTypeCloudQLBinary, ArtifactTypeAll)
	}

	var wg sync.WaitGroup
	errChan := make(chan error, 3) // Buffered channel to collect errors from goroutines
	var platformData []byte        // To store downloaded data for shared URI case

	// Access components directly from spec
	platformComp := spec.Components.PlatformBinary
	cloudqlComp := spec.Components.CloudQLBinary
	// Use ImageURL from embedded task (ID/Type/Name/Desc are guaranteed to be set by validatePluginStructure)
	discoveryImageURL := spec.Components.Discovery.ImageURL

	// --- Validate Discovery Task Image ---
	if validateDiscovery {
		log.Printf("Validating Discovery Image: %s", discoveryImageURL)
		// Assumes validateImageManifestExists is defined elsewhere (e.g., artifact_validation.go)
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
			// Use exported constant for component name
			// Assumes validateSingleDownloadableComponent is defined elsewhere (e.g., artifact_validation.go)
			platformData, err = v.validateSingleDownloadableComponent(comp, ArtifactTypePlatformBinary) // Retries handled inside
			if err != nil {
				log.Printf("Error validating PlatformBinary artifact '%s': %v", comp.URI, err)
				errChan <- fmt.Errorf("platform-binary artifact validation failed for URI '%s': %w", comp.URI, err)
				platformData = nil // Ensure nil on error
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
			// Use exported constant for component name
			// Assumes validateSingleDownloadableComponent is defined elsewhere (e.g., artifact_validation.go)
			_, err := v.validateSingleDownloadableComponent(comp, ArtifactTypeCloudQLBinary) // Retries handled inside
			if err != nil {
				log.Printf("Error validating CloudQLBinary artifact (separate URI) '%s': %v", comp.URI, err)
				errChan <- fmt.Errorf("cloudql-binary artifact validation failed for URI '%s': %w", comp.URI, err)
			} else {
				log.Printf("CloudQLBinary artifact validation successful (separate URI): %s", comp.URI)
			}
		}(cloudqlComp)
	}

	// Wait for concurrent validations
	wg.Wait()

	// --- Validate CloudQL Binary (Shared URI Case) ---
	if validateCloudQL && platformComp.URI == cloudqlComp.URI {
		log.Printf("Validating CloudQLBinary artifact (shared URI %s, path %s)...", cloudqlComp.URI, cloudqlComp.PathInArchive)
		if validatePlatform {
			if platformData == nil {
				errMsg := fmt.Errorf("cannot validate cloudql-binary path '%s' because shared archive download/validation failed for URI '%s'",
					cloudqlComp.PathInArchive, cloudqlComp.URI)
				log.Printf("Error validating CloudQLBinary artifact (shared URI): %v", errMsg)
				// Error already sent by the platform binary goroutine
			} else {
				log.Printf("Checking path '%s' within shared archive data from %s...", cloudqlComp.PathInArchive, cloudqlComp.URI)
				// Assumes validateArchivePathExists exists in artifact_validation.go
				err := v.validateArchivePathExists(platformData, cloudqlComp.PathInArchive, cloudqlComp.URI)
				if err != nil {
					cloudqlErr := fmt.Errorf("cloudql-binary path validation failed: %w", err)
					log.Printf("Error validating CloudQLBinary artifact path (shared URI): %v", cloudqlErr)
					errChan <- cloudqlErr
				} else {
					log.Printf("CloudQLBinary artifact validation successful (shared URI path '%s' exists).", cloudqlComp.PathInArchive)
				}
			}
		} else {
			log.Printf("Warning: CloudQL validation requested for shared URI '%s', but PlatformBinary validation was skipped. Downloading artifact again for path check.", platformComp.URI)
			// Use exported constant for component name
			// Assumes validateSingleDownloadableComponent exists in artifact_validation.go
			sharedDataForCloudQL, downloadErr := v.validateSingleDownloadableComponent(platformComp, "shared archive for CloudQL path check")
			if downloadErr != nil {
				log.Printf("Error downloading shared archive '%s' for CloudQL path check: %v", platformComp.URI, downloadErr)
				errChan <- fmt.Errorf("failed to download shared archive '%s' for cloudql-binary path check: %w", platformComp.URI, downloadErr)
			} else if sharedDataForCloudQL != nil {
				log.Printf("Checking path '%s' within shared archive data from %s...", cloudqlComp.PathInArchive, cloudqlComp.URI)
				// Assumes validateArchivePathExists exists in artifact_validation.go
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

// GetEmbeddedTaskSpecification generates a standalone TaskSpecification representation (YAML or JSON string)
// from the embedded discovery task within a validated PluginSpecification.
// It includes inherited metadata and platform support details.
func (v *defaultValidator) GetEmbeddedTaskSpecification(pluginSpec *PluginSpecification, format string) (string, error) {
	if pluginSpec == nil {
		return "", errors.New("input PluginSpecification cannot be nil")
	}
	// Assumes pluginSpec is already validated and defaulted

	log.Printf("Generating standalone specification string (format: %s) for embedded task from plugin: %s", format, pluginSpec.Name)

	// 1. Access the embedded discovery task (already defaulted)
	embeddedTask := pluginSpec.Components.Discovery

	// 2. Construct the standalone TaskSpecification struct
	// Create copies of slices/maps/pointers to avoid modifying the original pluginSpec
	metadataCopy := pluginSpec.Metadata // Shallow copy is okay for Metadata struct itself
	supportedVersionsCopy := make([]string, len(pluginSpec.SupportedPlatformVersions))
	copy(supportedVersionsCopy, pluginSpec.SupportedPlatformVersions)
	// Create a pointer to the metadata copy for the standalone task struct
	metadataPtr := &metadataCopy

	standaloneTask := TaskSpecification{
		// Inherited fields that are REQUIRED for standalone tasks
		APIVersion:                pluginSpec.APIVersion, // Inherit API version
		Type:                      SpecTypeTask,          // Explicitly set type to task
		Metadata:                  metadataPtr,           // Use pointer to the copied metadata
		SupportedPlatformVersions: supportedVersionsCopy, // Use copied slice

		// Fields copied directly from the (defaulted) embedded task
		ID:          embeddedTask.ID,
		Name:        embeddedTask.Name,
		Description: embeddedTask.Description,
		IsEnabled:   embeddedTask.IsEnabled,
		ImageURL:    embeddedTask.ImageURL,
		Command:     embeddedTask.Command, // Copy the command slice
		Timeout:     embeddedTask.Timeout,
		ScaleConfig: embeddedTask.ScaleConfig, // Copy struct
		Params:      embeddedTask.Params,      // Copy slice
		Configs:     embeddedTask.Configs,     // Copy slice
		RunSchedule: embeddedTask.RunSchedule, // Copy slice
	}

	// 3. Marshal the new struct to the requested format
	var outputBytes []byte
	var err error
	outputFormat := strings.ToLower(strings.TrimSpace(format))

	if outputFormat == FormatJSON {
		// Use json.MarshalIndent for pretty-printed JSON output
		outputBytes, err = json.MarshalIndent(&standaloneTask, "", "  ") // Use 2 spaces for indentation
		if err != nil {
			return "", fmt.Errorf("failed to marshal standalone task specification to JSON: %w", err)
		}
		log.Printf("Successfully marshaled task spec to JSON.")
	} else {
		// Default to YAML
		if outputFormat != FormatYAML && format != "" {
			log.Printf("Warning: Invalid format '%s' requested, defaulting to '%s'.", format, FormatYAML)
		}
		outputBytes, err = yaml.Marshal(&standaloneTask)
		if err != nil {
			return "", fmt.Errorf("failed to marshal standalone task specification to YAML: %w", err)
		}
		log.Printf("Successfully marshaled task spec to YAML.")
	}

	return string(outputBytes), nil
}
