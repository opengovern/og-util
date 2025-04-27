// tags.go
// Package platformspec provides utilities for loading, validating, and verifying
// various specification types (plugin, task, query, control, etc.).
package platformspec

import (
	"log"
	// Needed for init
	// Needed for init
)

// --- Exported Helper Functions ---

// GetFlattenedTags extracts tags from a validated specification object (obtained via ProcessSpecification)
// and returns them as a flat list of "key:value" strings.
// It handles types having a `Tags map[string]StringOrSlice` field.
// Returns an empty slice if the spec type is unsupported, nil, or has no tags.
// Assumes flattenTagsMap helper is defined elsewhere (e.g., common.go).
func GetFlattenedTags(spec interface{}) []string {
	if spec == nil {
		return []string{}
	}

	// Use type switch to check for known specification types with Tags field
	switch s := spec.(type) {
	case *QuerySpecification:
		// Call the internal helper function (assumed defined elsewhere)
		return flattenTagsMap(s.Tags) // Pass map[string]StringOrSlice
	case *PluginSpecification:
		// Call the internal helper function (assumed defined elsewhere)
		return flattenTagsMap(s.Tags) // Pass map[string]StringOrSlice
	case *TaskSpecification:
		// Call the internal helper function (assumed defined elsewhere)
		return flattenTagsMap(s.Tags) // Pass map[string]StringOrSlice
	case *ControlSpecification:
		// Call the internal helper function (assumed defined elsewhere)
		return flattenTagsMap(s.Tags) // Pass map[string]StringOrSlice
	default:
		// Log warning only if type is genuinely unknown/unsupported for tags
		log.Printf("Warning: GetFlattenedTags called with an unknown or unsupported specification type for tags: %T", s)
		return []string{} // Return empty slice for unknown types
	}
}
