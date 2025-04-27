// plugin_spec.go
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
// Assumes isNonEmpty, v.CheckPlatformSupport, v.validatePluginArtifacts are defined elsewhere.
func (v *defaultValidator) processPluginSpec(data []byte, filePath string, platformVersion string, artifactValidationType string, skipArtifactValidation bool) (*PluginSpecification, error) {
	var spec PluginSpecification
	// Unmarshal directly into the PluginSpecification struct
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("failed to parse YAML file '%s' as plugin spec: %w", filePath, err)
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

	log.Printf("Validating plugin specification structure for '%s'...", filePath)
	// Defaulting for embedded task ID/Type/Name/Description happens inside validatePluginStructure
	if err := v.validatePluginStructure(&spec); err != nil {
		// Wrap validation error with file path context
		return nil, fmt.Errorf("plugin specification structure validation failed for '%s': %w", filePath, err)
	}
	log.Printf("Plugin specification '%s' (Name: %s) structure validation successful.", filePath, spec.Name)

	// --- Optional Checks ---
	// Platform Support Check
	if isNonEmpty(platformVersion) {
		log.Printf("Checking platform support for plugin '%s' (Version: %s) against platform '%s'", spec.Name, spec.Version, platformVersion)
		supported, supportErr := v.CheckPlatformSupport(&spec, platformVersion) // Assumes method exists on v
		if supportErr != nil {
			// Log as warning, don't fail validation just for support check error
			log.Printf("Warning: Error checking platform support for plugin '%s': %v", spec.Name, supportErr)
		} else {
			status := "IS NOT"
			if supported {
				status = "IS"
			}
			log.Printf("Platform version %s %s supported by plugin '%s' version '%s'.", platformVersion, status, spec.Name, spec.Version)
		}
	} else {
		log.Println("Skipping platform support check (no platform version provided).")
	}

	// Artifact Validation
	if !skipArtifactValidation {
		log.Printf("Starting plugin artifact validation for '%s'...", spec.Name)
		// Assumes validatePluginArtifacts method exists on v
		if err := v.validatePluginArtifacts(&spec, artifactValidationType); err != nil {
			// Artifact validation failure IS a validation error for the spec
			return nil, fmt.Errorf("plugin artifact validation failed for '%s': %w", filePath, err)
		}
		log.Printf("Plugin artifact validation successful for '%s'.", spec.Name)
	} else {
		log.Println("Skipping plugin artifact validation as requested.")
	}

	return &spec, nil
}

