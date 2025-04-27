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
	if !isNonEmpty(spec.APIVersion) || spec.APIVersion != APIVersionV1 {
		return nil, fmt.Errorf("plugin specification '%s': api-version is required and must be '%s', got '%s'", filePath, APIVersionV1, spec.APIVersion)
	}
	if !isNonEmpty(spec.Type) || spec.Type != SpecTypePlugin {
		return nil, fmt.Errorf("plugin specification '%s': type is required and must be '%s', got '%s'", filePath, SpecTypePlugin, spec.Type)
	}

	log.Printf("Validating plugin specification structure for '%s'...", filePath)
	// Defaulting for embedded task happens inside validatePluginStructure
	if err := v.validatePluginStructure(&spec); err != nil {
		return nil, fmt.Errorf("plugin specification structure validation failed for '%s': %w", filePath, err)
	}
	log.Printf("Plugin specification '%s' (Name: %s) structure validation successful.", filePath, spec.Name)

	// --- Optional Checks ---
	// Platform Support Check
	if isNonEmpty(platformVersion) {
		log.Printf("Checking platform support for plugin '%s' (Version: %s) against platform '%s'", spec.Name, spec.Version, platformVersion)
		supported, supportErr := v.CheckPlatformSupport(&spec, platformVersion) // Assumes method exists on v
		if supportErr != nil {
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
			return nil, fmt.Errorf("plugin artifact validation failed for '%s': %w", filePath, err)
		}
		log.Printf("Plugin artifact validation successful for '%s'.", spec.Name)
	} else {
		log.Println("Skipping plugin artifact validation as requested.")
	}

	return &spec, nil
}

