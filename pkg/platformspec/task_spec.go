//task_spec.go

package platformspec

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"gopkg.in/yaml.v3"
)

// processTaskSpec handles the parsing and validation specific to standalone task specifications.
// It's called by ProcessSpecification in validator.go.
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
		return nil, fmt.Errorf("task specification '%s': api-version must be '%s' (or omitted to default), got '%s'", filePath, APIVersionV1, originalAPIVersion)
	}
	// Ensure type is set correctly (should be 'task' from base parse)
	if !isNonEmpty(spec.Type) {
		spec.Type = SpecTypeTask // Default if somehow missing after base parse
	} else if spec.Type != SpecTypeTask {
		return nil, fmt.Errorf("task specification '%s': type must be '%s', got '%s'", filePath, SpecTypeTask, spec.Type)
	}

	log.Println("Validating task specification structure (standalone)...")
	// Pass true for isStandalone check
	if err := v.validateTaskStructure(&spec, true); err != nil {
		return nil, fmt.Errorf("standalone task specification structure validation failed: %w", err)
	}
	log.Println("Standalone task specification structure validation successful.")

	// Task Image Validation (optional)
	if !skipArtifactValidation && isNonEmpty(spec.ImageURL) {
		log.Println("Initiating standalone task image validation...")
		// Assumes validateImageManifestExists exists in artifact_validation.go
		err := v.validateImageManifestExists(spec.ImageURL)
		if err != nil {
			return nil, fmt.Errorf("standalone task image validation failed for '%s': %w", spec.ImageURL, err)
		}
		log.Println("Standalone task image validation successful.")
	} else {
		log.Println("Skipping standalone task image validation (image_url empty or validation skipped).")
	}
	return &spec, nil
}

// GetTaskDefinition reads a specification file specifically expecting a 'task' type and parses it.
// Prefer ProcessSpecification for a unified approach.
func (v *defaultValidator) GetTaskDefinition(filePath string) (*TaskSpecification, error) {
	log.Printf("Loading standalone task definition from: %s", filePath)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file '%s': %w", filePath, err)
	}

	var base BaseSpecification
	if err := yaml.Unmarshal(data, &base); err != nil {
		return nil, fmt.Errorf("failed to parse base fields from '%s' (invalid YAML?): %w", filePath, err)
	}
	if strings.ToLower(base.Type) != SpecTypeTask {
		return nil, fmt.Errorf("expected specification type '%s' but got '%s' in file '%s'", SpecTypeTask, base.Type, filePath)
	}

	var spec TaskSpecification
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("failed to parse specification file '%s' as task (check syntax): %w", filePath, err)
	}

	// Apply API version default if needed
	if !isNonEmpty(spec.APIVersion) {
		if !isNonEmpty(base.APIVersion) { // If base also didn't have it
			spec.APIVersion = APIVersionV1
			log.Printf("Info: Standalone task specification '%s' missing 'api-version', defaulting to '%s'.", filePath, APIVersionV1)
		} else {
			spec.APIVersion = base.APIVersion // Should already be v1 if defaulted
		}
	}
	// Ensure type is correct
	if !isNonEmpty(spec.Type) {
		spec.Type = SpecTypeTask
	} else if strings.ToLower(spec.Type) != SpecTypeTask {
		return nil, fmt.Errorf("consistency check failed: expected specification type '%s' but parsed '%s' in file '%s'", SpecTypeTask, spec.Type, filePath)
	}

	// Perform structure validation for standalone task (includes metadata checks)
	log.Println("Validating standalone task specification structure...")
	if err := v.validateTaskStructure(&spec, true); err != nil {
		return nil, fmt.Errorf("standalone task structure validation failed: %w", err)
	}
	log.Printf("Successfully loaded and validated standalone task definition for ID: %s", spec.ID)
	return &spec, nil
}