// validatePluginStructure performs structural checks specific to 'plugin' specifications,
// including metadata, components, discovery task defaulting, and optional tags.
// Assumes isNonEmpty, v.validateMetadata, idFormatRegex, v.validateTaskStructure, and validateOptionalTagsMap are defined elsewhere.
func (v *defaultValidator) validatePluginStructure(spec *PluginSpecification) error {
	if spec == nil {
		return errors.New("plugin specification cannot be nil")
	}
	// Define context early
	specContext := "plugin specification (Name missing)"
	if isNonEmpty(spec.Name) {
		specContext = fmt.Sprintf("plugin specification (Name: %s)", spec.Name)
	} else {
		return errors.New("plugin specification: name is required") // Name is mandatory
	}
	// API Version and Type already validated by processPluginSpec

	// --- Top-Level Plugin Fields --- (Name checked above)
	if !isNonEmpty(spec.Version) {
		return fmt.Errorf("%s: version is required", specContext)
	}
	if _, err := semver.NewVersion(spec.Version); err != nil {
		return fmt.Errorf("%s: invalid semantic version format for version '%s': %w", specContext, spec.Version, err)
	}
	if len(spec.SupportedPlatformVersions) == 0 {
		return fmt.Errorf("%s: supported-platform-versions requires at least one constraint entry", specContext)
	}
	for i, constraintStr := range spec.SupportedPlatformVersions {
		if !isNonEmpty(constraintStr) {
			return fmt.Errorf("%s: supported-platform-versions entry %d cannot be empty", specContext, i)
		}
		if _, err := semver.NewConstraint(constraintStr); err != nil {
			return fmt.Errorf("%s: supported-platform-versions entry %d ('%s') is not a valid semantic version constraint: %w", specContext, i, constraintStr, err)
		}
	}

	// --- Metadata Block Fields ---
	// Assumes validateMetadata method exists on v
	if err := v.validateMetadata(&spec.Metadata, specContext+" metadata"); err != nil {
		return err // Error from validateMetadata is already contextualized
	}

	// --- Components Block Fields ---
	// Check presence of Components block itself might be needed depending on schema strictness
	if spec.Components == (PluginComponents{}) { // Basic check if the struct is zeroed
		return fmt.Errorf("%s: components section is required", specContext)
	}
	components := &spec.Components // Use pointer for modification

	// *** Validate Discovery Component (Task ID Reference or Embedded Task Spec) ***
	// Check presence of Discovery block itself
	if components.Discovery == (DiscoveryComponent{}) { // Basic check if zeroed
		return fmt.Errorf("%s: components.discovery section is required", specContext)
	}
	discoveryComp := &components.Discovery // Use pointer
	hasTaskID := isNonEmpty(discoveryComp.TaskID)
	hasTaskSpec := discoveryComp.TaskSpec != nil

	if !hasTaskID && !hasTaskSpec {
		return fmt.Errorf("%s: components.discovery requires either 'task-id' or 'task-spec' to be defined", specContext)
	}
	if hasTaskID && hasTaskSpec {
		return fmt.Errorf("%s: components.discovery cannot have both 'task-id' ('%s') and 'task-spec' defined", specContext, discoveryComp.TaskID)
	}

	if hasTaskID {
		// Validate the ID format if a reference is used
		// Assumes idFormatRegex is defined elsewhere
		if !idFormatRegex.MatchString(discoveryComp.TaskID) {
			// Use more specific error message from query_spec validation if possible
			return fmt.Errorf("%s: components.discovery.task-id '%s' has invalid format (must match standard ID rules)", specContext, discoveryComp.TaskID)
		}
		log.Printf("Info: %s uses referenced discovery task ID: %s", specContext, discoveryComp.TaskID)

	} else { // hasTaskSpec must be true here
		// Validate the embedded task specification structure
		// Assumes validateTaskStructure method exists on v
		if err := v.validateTaskStructure(discoveryComp.TaskSpec, false); err != nil { // false = embedded
			return fmt.Errorf("%s: components.discovery.task-spec validation failed: %w", specContext, err)
		}

		// *** Default ID, Type, Name, Description for Embedded Task ***
		embeddedTask := discoveryComp.TaskSpec
		defaultSuffix := "-task"
		defaultID := spec.Name + defaultSuffix
		defaultName := spec.Name + defaultSuffix
		defaultDescription := spec.Name + " Task"

		if !isNonEmpty(embeddedTask.ID) {
			log.Printf("Info: %s: Embedded discovery task is missing 'id'. Defaulting to '%s'.", specContext, defaultID)
			embeddedTask.ID = defaultID
		}
		if !isNonEmpty(embeddedTask.Name) {
			log.Printf("Info: %s: Embedded discovery task (ID: %s) is missing 'name'. Defaulting to '%s'.", specContext, embeddedTask.ID, defaultName)
			embeddedTask.Name = defaultName
		}
		if !isNonEmpty(embeddedTask.Description) {
			log.Printf("Info: %s: Embedded discovery task (ID: %s) is missing 'description'. Defaulting to '%s'.", specContext, embeddedTask.ID, defaultDescription)
			embeddedTask.Description = defaultDescription
		}
		if !isNonEmpty(embeddedTask.Type) {
			log.Printf("Info: %s: Embedded discovery task (ID: %s) is missing 'type'. Defaulting to '%s'.", specContext, embeddedTask.ID, SpecTypeTask)
			embeddedTask.Type = SpecTypeTask // Default Type to "task"
		}
		// Post-defaulting check: Ensure type is indeed "task"
		if embeddedTask.Type != SpecTypeTask {
			return fmt.Errorf("%s: components.discovery.task-spec (ID: %s): type must be '%s' if specified, got '%s'", specContext, embeddedTask.ID, SpecTypeTask, embeddedTask.Type)
		}
	}

	// *** Validate Downloadable Component References ***
	platformComp := components.PlatformBinary // Use value copy for checks
	cloudqlComp := components.CloudQLBinary   // Use value copy for checks

	if !isNonEmpty(platformComp.URI) {
		return fmt.Errorf("%s: components.platform-binary.uri is required", specContext)
	}
	if !isNonEmpty(cloudqlComp.URI) {
		return fmt.Errorf("%s: components.cloudql-binary.uri is required", specContext)
	}

	// If URIs are the same, both components MUST specify a PathInArchive, and they must differ.
	if platformComp.URI == cloudqlComp.URI {
		log.Printf("Info: %s: PlatformBinary and CloudQLBinary share the same URI: %s. PathInArchive is required for both and must differ.", specContext, platformComp.URI)
		if !isNonEmpty(platformComp.PathInArchive) {
			return fmt.Errorf("%s: components.platform-binary.path-in-archive is required when its URI ('%s') matches cloudql-binary.uri", specContext, platformComp.URI)
		}
		if !isNonEmpty(cloudqlComp.PathInArchive) {
			return fmt.Errorf("%s: components.cloudql-binary.path-in-archive is required when its URI ('%s') matches platform-binary.uri", specContext, cloudqlComp.URI)
		}
		if platformComp.PathInArchive == cloudqlComp.PathInArchive {
			return fmt.Errorf("%s: components.platform-binary.path-in-archive ('%s') cannot be the same as cloudql-binary.path-in-archive ('%s') when URIs match", specContext, platformComp.PathInArchive, cloudqlComp.PathInArchive)
		}
	}

	// --- Sample Data (Optional) ---
	if spec.SampleData != nil { // Check if the section exists
		if !isNonEmpty(spec.SampleData.URI) {
			return fmt.Errorf("%s: sample-data.uri is required when the sample-data section is present", specContext)
		}
		// Could add checksum validation for sample data here if needed
	}

	// --- Tags Validation (Using Helper) ---
	// Assumes validateOptionalTagsMap is defined elsewhere (e.g., common.go)
	if err := validateOptionalTagsMap(spec.Tags, specContext); err != nil {
		return err // Return error from helper
	}

	return nil
} // --- END validatePluginStructure ---

