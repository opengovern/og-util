// task_spec.go
package platformspec

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/Masterminds/semver/v3"
	"gopkg.in/yaml.v3"
)

// processTaskSpec handles the parsing and validation specific to standalone task specifications.
// It's called by ProcessSpecification in validator.go.
// Assumes isNonEmpty and v.validateImageManifestExists are defined elsewhere.
func (v *defaultValidator) processTaskSpec(data []byte, filePath string, skipArtifactValidation bool, defaultedAPIVersion, originalAPIVersion string) (*TaskSpecification, error) {
	var spec TaskSpecification
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("failed to parse specification file '%s' as task: %w", filePath, err)
	}

	// Apply defaulted API version if necessary
	if !isNonEmpty(spec.APIVersion) {
		spec.APIVersion = defaultedAPIVersion // Use defaulted version from base parse
	}
	// Ensure parsed APIVersion matches base (and is v1 after defaulting)
	if spec.APIVersion != APIVersionV1 {
		actualVersion := originalAPIVersion
		if isNonEmpty(spec.APIVersion) && spec.APIVersion != defaultedAPIVersion {
			actualVersion = spec.APIVersion
		}
		return nil, fmt.Errorf("task specification '%s': api-version must be '%s' (or omitted to default), got '%s'", filePath, APIVersionV1, actualVersion)
	}
	// Ensure type is set correctly (should be 'task' from base parse)
	if !isNonEmpty(spec.Type) {
		spec.Type = SpecTypeTask // Default if somehow missing after base parse
	} else if spec.Type != SpecTypeTask {
		return nil, fmt.Errorf("task specification '%s': type must be '%s', got '%s'", filePath, SpecTypeTask, spec.Type)
	}

	log.Printf("Validating standalone task specification structure for '%s'...", filePath)
	// Pass true for isStandalone check
	if err := v.validateTaskStructure(&spec, true); err != nil {
		// Wrap validation error with file path context
		return nil, fmt.Errorf("standalone task specification structure validation failed for '%s': %w", filePath, err)
	}
	log.Printf("Standalone task specification '%s' (ID: %s) structure validation successful.", filePath, spec.ID)

	// Task Image Validation (optional)
	if !skipArtifactValidation && isNonEmpty(spec.ImageURL) {
		log.Printf("Initiating standalone task image validation for '%s'...", spec.ImageURL)
		// Assumes validateImageManifestExists method exists on v
		err := v.validateImageManifestExists(spec.ImageURL)
		if err != nil {
			return nil, fmt.Errorf("standalone task image validation failed for '%s' (task ID: %s): %w", spec.ImageURL, spec.ID, err)
		}
		log.Printf("Standalone task image validation successful for '%s'.", spec.ImageURL)
	} else if !skipArtifactValidation {
		log.Printf("Skipping standalone task image validation (ImageURL empty or validation skipped) for task ID: %s.", spec.ID)
	}
	return &spec, nil
}

// GetTaskDefinition reads a specification file specifically expecting a 'task' type and parses it.
// It calls ProcessSpecification internally to ensure consistent validation.
// Assumes isNonEmpty is defined elsewhere.
func (v *defaultValidator) getTaskDefinitionImpl(data []byte, filePath string) (*TaskSpecification, error) {
	// Delegate validation and parsing to ProcessSpecification
	log.Printf("Loading standalone task definition from: %s (using ProcessSpecification)", filePath)
	processedSpec, err := v.ProcessSpecification(data, filePath, "", "", true) // Skip platform/artifact checks
	if err != nil {
		return nil, err // Error already contextualized
	}
	taskSpec, ok := processedSpec.(*TaskSpecification)
	if !ok {
		baseData, readErr := os.ReadFile(filePath)
		if readErr == nil {
			var base BaseSpecification
			if yaml.Unmarshal(baseData, &base) == nil && isNonEmpty(base.Type) {
				return nil, fmt.Errorf("expected type '%s' but found type '%s' in file '%s'", SpecTypeTask, base.Type, filePath)
			}
		}
		return nil, fmt.Errorf("internal error: ProcessSpecification for '%s' did not return *TaskSpecification", filePath)
	}
	log.Printf("Successfully loaded and validated standalone task definition for ID: %s", taskSpec.ID)
	return taskSpec, nil
}