// validatePluginStructure performs structural checks for 'plugin' specifications.
// Assumes isNonEmpty, v.validateMetadata, idFormatRegex, v.validateTaskStructure,
// validateOptionalTagsMap, and validateOptionalClassification are defined elsewhere.
func (v *defaultValidator) validatePluginStructure(spec *PluginSpecification) error {
	if spec == nil {
		return errors.New("plugin specification cannot be nil")
	}

	specContext := "plugin specification (Name missing)"
	if isNonEmpty(spec.Name) {
		specContext = fmt.Sprintf("plugin specification (Name: %s)", spec.Name)
	} else {
		return errors.New("plugin specification: name is required")
	}
	// APIVersion, Type validated previously

	// --- Top-Level Plugin Fields ---
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
			return fmt.Errorf("%s: supported-platform-versions entry %d ('%s') is not valid: %w", specContext, i, constraintStr, err)
		}
	}

	// --- Metadata Block Fields ---
	if err := v.validateMetadata(&spec.Metadata, specContext+" metadata"); err != nil {
		return err
	} // Assumes method exists

	// --- Components Block Fields ---
	if spec.Components == (PluginComponents{}) {
		return fmt.Errorf("%s: components section is required", specContext)
	}
	components := &spec.Components
	if components.Discovery == (DiscoveryComponent{}) {
		return fmt.Errorf("%s: components.discovery section is required", specContext)
	}
	discoveryComp := &components.Discovery
	hasTaskID := isNonEmpty(discoveryComp.TaskID)
	hasTaskSpec := discoveryComp.TaskSpec != nil
	if !hasTaskID && !hasTaskSpec {
		return fmt.Errorf("%s: components.discovery requires 'task-id' or 'task-spec'", specContext)
	}
	if hasTaskID && hasTaskSpec {
		return fmt.Errorf("%s: components.discovery cannot have both 'task-id' and 'task-spec'", specContext)
	}

	if hasTaskID {
		if !idFormatRegex.MatchString(discoveryComp.TaskID) {
			return fmt.Errorf("%s: components.discovery.task-id '%s' has invalid format", specContext, discoveryComp.TaskID)
		} // Assumes regex exists
		log.Printf("Info: %s uses referenced discovery task ID: %s", specContext, discoveryComp.TaskID)
	} else { // hasTaskSpec must be true
		if err := v.validateTaskStructure(discoveryComp.TaskSpec, false); err != nil {
			return fmt.Errorf("%s: components.discovery.task-spec validation failed: %w", specContext, err)
		} // Assumes method exists, false=embedded
		// Default embedded task fields
		embeddedTask := discoveryComp.TaskSpec
		defaultSuffix := "-task"
		defaultID := spec.Name + defaultSuffix
		defaultName := spec.Name + defaultSuffix
		defaultDescription := spec.Name + " Task"
		if !isNonEmpty(embeddedTask.ID) {
			embeddedTask.ID = defaultID
		}
		if !isNonEmpty(embeddedTask.Name) {
			embeddedTask.Name = defaultName
		}
		if !isNonEmpty(embeddedTask.Description) {
			embeddedTask.Description = defaultDescription
		}
		if !isNonEmpty(embeddedTask.Type) {
			embeddedTask.Type = SpecTypeTask
		}
		if embeddedTask.Type != SpecTypeTask {
			return fmt.Errorf("%s: embedded task type must be '%s', got '%s'", specContext, SpecTypeTask, embeddedTask.Type)
		}
	}

	// --- Downloadable Components ---
	platformComp := components.PlatformBinary
	cloudqlComp := components.CloudQLBinary
	if !isNonEmpty(platformComp.URI) {
		return fmt.Errorf("%s: components.platform-binary.uri is required", specContext)
	}
	if !isNonEmpty(cloudqlComp.URI) {
		return fmt.Errorf("%s: components.cloudql-binary.uri is required", specContext)
	}
	if platformComp.URI == cloudqlComp.URI {
		if !isNonEmpty(platformComp.PathInArchive) {
			return fmt.Errorf("%s: platform-binary.path-in-archive required when URIs match", specContext)
		}
		if !isNonEmpty(cloudqlComp.PathInArchive) {
			return fmt.Errorf("%s: cloudql-binary.path-in-archive required when URIs match", specContext)
		}
		if platformComp.PathInArchive == cloudqlComp.PathInArchive {
			return fmt.Errorf("%s: platform-binary.path-in-archive cannot match cloudql-binary.path-in-archive when URIs match", specContext)
		}
	}

	// --- Sample Data ---
	if spec.SampleData != nil && !isNonEmpty(spec.SampleData.URI) {
		return fmt.Errorf("%s: sample-data.uri is required when sample-data section present", specContext)
	}

	// --- Tags Validation ---
	if err := validateOptionalTagsMap(spec.Tags, specContext); err != nil {
		return err
	} // Assumes helper exists

	// --- Classification Validation --- <<< ADDED THIS CALL
	if err := validateOptionalClassification(spec.Classification, specContext); err != nil {
		return err
	} // Assumes helper exists

	return nil
} // --- END validatePluginStructure ---