// GetTaskDetailsFromPluginSpecification extracts discovery task details from an already validated PluginSpecification.
// It includes inherited fields (Metadata, SupportedPlatformVersions, APIVersion, Tags) and performs an additional image existence check.
// Returns an error if the discovery component used a task-id reference instead of embedding the spec.
// Assumes isNonEmpty and v.validateImageManifestExists are defined elsewhere.
func (v *defaultValidator) getTaskDetailsFromPluginSpecificationImpl(pluginSpec *PluginSpecification) (*TaskDetails, error) {
	if pluginSpec == nil {
		return nil, errors.New("input PluginSpecification cannot be nil")
	}
	// Assume pluginSpec is already structurally validated and defaulted by processPluginSpec

	discoveryComp := pluginSpec.Components.Discovery

	// Check if the discovery component is a reference or missing the spec
	if isNonEmpty(discoveryComp.TaskID) {
		// Return details containing mostly inherited info and the reference ID
		log.Printf("Returning partial task details for referenced task ID '%s' from plugin '%s'", discoveryComp.TaskID, pluginSpec.Name)
		// NOTE: Tags are NOT inherited when referencing an external task ID.
		return &TaskDetails{
			PluginName:                pluginSpec.Name,
			APIVersion:                pluginSpec.APIVersion,
			SupportedPlatformVersions: pluginSpec.SupportedPlatformVersions, // Copy slice
			Metadata:                  pluginSpec.Metadata,                  // Copy struct
			IsReference:               true,
			ReferencedTaskID:          discoveryComp.TaskID,
			// Tags field is omitted (or nil) as it's not inherited for references
		}, nil // Not an error, but indicates partial data
	}
	if discoveryComp.TaskSpec == nil {
		// This case should ideally be caught by validatePluginStructure, but check defensively
		return nil, fmt.Errorf("internal error: plugin '%s' discovery component has neither task-id nor task-spec", pluginSpec.Name)
	}

	log.Printf("Getting full task details from embedded task spec within plugin: %s (Version: %s)", pluginSpec.Name, pluginSpec.Version)

	// 1. Access the embedded discovery task (ID, Name, Description, Type are guaranteed to be set)
	embeddedTask := discoveryComp.TaskSpec // Use the embedded spec pointer

	// 2. Validate the Image URL existence (Format check already done)
	log.Printf("Validating image existence for embedded discovery task (ID: %s, Image: %s)...", embeddedTask.ID, embeddedTask.ImageURL)
	// Assumes validateImageManifestExists method exists on v
	if err := v.validateImageManifestExists(embeddedTask.ImageURL); err != nil {
		// Wrap error for better context
		return nil, fmt.Errorf("embedded discovery task image URI existence check failed for '%s' (plugin: %s): %w",
			embeddedTask.ImageURL, pluginSpec.Name, err)
	}
	log.Printf("Image existence validated successfully for: %s", embeddedTask.ImageURL)

	// 3. Populate the TaskDetails struct, including inherited fields AND TAGS
	// Create copies of slices to prevent accidental modification of the original spec
	commandCopy := make([]string, len(embeddedTask.Command))
	copy(commandCopy, embeddedTask.Command)
	paramsCopy := make([]string, len(embeddedTask.Params))
	copy(paramsCopy, embeddedTask.Params)
	configsCopy := make([]interface{}, len(embeddedTask.Configs))
	copy(configsCopy, embeddedTask.Configs)
	runScheduleCopy := make([]RunScheduleEntry, len(embeddedTask.RunSchedule))
	copy(runScheduleCopy, embeddedTask.RunSchedule)
	supportedVersionsCopy := make([]string, len(pluginSpec.SupportedPlatformVersions))
	copy(supportedVersionsCopy, pluginSpec.SupportedPlatformVersions)
	// Tags map is assigned directly (shallow copy), modify if deep copy needed later.

	details := &TaskDetails{
		// Task-specific fields from the embedded spec
		TaskID:            embeddedTask.ID,          // Includes defaults if applied
		TaskName:          embeddedTask.Name,        // Includes defaults if applied
		TaskDescription:   embeddedTask.Description, // Includes defaults if applied
		ValidatedImageURI: embeddedTask.ImageURL,    // Use the validated URL
		Command:           commandCopy,              // Use copied slice
		Timeout:           embeddedTask.Timeout,
		ScaleConfig:       embeddedTask.ScaleConfig, // Struct copy is implicit
		Params:            paramsCopy,               // Use copied slice
		Configs:           configsCopy,              // Use copied slice
		RunSchedule:       runScheduleCopy,          // Use copied slice

		// Inherited fields from PluginSpecification
		PluginName:                pluginSpec.Name,
		APIVersion:                pluginSpec.APIVersion,
		SupportedPlatformVersions: supportedVersionsCopy, // Use copied slice
		Metadata:                  pluginSpec.Metadata,   // Struct copy is implicit
		Tags:                      pluginSpec.Tags,       // *** ADDED: Inherit Tags from Plugin ***

		// Indicate this was not from a reference
		IsReference: false,
	}

	log.Printf("Successfully retrieved and validated task details for embedded task ID '%s' from plugin '%s'", details.TaskID, details.PluginName)
	return details, nil
}

