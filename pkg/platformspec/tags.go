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
// It currently supports *QuerySpecification and potentially other types if they are added
// with a `Tags map[string][]string` field.
// Returns an empty slice if the spec type is unsupported, nil, or has no tags.
func GetFlattenedTags(spec interface{}) []string {
	if spec == nil {
		return []string{}
	}

	// Use type switch to check for known specification types with Tags field
	switch s := spec.(type) {
	case *QuerySpecification:
		// Call the internal helper function (assumed to be in common.go)
		return flattenTagsMap(s.Tags)
	case *PluginSpecification:
		// Example: Plugins currently don't have a top-level Tags field in the provided structs.
		// If they did (e.g., s.Tags map[string][]string), you would add:
		// return flattenTagsMap(s.Tags)
		log.Printf("Warning: GetFlattenedTags called with *PluginSpecification, which currently has no standard Tags field.")
		return []string{}
	case *TaskSpecification:
		// Example: Tasks currently don't have a top-level Tags field.
		log.Printf("Warning: GetFlattenedTags called with *TaskSpecification, which currently has no standard Tags field.")
		return []string{}
	case *ControlSpecification:
		// Example: Controls might have tags in the future.
		// If they did (e.g., s.Tags map[string][]string), you would add:
		// return flattenTagsMap(s.Tags)
		log.Printf("Warning: GetFlattenedTags called with *ControlSpecification, which currently has no standard Tags field.")
		return []string{}
	default:
		log.Printf("Warning: GetFlattenedTags called with an unknown or unsupported specification type: %T", s)
		return []string{} // Return empty slice for unknown types
	}
}