// getTaskDetailsFromPluginSpecificationImpl implements logic for GetTaskDetailsFromPluginSpecification.
// Assumes isNonEmpty and v.validateImageManifestExists are defined elsewhere.
func (v *defaultValidator) getTaskDetailsFromPluginSpecificationImpl(pluginSpec *PluginSpecification) (*TaskDetails, error) {
	if pluginSpec == nil {
		return nil, errors.New("input PluginSpecification cannot be nil")
	}

	discoveryComp := pluginSpec.Components.Discovery

	// Handle referenced task
	if isNonEmpty(discoveryComp.TaskID) {
		log.Printf("Returning partial task details for referenced task ID '%s' from plugin '%s'", discoveryComp.TaskID, pluginSpec.Name)
		// NOTE: Tags & Classification are NOT inherited when referencing an external task ID.
		return &TaskDetails{
			PluginName:                pluginSpec.Name,
			APIVersion:                pluginSpec.APIVersion,
			SupportedPlatformVersions: pluginSpec.SupportedPlatformVersions,
			Metadata:                  pluginSpec.Metadata,
			IsReference:               true,
			ReferencedTaskID:          discoveryComp.TaskID,
			// Tags: nil, // Omitted
			// Classification: nil, // Omitted
		}, nil
	}

	// Handle embedded task
	if discoveryComp.TaskSpec == nil {
		return nil, fmt.Errorf("internal error: plugin '%s' discovery has neither task-id nor task-spec", pluginSpec.Name)
	}
	log.Printf("Getting full task details from embedded task spec within plugin: %s (Version: %s)", pluginSpec.Name, pluginSpec.Version)
	embeddedTask := discoveryComp.TaskSpec

	// Validate Image Exists
	log.Printf("Validating image existence for embedded task (ID: %s, Image: %s)...", embeddedTask.ID, embeddedTask.ImageURL)
	if err := v.validateImageManifestExists(embeddedTask.ImageURL); err != nil { // Assumes method exists
		return nil, fmt.Errorf("embedded discovery task image check failed for '%s' (plugin: %s): %w", embeddedTask.ImageURL, pluginSpec.Name, err)
	}
	log.Printf("Image existence validated successfully for: %s", embeddedTask.ImageURL)

	// Populate TaskDetails, including inherited fields
	// Create copies of slices to prevent accidental modification
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
	// Tags map and Classification slice are assigned directly (shallow copy)

	details := &TaskDetails{
		TaskID:                    embeddedTask.ID,
		TaskName:                  embeddedTask.Name,
		TaskDescription:           embeddedTask.Description,
		ValidatedImageURI:         embeddedTask.ImageURL,
		Command:                   commandCopy,
		Timeout:                   embeddedTask.Timeout,
		ScaleConfig:               embeddedTask.ScaleConfig, // Struct copy ok
		Params:                    paramsCopy,
		Configs:                   configsCopy,
		RunSchedule:               runScheduleCopy,
		PluginName:                pluginSpec.Name,
		APIVersion:                pluginSpec.APIVersion,
		SupportedPlatformVersions: supportedVersionsCopy,
		Metadata:                  pluginSpec.Metadata, // Struct copy ok
		Tags:                      pluginSpec.Tags,     // Inherit Tags
		// Classification: pluginSpec.Classification, // <<< REMOVED: Classification not in TaskDetails anymore
		IsReference: false,
	}

	log.Printf("Successfully retrieved and validated task details for embedded task ID '%s' from plugin '%s'", details.TaskID, details.PluginName)
	return details, nil
} // --- END getTaskDetailsFromPluginSpecificationImpl ---