// validatePluginArtifacts handles the download and validation logic for 'plugin' type artifacts.
// Assumes methods v.validateImageManifestExists, v.validateSingleDownloadableComponent, v.validateArchivePathExists exist.
// Assumes isNonEmpty func exists.
func (v *defaultValidator) validatePluginArtifacts(spec *PluginSpecification, artifactType string) error {
	if spec == nil {
		return errors.New("plugin specification cannot be nil for artifact validation")
	}

	normalizedType := strings.ToLower(strings.TrimSpace(artifactType))
	if !isNonEmpty(normalizedType) {
		normalizedType = ArtifactTypeAll // Use exported constant
	}
	log.Printf("--- Starting Plugin Artifact Validation (Plugin: %s, Requested Type: %s) ---", spec.Name, normalizedType)

	validateDiscovery := false
	validatePlatform := false
	validateCloudQL := false
	discoveryIsEmbedded := spec.Components.Discovery.TaskSpec != nil

	switch normalizedType {
	case ArtifactTypeAll:
		if discoveryIsEmbedded {
			validateDiscovery = true
		}
		validatePlatform = true
		validateCloudQL = true
		logScope := "PlatformBinary, CloudQLBinary artifacts"
		if discoveryIsEmbedded {
			logScope = "Discovery Image, " + logScope
		} else {
			logScope += " (Discovery is referenced)"
		}
		log.Printf("Scope: Validating %s.", logScope)
	case ArtifactTypeDiscovery:
		if discoveryIsEmbedded {
			validateDiscovery = true
			log.Println("Scope: Validating only Discovery Image artifact.")
		} else {
			log.Println("Scope: Skipping Discovery Image artifact validation (Discovery is referenced).")
		}
	case ArtifactTypePlatformBinary:
		validatePlatform = true
		log.Println("Scope: Validating only PlatformBinary artifact.")
	case ArtifactTypeCloudQLBinary:
		validateCloudQL = true
		log.Println("Scope: Validating only CloudQLBinary artifact.")
	default:
		return fmt.Errorf("invalid artifactType '%s'. Must be one of: '%s', '%s', '%s', or '%s' (or empty)", artifactType, ArtifactTypeDiscovery, ArtifactTypePlatformBinary, ArtifactTypeCloudQLBinary, ArtifactTypeAll)
	}

	var wg sync.WaitGroup
	// Increase buffer size slightly in case multiple errors occur quickly
	errChan := make(chan error, 3)
	var platformData []byte // To store downloaded data for shared URI case

	platformComp := spec.Components.PlatformBinary
	cloudqlComp := spec.Components.CloudQLBinary

	// --- Validate Discovery Task Image ---
	if validateDiscovery {
		discoveryImageURL := spec.Components.Discovery.TaskSpec.ImageURL
		log.Printf("Validating Discovery Image: %s", discoveryImageURL)
		if err := v.validateImageManifestExists(discoveryImageURL); err != nil { // Retries handled inside
			log.Printf("Error validating Discovery Image '%s': %v", discoveryImageURL, err)
			errChan <- fmt.Errorf("discovery image validation failed for '%s': %w", discoveryImageURL, err)
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
			_, err := v.validateSingleDownloadableComponent(comp, ArtifactTypeCloudQLBinary) // Retries handled inside
			if err != nil {
				log.Printf("Error validating CloudQLBinary artifact (separate URI) '%s': %v", comp.URI, err)
				errChan <- fmt.Errorf("cloudql-binary artifact validation failed for URI '%s': %w", comp.URI, err)
			} else {
				log.Printf("CloudQLBinary artifact validation successful (separate URI): %s", comp.URI)
			}
		}(cloudqlComp)
	}

	// Wait for concurrent binary downloads/validations to finish
	wg.Wait()

	// --- Validate CloudQL Binary (Shared URI Case) ---
	// This runs *after* the potential download of the shared archive by the platform binary validation.
	if validateCloudQL && platformComp.URI == cloudqlComp.URI {
		log.Printf("Validating CloudQLBinary artifact path '%s' (shared URI %s)...", cloudqlComp.PathInArchive, cloudqlComp.URI)
		// Check if platform binary validation ran and succeeded (platformData would be non-nil)
		if validatePlatform {
			if platformData == nil {
				// Error should have already been sent to errChan by the platform binary goroutine.
				// Log context here, but don't send another error for the download failure itself.
				log.Printf("Skipping cloudql-binary path check for '%s': shared archive download/validation failed for URI '%s'", cloudqlComp.PathInArchive, cloudqlComp.URI)
			} else {
				// Platform binary download succeeded, check path in the downloaded data
				log.Printf("Checking path '%s' within shared archive data from %s...", cloudqlComp.PathInArchive, cloudqlComp.URI)
				err := v.validateArchivePathExists(platformData, cloudqlComp.PathInArchive, cloudqlComp.URI) // Assumes method exists
				if err != nil {
					cloudqlErr := fmt.Errorf("cloudql-binary path validation failed within shared archive '%s': %w", cloudqlComp.URI, err)
					log.Printf("Error validating CloudQLBinary artifact path (shared URI): %v", cloudqlErr)
					errChan <- cloudqlErr
				} else {
					log.Printf("CloudQLBinary artifact validation successful (shared URI path '%s' exists).", cloudqlComp.PathInArchive)
				}
			}
		} else {
			// Platform validation was skipped, need to download the shared archive specifically for CloudQL path check
			log.Printf("Warning: CloudQL validation requested for shared URI '%s', but PlatformBinary validation was skipped. Downloading artifact again for path check.", platformComp.URI)
			sharedDataForCloudQL, downloadErr := v.validateSingleDownloadableComponent(platformComp, "shared archive for CloudQL path check") // Retries handled inside
			if downloadErr != nil {
				log.Printf("Error downloading shared archive '%s' for CloudQL path check: %v", platformComp.URI, downloadErr)
				errChan <- fmt.Errorf("failed to download shared archive '%s' for cloudql-binary path check: %w", platformComp.URI, downloadErr)
			} else if sharedDataForCloudQL != nil { // Check download succeeded
				log.Printf("Checking path '%s' within shared archive data from %s...", cloudqlComp.PathInArchive, cloudqlComp.URI)
				err := v.validateArchivePathExists(sharedDataForCloudQL, cloudqlComp.PathInArchive, cloudqlComp.URI) // Assumes method exists
				if err != nil {
					cloudqlErr := fmt.Errorf("cloudql-binary path validation failed within shared archive '%s': %w", cloudqlComp.URI, err)
					log.Printf("Error validating CloudQLBinary artifact path (shared URI): %v", cloudqlErr)
					errChan <- cloudqlErr
				} else {
					log.Printf("CloudQLBinary artifact validation successful (shared URI path '%s' exists).", cloudqlComp.PathInArchive)
				}
			}
			// No else needed if sharedDataForCloudQL is nil, downloadErr already sent
		}
	}

	// Close the error channel and collect any errors sent by goroutines or sequential checks
	close(errChan)
	var combinedErrors []string
	for err := range errChan {
		combinedErrors = append(combinedErrors, err.Error())
	}

	if len(combinedErrors) > 0 {
		return fmt.Errorf("one or more artifact validations failed for plugin '%s': %s", spec.Name, strings.Join(combinedErrors, "; "))
	}

	log.Println("--- Plugin Artifact Validation Completed Successfully ---")
	return nil
} // --- END validatePluginArtifacts ---

