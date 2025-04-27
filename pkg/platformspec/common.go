package platformspec

import (
	"errors"
	"fmt"
	"log"

	"github.com/Masterminds/semver/v3"
)

// CheckPlatformSupport checks platform compatibility using an already validated PluginSpecification.
func (v *defaultValidator) CheckPlatformSupport(pluginSpec *PluginSpecification, platformVersion string) (bool, error) {
	if pluginSpec == nil {
		return false, errors.New("plugin specification cannot be nil for platform support check")
	}
	// Assume pluginSpec is already structurally validated by ProcessSpecification
	if !isNonEmpty(platformVersion) {
		return false, errors.New("platformVersion cannot be empty for platform support check")
	}

	// Parse the current platform version
	currentV, err := semver.NewVersion(platformVersion)
	if err != nil {
		return false, fmt.Errorf("invalid platform version format '%s': %w", platformVersion, err)
	}

	supportedVersions := pluginSpec.Plugin.SupportedPlatformVersions
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
			log.Printf("Platform version '%s' matches constraint '%s' for plugin '%s'.", platformVersion, constraintStr, pluginSpec.Plugin.Name)
			return true, nil // Found a matching constraint
		}
	}

	// If no constraint matched
	log.Printf("Platform version '%s' does not satisfy any supported-platform-versions constraints %v for plugin '%s'.",
		platformVersion, supportedVersions, pluginSpec.Plugin.Name)
	return false, nil
}
