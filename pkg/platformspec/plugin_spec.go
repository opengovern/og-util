package platformspec

import (
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
	// Defaulting for embedded task ID/Type happens inside validatePluginStructure
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
			log.Printf("Warning: Error checking platform support for plugin '%s': %v", spec.Plugin.Name, supportErr)
		} else {
			status := "IS NOT"
			if supported {
				status = "IS"
			}
			log.Printf("Platform version %s %s supported by plugin '%s' version '%s'.", platformVersion, status, spec.Plugin.Name, spec.Plugin.Version)
		}
	} else {
		log.Println("Skipping platform support check (no platform version provided).")
	}

	// Artifact Validation
	if !skipArtifactValidation {
		log.Println("Starting plugin artifact validation...")
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
// defaulting for the embedded task's ID and Type.
func (v *defaultValidator) validatePluginStructure(spec *PluginSpecification) error {
	if spec == nil {
		return errors.New("plugin specification cannot be nil")
	}
	// API Version and Type already validated by processPluginSpec

	// --- Plugin Block Fields ---
	plugin := &spec.Plugin // Use pointer for potential modification of embedded task
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
	// Assumes validateMetadata exists in metadata_validation.go
	if err := v.validateMetadata(&plugin.Metadata, fmt.Sprintf("plugin '%s' metadata", plugin.Name)); err != nil {
		return err // Error from validateMetadata is already contextualized
	}

	// --- Components Block Fields ---
	components := &plugin.Components // Use pointer for modification

	// *** Validate Embedded Discovery Task Structure ***
	// Pass 'false' for isStandalone. Metadata and APIVersion checks happen inside.
	// ID and Type are now optional for embedded tasks within this function.
	// Assumes validateTaskStructure exists in task_spec.go
	if err := v.validateTaskStructure(&components.Discovery, false); err != nil { // false = embedded
		return fmt.Errorf("plugin.components.discovery task validation failed: %w", err)
	}

	// *** Default ID and Type for Embedded Discovery Task ***
	discoveryTask := &components.Discovery // Use pointer to modify
	if !isNonEmpty(discoveryTask.ID) {
		log.Printf("Info: Embedded discovery task for plugin '%s' is missing 'id'. Defaulting to plugin name.", plugin.Name)
		discoveryTask.ID = plugin.Name // Default ID to plugin name
	}
	if !isNonEmpty(discoveryTask.Type) {
		log.Printf("Info: Embedded discovery task for plugin '%s' (ID: %s) is missing 'type'. Defaulting to '%s'.", plugin.Name, discoveryTask.ID, SpecTypeTask)
		discoveryTask.Type = SpecTypeTask // Default Type to "task"
	}
	// Post-defaulting check: Ensure type is indeed "task" (covers case where it was specified incorrectly)
	if discoveryTask.Type != SpecTypeTask {
		return fmt.Errorf("plugin.components.discovery task (ID: %s): type must be '%s' if specified, got '%s'", discoveryTask.ID, SpecTypeTask, discoveryTask.Type)
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

// GetTaskDetailsFromPluginSpecification extracts discovery task details from an already validated PluginSpecification.
// It includes inherited fields and performs an additional image existence check.
func (v *defaultValidator) GetTaskDetailsFromPluginSpecification(pluginSpec *PluginSpecification) (*TaskDetails, error) {
	if pluginSpec == nil {
		return nil, errors.New("input PluginSpecification cannot be nil")
	}
	// Assume pluginSpec is already structurally validated and defaulted by ProcessSpecification

	log.Printf("Getting task details from pre-validated plugin specification: %s (Version: %s)", pluginSpec.Plugin.Name, pluginSpec.Plugin.Version)

	// 1. Access the embedded discovery task (ID and Type are guaranteed to be set by ProcessSpecification/validatePluginStructure)
	discoveryTask := pluginSpec.Plugin.Components.Discovery

	// 2. Validate the Image URL existence (Format check was done during ProcessSpecification)
	log.Printf("Validating image existence for discovery task (ID: %s, Image: %s)...", discoveryTask.ID, discoveryTask.ImageURL)
	// Assumes validateImageManifestExists exists in artifact_validation.go
	if err := v.validateImageManifestExists(discoveryTask.ImageURL); err != nil {
		// Wrap error for better context
		return nil, fmt.Errorf("discovery task image URI existence check failed for '%s' (plugin: %s): %w",
			discoveryTask.ImageURL, pluginSpec.Plugin.Name, err)
	}
	log.Printf("Image existence validated successfully for: %s", discoveryTask.ImageURL)

	// 3. Populate the TaskDetails struct, including inherited fields
	details := &TaskDetails{
		// Task-specific fields
		TaskID:            discoveryTask.ID, // This ID includes the default if it was applied
		TaskName:          discoveryTask.Name,
		ValidatedImageURI: discoveryTask.ImageURL, // Use the validated URL
		Command:           discoveryTask.Command,
		Timeout:           discoveryTask.Timeout,
		ScaleConfig:       discoveryTask.ScaleConfig,
		Params:            discoveryTask.Params,
		Configs:           discoveryTask.Configs,
		RunSchedule:       discoveryTask.RunSchedule,

		// Inherited fields from PluginSpecification
		PluginName:                pluginSpec.Plugin.Name,
		APIVersion:                pluginSpec.APIVersion,
		SupportedPlatformVersions: pluginSpec.Plugin.SupportedPlatformVersions, // Copy slice
		Metadata:                  pluginSpec.Plugin.Metadata,                  // Copy struct
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

	platformComp := spec.Plugin.Components.PlatformBinary
	cloudqlComp := spec.Plugin.Components.CloudQLBinary
	// Use ImageURL from embedded task (ID/Type are guaranteed to be set by validatePluginStructure)
	discoveryImageURL := spec.Plugin.Components.Discovery.ImageURL

	// --- Validate Discovery Task Image ---
	if validateDiscovery {
		log.Printf("Validating Discovery Image: %s", discoveryImageURL)
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