// validateTaskStructure performs structural checks specific to 'task' specifications.
// The isStandalone flag determines if APIVersion, Metadata, and SupportedPlatformVersions are required (true)
// or must be absent (false, for embedded discovery tasks).
// For embedded tasks (isStandalone=false), ID, Name, Description, and Type are optional here, defaulting happens in validatePluginStructure.
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
			taskDesc = "standalone task (ID missing)" // ID is required for standalone
		}
	} else {
		// For embedded, ID/Name might be missing at this stage
		if isNonEmpty(spec.ID) {
			taskDesc = fmt.Sprintf("embedded discovery task (ID: %s)", spec.ID)
		} else if isNonEmpty(spec.Name) {
			taskDesc = fmt.Sprintf("embedded discovery task (Name: %s, ID/Name default later)", spec.Name)
		} else {
			taskDesc = "embedded discovery task (ID/Name default later)"
		}
	}

	// --- Standalone vs Embedded Field Requirements ---
	if isStandalone {
		// These fields are REQUIRED for standalone tasks.
		if !isNonEmpty(spec.APIVersion) || spec.APIVersion != APIVersionV1 {
			// This check might be redundant if ProcessSpecification already defaulted/checked, but keep for direct GetTaskDefinition calls
			return fmt.Errorf("%s: api-version is required and must be '%s' (or omitted to default), got: '%s'", taskDesc, APIVersionV1, spec.APIVersion)
		}
		if spec.Metadata == nil {
			return fmt.Errorf("%s: metadata section is required for standalone task", taskDesc)
		}
		// Validate the metadata content using the helper
		// Assumes validateMetadata exists in metadata_validation.go
		if err := v.validateMetadata(spec.Metadata, fmt.Sprintf("%s metadata", taskDesc)); err != nil {
			return err // Error already contextualized
		}
		if len(spec.SupportedPlatformVersions) == 0 {
			return fmt.Errorf("%s: supported-platform-versions requires at least one constraint entry for standalone task", taskDesc)
		}
		for i, constraintStr := range spec.SupportedPlatformVersions {
			if !isNonEmpty(constraintStr) {
				return fmt.Errorf("%s: supported-platform-versions entry %d cannot be empty for standalone task", taskDesc, i)
			}
			if _, err := semver.NewConstraint(constraintStr); err != nil {
				return fmt.Errorf("%s: supported-platform-versions entry %d ('%s') is not a valid semantic version constraint for standalone task: %w", taskDesc, i, constraintStr, err)
			}
		}
		// Standalone tasks MUST have ID, Name, Description, and Type specified
		if !isNonEmpty(spec.ID) {
			return fmt.Errorf("%s: id is required for standalone task", taskDesc)
		}
		if !isNonEmpty(spec.Name) {
			return fmt.Errorf("%s: name is required for standalone task", taskDesc)
		}
		if !isNonEmpty(spec.Description) {
			return fmt.Errorf("%s: description is required for standalone task", taskDesc)
		}
		if !isNonEmpty(spec.Type) || spec.Type != SpecTypeTask {
			return fmt.Errorf("%s: type is required and must be '%s' for standalone task, got: '%s'", taskDesc, SpecTypeTask, spec.Type)
		}
		// Standalone tasks MUST have a non-empty command list with a non-empty first element
		if spec.Command == nil || len(spec.Command) == 0 {
			return fmt.Errorf("%s: command is required and cannot be empty for standalone task", taskDesc)
		}
		if !isNonEmpty(spec.Command[0]) {
			return fmt.Errorf("%s: the first element of the command list (the executable) cannot be empty for standalone task", taskDesc)
		}

	} else { // Embedded task specific checks
		// These fields MUST NOT be present for embedded discovery tasks (they are inherited from the plugin).
		if isNonEmpty(spec.APIVersion) {
			return fmt.Errorf("%s: must not contain api-version (it's inherited from plugin), but found: '%s'", taskDesc, spec.APIVersion)
		}
		if spec.Metadata != nil {
			return fmt.Errorf("%s: must not contain metadata section (it's inherited from plugin)", taskDesc)
		}
		if len(spec.SupportedPlatformVersions) > 0 {
			return fmt.Errorf("%s: must not contain supported-platform-versions (it's inherited from plugin), but found: %v", taskDesc, spec.SupportedPlatformVersions)
		}
		// ID, Name, Description, Type are OPTIONAL here for embedded tasks. Defaulting happens in validatePluginStructure.
		// However, if Type *is* specified, it must be "task".
		if isNonEmpty(spec.Type) && spec.Type != SpecTypeTask {
			return fmt.Errorf("%s: if type is specified for embedded task, it must be '%s', got: '%s'", taskDesc, SpecTypeTask, spec.Type)
		}
		// Name and Description are now optional for embedded tasks, no check needed here.
		// Command list check for embedded (must not be empty if provided)
		if spec.Command != nil && len(spec.Command) == 0 {
			return fmt.Errorf("%s: command list cannot be empty if provided", taskDesc)
		}
		if spec.Command != nil && len(spec.Command) > 0 && !isNonEmpty(spec.Command[0]) {
			return fmt.Errorf("%s: the first element of the command list (the executable) cannot be empty if command is provided", taskDesc)
		}
	}

	// --- Common Task Field Checks (Required for both Standalone and Embedded, except where noted for embedded) ---
	// ID, Name, Description, Type presence/value checked above based on isStandalone flag and defaulting logic.

	// IsEnabled is a boolean, always present.

	if !isNonEmpty(spec.ImageURL) {
		return fmt.Errorf("%s: image_url is required", taskDesc)
	}
	// ** Enforce Digest Format for ImageURL **
	// Assumes imageDigestRegex is initialized in validator.go
	if !imageDigestRegex.MatchString(spec.ImageURL) {
		return fmt.Errorf("%s: image_url ('%s') must be in digest format (e.g., registry/repository/image@sha256:hash)", taskDesc, spec.ImageURL)
	}
	// Command presence check (must have at least one element)
	if spec.Command == nil || len(spec.Command) == 0 {
		return fmt.Errorf("%s: command is required and must contain at least the executable path", taskDesc)
	}
	if !isNonEmpty(spec.Command[0]) {
		return fmt.Errorf("%s: the first element of the command list (the executable) cannot be empty", taskDesc)
	}

	if !isNonEmpty(spec.Timeout) {
		return fmt.Errorf("%s: timeout is required", taskDesc)
	}
	timeoutDuration, err := time.ParseDuration(spec.Timeout)
	if err != nil {
		return fmt.Errorf("%s: invalid timeout format '%s', requires format like '5m', '1h30s': %w", taskDesc, spec.Timeout, err)
	}
	twentyFourHours := 24 * time.Hour
	if timeoutDuration >= twentyFourHours {
		return fmt.Errorf("%s: timeout '%s' must be less than 24 hours (%s)", taskDesc, spec.Timeout, twentyFourHours)
	}
	if timeoutDuration <= 0 {
		return fmt.Errorf("%s: timeout '%s' must be a positive duration", taskDesc, spec.Timeout)
	}

	// --- Scale Config Check ---
	sc := spec.ScaleConfig // Alias for readability
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
	if spec.Params == nil {
		return fmt.Errorf("%s: params field is required (use an empty list [] if no parameters)", taskDesc)
	}
	if spec.Configs == nil {
		return fmt.Errorf("%s: configs field is required (use an empty list [] if no configs)", taskDesc)
	}

	// --- Run Schedule Check ---
	if spec.RunSchedule == nil {
		return fmt.Errorf("%s: run_schedule field is required (must contain at least one entry)", taskDesc)
	}
	if len(spec.RunSchedule) < 1 {
		return fmt.Errorf("%s: run_schedule must contain at least one schedule entry", taskDesc)
	}

	// Check schedule entries and ensure default exists if params are defined
	defaultScheduleFound := false
	paramSet := make(map[string]struct{})
	for _, p := range spec.Params {
		if !isNonEmpty(p) {
			return fmt.Errorf("%s: parameter names in top-level 'params' list cannot be empty", taskDesc)
		}
		if _, exists := paramSet[p]; exists {
			return fmt.Errorf("%s: duplicate parameter name '%s' found in top-level 'params' list", taskDesc, p)
		}
		paramSet[p] = struct{}{}
	}

	scheduleIDs := make(map[string]struct{})
	for i, schedule := range spec.RunSchedule {
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
					return fmt.Errorf("%s: defines parameter '%s' which is not declared in the task's top-level 'params' list %v", entryContext, definedParam, spec.Params)
				}
			}
		}
	}

	// If the task defines parameters, a default schedule covering them is mandatory.
	if !defaultScheduleFound && len(paramSet) > 0 {
		return fmt.Errorf("%s: task defines parameters (%v), but no run_schedule entry with id 'describe-all' or 'default' was found to provide default values", taskDesc, spec.Params)
	}

	return nil
}