// GetEmbeddedTaskSpecification generates a standalone TaskSpecification representation (YAML or JSON string)
// from the embedded discovery task within a validated PluginSpecification.
// Assumes isNonEmpty is defined elsewhere.
func (v *defaultValidator) getEmbeddedTaskSpecificationImpl(pluginSpec *PluginSpecification, format string) (string, error) {
	if pluginSpec == nil {
		return "", errors.New("input PluginSpecification cannot be nil")
	}
	// Assumes pluginSpec is already validated and defaulted

	discoveryComp := pluginSpec.Components.Discovery

	// Check if the discovery component is a reference or missing the spec
	if isNonEmpty(discoveryComp.TaskID) {
		return "", fmt.Errorf("plugin '%s' uses task-id reference ('%s') for discovery; cannot generate embedded specification", pluginSpec.Name, discoveryComp.TaskID)
	}
	if discoveryComp.TaskSpec == nil {
		return "", fmt.Errorf("internal error: plugin '%s' discovery component has neither task-id nor task-spec", pluginSpec.Name)
	}

	log.Printf("Generating standalone specification string (format: %s) for embedded task from plugin: %s", format, pluginSpec.Name)

	// 1. Access the embedded discovery task (already defaulted)
	embeddedTask := discoveryComp.TaskSpec

	// 2. Construct the standalone TaskSpecification struct
	metadataCopy := pluginSpec.Metadata
	supportedVersionsCopy := make([]string, len(pluginSpec.SupportedPlatformVersions))
	copy(supportedVersionsCopy, pluginSpec.SupportedPlatformVersions)
	metadataPtr := &metadataCopy
	// Note: Directly assigning Tags map (shallow copy)
	standaloneTask := TaskSpecification{
		APIVersion:                pluginSpec.APIVersion,
		Type:                      SpecTypeTask,
		Metadata:                  metadataPtr,
		SupportedPlatformVersions: supportedVersionsCopy,
		ID:                        embeddedTask.ID,
		Name:                      embeddedTask.Name,
		Description:               embeddedTask.Description,
		IsEnabled:                 embeddedTask.IsEnabled,
		ImageURL:                  embeddedTask.ImageURL,
		Command:                   embeddedTask.Command, // Slices copied implicitly by marshal below? Prefer explicit copy if needed.
		Timeout:                   embeddedTask.Timeout,
		ScaleConfig:               embeddedTask.ScaleConfig, // Struct copy is implicit
		Params:                    embeddedTask.Params,
		Configs:                   embeddedTask.Configs,
		RunSchedule:               embeddedTask.RunSchedule,
		Tags:                      pluginSpec.Tags, // *** Inherit Tags from Plugin ***
	}

	// 3. Marshal the new struct to the requested format
	var outputBytes []byte
	var err error
	outputFormat := strings.ToLower(strings.TrimSpace(format))

	if outputFormat == FormatJSON {
		outputBytes, err = json.MarshalIndent(&standaloneTask, "", "  ")
		if err != nil {
			return "", fmt.Errorf("failed to marshal standalone task specification to JSON: %w", err)
		}
		log.Printf("Successfully marshaled embedded task spec to JSON.")
	} else {
		if outputFormat != FormatYAML && format != "" {
			log.Printf("Warning: Invalid format '%s' requested for GetEmbeddedTaskSpecification, defaulting to '%s'.", format, FormatYAML)
		}
		outputBytes, err = yaml.Marshal(&standaloneTask)
		if err != nil {
			return "", fmt.Errorf("failed to marshal standalone task specification to YAML: %w", err)
		}
		log.Printf("Successfully marshaled embedded task spec to YAML.")
	}

	return string(outputBytes), nil
} // --- END getEmbeddedTaskSpecificationImpl ---

// Note: Assumes defaultValidator struct is defined elsewhere (e.g., validator.go)
// Note: Assumes necessary helper functions (isNonEmpty, validateMetadata, etc.) and types/constants are defined elsewhere.