// validatePluginArtifacts handles artifact validation logic.
// Assumes isNonEmpty and artifact validation methods (v.validate...) exist elsewhere.
func (v *defaultValidator) validatePluginArtifacts(spec *PluginSpecification, artifactType string) error {
	if spec == nil {
		return errors.New("plugin spec cannot be nil for artifact validation")
	}

	normalizedType := strings.ToLower(strings.TrimSpace(artifactType))
	if !isNonEmpty(normalizedType) {
		normalizedType = ArtifactTypeAll
	}
	log.Printf("--- Starting Plugin Artifact Validation (Plugin: %s, Type: %s) ---", spec.Name, normalizedType)

	validateDiscovery, validatePlatform, validateCloudQL := false, false, false
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
			logScope += " (Discovery referenced)"
		}
		log.Printf("Scope: Validating %s.", logScope)
	case ArtifactTypeDiscovery:
		if discoveryIsEmbedded {
			validateDiscovery = true
			log.Println("Scope: Validating only Discovery Image.")
		} else {
			log.Println("Scope: Skipping Discovery Image (referenced).")
		}
	case ArtifactTypePlatformBinary:
		validatePlatform = true
		log.Println("Scope: Validating only PlatformBinary.")
	case ArtifactTypeCloudQLBinary:
		validateCloudQL = true
		log.Println("Scope: Validating only CloudQLBinary.")
	default:
		return fmt.Errorf("invalid artifactType '%s'. Must be one of: '%s', '%s', '%s', or '%s'", artifactType, ArtifactTypeDiscovery, ArtifactTypePlatformBinary, ArtifactTypeCloudQLBinary, ArtifactTypeAll)
	}

	var wg sync.WaitGroup
	errChan := make(chan error, 3)
	var platformData []byte
	platformComp := spec.Components.PlatformBinary
	cloudqlComp := spec.Components.CloudQLBinary

	// Validate Discovery Image
	if validateDiscovery {
		discoveryImageURL := spec.Components.Discovery.TaskSpec.ImageURL
		log.Printf("Validating Discovery Image: %s", discoveryImageURL)
		if err := v.validateImageManifestExists(discoveryImageURL); err != nil {
			errChan <- fmt.Errorf("discovery image validation failed for '%s': %w", discoveryImageURL, err)
		} else {
			log.Printf("Discovery Image valid: %s", discoveryImageURL)
		}
	}

	// Validate Platform Binary (concurrently)
	if validatePlatform {
		wg.Add(1)
		go func(comp Component) {
			defer wg.Done()
			log.Printf("Validating PlatformBinary artifact: %s", comp.URI)
			var err error
			platformData, err = v.validateSingleDownloadableComponent(comp, ArtifactTypePlatformBinary)
			if err != nil {
				errChan <- fmt.Errorf("platform-binary artifact validation failed for URI '%s': %w", comp.URI, err)
				platformData = nil
			} else {
				log.Printf("PlatformBinary artifact valid: %s", comp.URI)
			}
		}(platformComp)
	}

	// Validate CloudQL Binary (Separate URI, concurrently)
	if validateCloudQL && platformComp.URI != cloudqlComp.URI {
		wg.Add(1)
		go func(comp Component) {
			defer wg.Done()
			log.Printf("Validating CloudQLBinary artifact (separate URI): %s", comp.URI)
			_, err := v.validateSingleDownloadableComponent(comp, ArtifactTypeCloudQLBinary)
			if err != nil {
				errChan <- fmt.Errorf("cloudql-binary artifact validation failed for URI '%s': %w", comp.URI, err)
			} else {
				log.Printf("CloudQLBinary artifact valid (separate URI): %s", comp.URI)
			}
		}(cloudqlComp)
	}

	wg.Wait() // Wait for binary downloads

	// Validate CloudQL Binary (Shared URI Case, sequentially after potential download)
	if validateCloudQL && platformComp.URI == cloudqlComp.URI {
		log.Printf("Validating CloudQLBinary path '%s' (shared URI %s)...", cloudqlComp.PathInArchive, cloudqlComp.URI)
		if validatePlatform { // Did platform binary validation run?
			if platformData == nil {
				log.Printf("Skipping cloudql-binary path check: shared archive '%s' failed download/validation.", cloudqlComp.URI)
			} else {
				if err := v.validateArchivePathExists(platformData, cloudqlComp.PathInArchive, cloudqlComp.URI); err != nil {
					errChan <- fmt.Errorf("cloudql-binary path validation failed in archive '%s': %w", cloudqlComp.URI, err)
				} else {
					log.Printf("CloudQLBinary path valid (shared URI path '%s' exists).", cloudqlComp.PathInArchive)
				}
			}
		} else { // Platform binary validation skipped, need to download specifically for this check
			log.Printf("Warning: Downloading shared archive '%s' again for CloudQL path check.", platformComp.URI)
			sharedData, dlErr := v.validateSingleDownloadableComponent(platformComp, "shared archive for CloudQL check")
			if dlErr != nil {
				errChan <- fmt.Errorf("failed download for cloudql path check '%s': %w", platformComp.URI, dlErr)
			} else if sharedData != nil {
				if err := v.validateArchivePathExists(sharedData, cloudqlComp.PathInArchive, cloudqlComp.URI); err != nil {
					errChan <- fmt.Errorf("cloudql-binary path validation failed in archive '%s': %w", cloudqlComp.URI, err)
				} else {
					log.Printf("CloudQLBinary path valid (shared URI path '%s' exists).", cloudqlComp.PathInArchive)
				}
			}
		}
	}

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