// validateTaskStructure performs structural checks specific to 'task' specifications.
// Assumes isNonEmpty, v.validateMetadata, imageDigestRegex, validateOptionalTagsMap,
// and validateOptionalClassification are defined elsewhere.
func (v *defaultValidator) validateTaskStructure(spec *TaskSpecification, isStandalone bool) error {
	if spec == nil {
		return errors.New("task specification cannot be nil")
	}

	// Determine context for error messages early
	taskDesc := "embedded discovery task"
	if isStandalone {
		if isNonEmpty(spec.ID) {
			taskDesc = fmt.Sprintf("standalone task (ID: %s)", spec.ID)
		} else {
			taskDesc = "standalone task (ID missing)"
			// ID is required for standalone, check below
		}
	} else { // Embedded task
		if isNonEmpty(spec.ID) {
			taskDesc = fmt.Sprintf("embedded discovery task (ID: %s)", spec.ID)
		} else if isNonEmpty(spec.Name) {
			taskDesc = fmt.Sprintf("embedded discovery task (Name: %s)", spec.Name)
		}
	}

	// --- Standalone vs Embedded Field Requirements ---
	if isStandalone {
		// --- Standalone: Required Fields ---
		if !isNonEmpty(spec.APIVersion) || spec.APIVersion != APIVersionV1 {
			return fmt.Errorf("%s: api-version is required and must be '%s', got: '%s'", taskDesc, APIVersionV1, spec.APIVersion)
		}
		if spec.Metadata == nil {
			return fmt.Errorf("%s: metadata section is required", taskDesc)
		}
		if err := v.validateMetadata(spec.Metadata, fmt.Sprintf("%s metadata", taskDesc)); err != nil { // Assumes method exists on v
			return err
		}
		if len(spec.SupportedPlatformVersions) == 0 {
			return fmt.Errorf("%s: supported-platform-versions requires at least one constraint entry", taskDesc)
		}
		for i, constraintStr := range spec.SupportedPlatformVersions {
			if !isNonEmpty(constraintStr) {
				return fmt.Errorf("%s: supported-platform-versions entry %d cannot be empty", taskDesc, i)
			}
			if _, err := semver.NewConstraint(constraintStr); err != nil {
				return fmt.Errorf("%s: supported-platform-versions entry %d ('%s') is not a valid semantic version constraint: %w", taskDesc, i, constraintStr, err)
			}
		}
		if !isNonEmpty(spec.ID) {
			return fmt.Errorf("%s: id is required", taskDesc)
		} // Re-check since context depends on it
		if !isNonEmpty(spec.Name) {
			return fmt.Errorf("%s: name is required", taskDesc)
		}
		if !isNonEmpty(spec.Description) {
			return fmt.Errorf("%s: description is required", taskDesc)
		}
		if !isNonEmpty(spec.Type) || spec.Type != SpecTypeTask {
			return fmt.Errorf("%s: type is required and must be '%s', got: '%s'", taskDesc, SpecTypeTask, spec.Type)
		}

		// --- Standalone: Optional Field Validations ---
		// Validate Tags (Optional)
		if err := validateOptionalTagsMap(spec.Tags, taskDesc); err != nil { // Assumes helper exists
			return err
		}
		// Validate Classification (Optional) <<< ADDED THIS CALL
		if err := validateOptionalClassification(spec.Classification, taskDesc); err != nil { // Assumes helper exists
			return err
		}

	} else { // --- Embedded task specific checks ---
		// Ensure standalone-only fields are ABSENT
		if isNonEmpty(spec.APIVersion) {
			return fmt.Errorf("%s: must not contain api-version", taskDesc)
		}
		if spec.Metadata != nil {
			return fmt.Errorf("%s: must not contain metadata section", taskDesc)
		}
		if len(spec.SupportedPlatformVersions) > 0 {
			return fmt.Errorf("%s: must not contain supported-platform-versions", taskDesc)
		}
		// Type, if present, must be "task" (checked post-defaulting in plugin validation)
		if isNonEmpty(spec.Type) && spec.Type != SpecTypeTask {
			return fmt.Errorf("%s: if type is specified, it must be '%s', got: '%s'", taskDesc, SpecTypeTask, spec.Type)
		}
		// ID, Name, Description are optional here (defaulted later).
		// Tags and Classification are also optional, and currently ignored/not validated for embedded tasks
		// as they are meant to be inherited. Add warnings if they *are* present?
		if spec.Tags != nil {
			log.Printf("Warning: %s: contains 'tags' field, which is ignored for embedded tasks (inherited from plugin).", taskDesc)
		}
		if spec.Classification != nil {
			log.Printf("Warning: %s: contains 'classification' field, which is ignored for embedded tasks (inherited from plugin).", taskDesc)
		}
	}

	// --- Common Task Field Checks (Required for both Standalone and Embedded) ---
	// IsEnabled is boolean, always valid parse.

	// ImageURL checks
	if !isNonEmpty(spec.ImageURL) {
		return fmt.Errorf("%s: image_url is required", taskDesc)
	}
	if !imageDigestRegex.MatchString(spec.ImageURL) {
		return fmt.Errorf("%s: image_url ('%s') must be in digest format (e.g., registry/repo@sha256:hash)", taskDesc, spec.ImageURL)
	}

	// Command checks
	if spec.Command == nil || len(spec.Command) == 0 {
		return fmt.Errorf("%s: command is required (min 1 element for executable)", taskDesc)
	}
	if !isNonEmpty(spec.Command[0]) {
		return fmt.Errorf("%s: the first element of command (executable) cannot be empty", taskDesc)
	}

	// Timeout checks
	if !isNonEmpty(spec.Timeout) {
		return fmt.Errorf("%s: timeout is required", taskDesc)
	}
	timeoutDuration, err := time.ParseDuration(spec.Timeout)
	if err != nil {
		return fmt.Errorf("%s: invalid timeout format '%s': %w", taskDesc, spec.Timeout, err)
	}
	if timeoutDuration >= (24 * time.Hour) {
		return fmt.Errorf("%s: timeout '%s' must be less than 24 hours", taskDesc, spec.Timeout)
	}
	if timeoutDuration <= 0 {
		return fmt.Errorf("%s: timeout '%s' must be positive", taskDesc, spec.Timeout)
	}

	// Scale Config checks
	sc := spec.ScaleConfig
	if !isNonEmpty(sc.LagThreshold) {
		return fmt.Errorf("%s: scale_config.lag_threshold is required", taskDesc)
	}
	lagInt, err := strconv.Atoi(sc.LagThreshold)
	if err != nil || lagInt <= 0 {
		return fmt.Errorf("%s: scale_config.lag_threshold ('%s') must be a positive integer string", taskDesc, sc.LagThreshold)
	}
	if sc.MinReplica < 0 {
		return fmt.Errorf("%s: scale_config.min_replica (%d) cannot be negative", taskDesc, sc.MinReplica)
	}
	if sc.MaxReplica < sc.MinReplica {
		return fmt.Errorf("%s: scale_config.max_replica (%d) must be >= min_replica (%d)", taskDesc, sc.MaxReplica, sc.MinReplica)
	}

	// Params & Configs presence checks (must exist, can be empty list)
	if spec.Params == nil {
		return fmt.Errorf("%s: params field is required (use [] for none)", taskDesc)
	}
	if spec.Configs == nil {
		return fmt.Errorf("%s: configs field is required (use [] for none)", taskDesc)
	}

	// Run Schedule checks
	if spec.RunSchedule == nil {
		return fmt.Errorf("%s: run_schedule field is required (min 1 entry)", taskDesc)
	}
	if len(spec.RunSchedule) < 1 {
		return fmt.Errorf("%s: run_schedule must contain at least one entry", taskDesc)
	}

	// Detailed Run Schedule Entry checks
	defaultScheduleFound := false
	paramSet := make(map[string]struct{})
	for _, p := range spec.Params {
		if !isNonEmpty(p) {
			return fmt.Errorf("%s: parameter name in top-level 'params' cannot be empty", taskDesc)
		}
		if _, exists := paramSet[p]; exists {
			return fmt.Errorf("%s: duplicate top-level parameter '%s'", taskDesc, p)
		}
		paramSet[p] = struct{}{}
	}
	scheduleIDs := make(map[string]struct{})
	for i, schedule := range spec.RunSchedule {
		entryContext := fmt.Sprintf("%s run_schedule entry %d", taskDesc, i)
		if !isNonEmpty(schedule.ID) {
			return fmt.Errorf("%s: id field is required", entryContext)
		}
		entryContext = fmt.Sprintf("%s (id: '%s')", entryContext, schedule.ID) // Update context with ID
		if _, exists := scheduleIDs[schedule.ID]; exists {
			return fmt.Errorf("%s: duplicate schedule ID '%s'", entryContext, schedule.ID)
		}
		scheduleIDs[schedule.ID] = struct{}{}
		if schedule.Params == nil {
			return fmt.Errorf("%s: params map field is required (use {} for none)", entryContext)
		}
		if !isNonEmpty(schedule.Frequency) {
			return fmt.Errorf("%s: frequency field is required", entryContext)
		}
		if schedule.ID == "describe-all" || schedule.ID == "default" {
			defaultScheduleFound = true
			for requiredParam := range paramSet {
				if _, ok := schedule.Params[requiredParam]; !ok {
					return fmt.Errorf("%s: default schedule missing required parameter '%s'", entryContext, requiredParam)
				}
			}
		} else {
			for definedParam := range schedule.Params {
				if _, ok := paramSet[definedParam]; !ok {
					return fmt.Errorf("%s: defines parameter '%s' not declared in top-level 'params'", entryContext, definedParam)
				}
			}
		}
	}
	if !defaultScheduleFound && len(paramSet) > 0 {
		return fmt.Errorf("%s: task defines parameters but no run_schedule entry with id 'describe-all' or 'default' was found", taskDesc)
	}

	return nil // All checks passed
} // --- END validateTaskStructure ---