// getEmbeddedTaskSpecificationImpl generates a standalone TaskSpecification string from an embedded task.
// Assumes isNonEmpty is defined elsewhere.
func (v *defaultValidator) getEmbeddedTaskSpecificationImpl(pluginSpec *PluginSpecification, format string) (string, error) {
	if pluginSpec == nil {
		return "", errors.New("input PluginSpecification cannot be nil")
	}
	discoveryComp := pluginSpec.Components.Discovery
	if isNonEmpty(discoveryComp.TaskID) {
		return "", fmt.Errorf("plugin '%s' uses task-id reference; cannot generate embedded specification", pluginSpec.Name)
	}
	if discoveryComp.TaskSpec == nil {
		return "", fmt.Errorf("internal error: plugin '%s' discovery has no embedded task-spec", pluginSpec.Name)
	}

	log.Printf("Generating standalone specification string (format: %s) for embedded task from plugin: %s", format, pluginSpec.Name)
	embeddedTask := discoveryComp.TaskSpec

	// Construct standalone struct, inheriting Plugin fields where appropriate for standalone Tasks
	metadataCopy := pluginSpec.Metadata
	supportedVersionsCopy := make([]string, len(pluginSpec.SupportedPlatformVersions))
	copy(supportedVersionsCopy, pluginSpec.SupportedPlatformVersions)
	metadataPtr := &metadataCopy

	// NOTE: Classification IS NOT inherited/included as per requirement
	standaloneTask := TaskSpecification{
		APIVersion:                pluginSpec.APIVersion, // Inherited
		Type:                      SpecTypeTask,          // Explicit
		Metadata:                  metadataPtr,           // Inherited
		SupportedPlatformVersions: supportedVersionsCopy, // Inherited
		ID:                        embeddedTask.ID,       // From (defaulted) embedded
		Name:                      embeddedTask.Name,
		Description:               embeddedTask.Description,
		IsEnabled:                 embeddedTask.IsEnabled,
		ImageURL:                  embeddedTask.ImageURL,
		Command:                   embeddedTask.Command,
		Timeout:                   embeddedTask.Timeout,
		ScaleConfig:               embeddedTask.ScaleConfig,
		Params:                    embeddedTask.Params,
		Configs:                   embeddedTask.Configs,
		RunSchedule:               embeddedTask.RunSchedule,
		Tags:                      pluginSpec.Tags, // Inherited Tags
		// Classification field omitted
	}

	// Marshal to requested format
	var outputBytes []byte
	var err error
	outputFormat := strings.ToLower(strings.TrimSpace(format))
	if outputFormat == FormatJSON {
		outputBytes, err = json.MarshalIndent(&standaloneTask, "", "  ")
		if err != nil {
			return "", fmt.Errorf("failed to marshal standalone task spec to JSON: %w", err)
		}
		log.Printf("Successfully marshaled embedded task spec to JSON.")
	} else {
		if outputFormat != FormatYAML && format != "" {
			log.Printf("Warning: Invalid format '%s', defaulting to YAML.", format)
		}
		outputBytes, err = yaml.Marshal(&standaloneTask)
		if err != nil {
			return "", fmt.Errorf("failed to marshal standalone task spec to YAML: %w", err)
		}
		log.Printf("Successfully marshaled embedded task spec to YAML.")
	}

	return string(outputBytes), nil
} // --- END getEmbeddedTaskSpecificationImpl ---
