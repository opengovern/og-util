// specification_types.go
package platformspec

import (
	"fmt"

	"gopkg.in/yaml.v3" // Ensure yaml.v3 is imported
	// Removed "log" import as debug line is removed
)

// --- Custom Type for Flexible Tag/IntegrationType Values ---

// StringOrSlice is a slice of strings that can be unmarshalled from
// either a single YAML string or a YAML sequence of strings.
type StringOrSlice []string

// UnmarshalYAML implements the yaml.Unmarshaler interface for StringOrSlice.
// This allows fields like 'tags' or 'integration_type' to accept either
// a single string or a list of strings in the YAML.
func (s *StringOrSlice) UnmarshalYAML(node *yaml.Node) error {
	// Removed Debug log line
	// log.Printf("DEBUG: StringOrSlice.UnmarshalYAML called - Node Kind: %v, Tag: %s, Value: %q", node.Kind, node.Tag, node.Value)

	if node.Kind == yaml.ScalarNode && node.Tag == "!!str" {
		// Handle single string value
		if node.Value == "" {
			// Treat explicitly empty string as empty slice? Or error?
			// Let's treat as empty slice for flexibility, like !!null.
			*s = StringOrSlice{}
			return nil
		}
		*s = StringOrSlice{node.Value} // Wrap the non-empty string in a slice
		return nil
	}
	if node.Kind == yaml.SequenceNode {
		// Handle sequence of strings
		var multi []string
		err := node.Decode(&multi) // Decode sequence into a standard string slice
		if err != nil {
			// Check specifically for non-string elements in the sequence
			for _, itemNode := range node.Content {
				// Allow !!null within sequences? Let's disallow for now unless needed.
				// Allow empty strings "" within sequences? Let's disallow for now.
				if itemNode.Kind != yaml.ScalarNode || itemNode.Tag != "!!str" || itemNode.Value == "" {
					// Added check for empty string value within sequence
					return fmt.Errorf("cannot unmarshal YAML sequence element (kind %v, tag %s, value %q) into non-empty string within StringOrSlice", itemNode.Kind, itemNode.Tag, itemNode.Value)
				}
			}
			// If the loop didn't find a specific non-string/empty item, return the original decode error
			return fmt.Errorf("failed to decode YAML sequence into []string for StringOrSlice: %w", err)
		}
		// Check for empty strings within the successfully decoded slice
		// (Redundant if checked above, but safe)
		// for i, item := range multi {
		//     if item == "" {
		//        return fmt.Errorf("empty string at index %d is not allowed in StringOrSlice sequence", i)
		//    }
		// }
		*s = StringOrSlice(multi) // Assign the decoded slice
		return nil
	}
	// Handle explicit null as empty slice
	if node.Kind == yaml.ScalarNode && node.Tag == "!!null" {
		*s = StringOrSlice{} // Assign empty slice for null input
		return nil
	}

	return fmt.Errorf("cannot unmarshal YAML node (kind %v, tag %s) into StringOrSlice", node.Kind, node.Tag)
}

// --- BaseSpecification, Component, Metadata (Unchanged from your 'current' version) ---
type BaseSpecification struct {
	APIVersion string `yaml:"api_version"`
	Type       string `yaml:"type"`
	ID         string `yaml:"id"`
}

type Component struct {
	URI           string `yaml:"uri,omitempty" json:"uri,omitempty"`
	ImageURI      string `yaml:"image_uri,omitempty" json:"image_uri,omitempty"` // Deprecated
	PathInArchive string `yaml:"path_in_archive,omitempty" json:"path_in_archive,omitempty"`
	Checksum      string `yaml:"checksum,omitempty" json:"checksum,omitempty"`
}

type Metadata struct {
	Author        string `yaml:"author" json:"author"`
	PublishedDate string `yaml:"published_date" json:"published_date"`
	Contact       string `yaml:"contact" json:"contact"`
	License       string `yaml:"license" json:"license"`
	Description   string `yaml:"description,omitempty" json:"description,omitempty"`
	Website       string `yaml:"website,omitempty" json:"website,omitempty"`
}

// --- Plugin Specific Structs ---
type DiscoveryComponent struct {
	TaskID   string             `yaml:"task_id,omitempty" json:"task_id,omitempty"`
	TaskSpec *TaskSpecification `yaml:"task_spec,omitempty" json:"task_spec,omitempty"`
}

type PluginComponents struct {
	Discovery      DiscoveryComponent `yaml:"discovery" json:"discovery"`
	PlatformBinary Component          `yaml:"platform_binary" json:"platform_binary"`
	CloudQLBinary  Component          `yaml:"cloudql_binary" json:"cloudql_binary"`
}

type PluginSpecification struct {
	APIVersion string `yaml:"api_version"`
	Type       string `yaml:"type"`

	Name                      string                   `yaml:"name"`
	Version                   string                   `yaml:"version"`
	SupportedPlatformVersions []string                 `yaml:"supported_platform_versions"`
	Metadata                  Metadata                 `yaml:"metadata"`
	Components                PluginComponents         `yaml:"components"`
	SampleData                *Component               `yaml:"sample_data,omitempty"`
	Tags                      map[string]StringOrSlice `yaml:"tags,omitempty"`           // Using StringOrSlice
	Classification            [][]string               `yaml:"classification,omitempty"` // <<< Ensure Present & Optional
}

// --- Task Specific Structs ---
type ScaleConfig struct {
	LagThreshold string `yaml:"lag_threshold" json:"lag_threshold"`
	MinReplica   int    `yaml:"min_replica" json:"min_replica"`
	MaxReplica   int    `yaml:"max_replica" json:"max_replica"`
}

type RunScheduleEntry struct {
	ID        string            `yaml:"id" json:"id"`
	Params    map[string]string `yaml:"params" json:"params"`
	Frequency string            `yaml:"frequency" json:"frequency"`
}

type TaskSpecification struct {
	APIVersion                string    `yaml:"api_version,omitempty"`
	Metadata                  *Metadata `yaml:"metadata,omitempty"`
	SupportedPlatformVersions []string  `yaml:"supported_platform_versions,omitempty"`

	ID             string                   `yaml:"id,omitempty"`
	Name           string                   `yaml:"name,omitempty"`
	Description    string                   `yaml:"description,omitempty"`
	IsEnabled      bool                     `yaml:"is_enabled"`
	Type           string                   `yaml:"type,omitempty"`
	ImageURL       string                   `yaml:"image_url"`
	Command        []string                 `yaml:"command"`
	Timeout        string                   `yaml:"timeout"`
	ScaleConfig    ScaleConfig              `yaml:"scale_config"`
	Params         []string                 `yaml:"params"`
	Configs        []interface{}            `yaml:"configs"`
	RunSchedule    []RunScheduleEntry       `yaml:"run_schedule"`
	Tags           map[string]StringOrSlice `yaml:"tags,omitempty"`           // Using StringOrSlice
	Classification [][]string               `yaml:"classification,omitempty"` // <<< Ensure Present & Optional

}

type TaskDetails struct {
	TaskID                    string
	TaskName                  string
	TaskDescription           string
	ValidatedImageURI         string
	Command                   []string
	Timeout                   string
	ScaleConfig               ScaleConfig
	Params                    []string
	Configs                   []interface{}
	RunSchedule               []RunScheduleEntry
	PluginName                string
	APIVersion                string
	SupportedPlatformVersions []string
	Metadata                  Metadata
	IsReference               bool                     `json:"is_reference"`
	ReferencedTaskID          string                   `json:"referenced_task_id,omitempty"`
	Tags                      map[string]StringOrSlice `json:"tags,omitempty"`           // Using StringOrSlice
	Classification            [][]string               `json:"classification,omitempty"` // <<< Ensure Present

}

// --- Query Specific Structs ---
type QueryParameter struct {
	Key   string `yaml:"key"`
	Value string `yaml:"value"`
}

type QuerySpecification struct {
	APIVersion string `yaml:"api_version"` // Defaults to v1 if omitted via processing logic
	Type       string `yaml:"type"`        // Must be 'query'
	ID         string `yaml:"id"`          // Required

	Title           string                   `yaml:"title"`                      // Required
	Description     string                   `yaml:"description,omitempty"`      // Optional
	IntegrationType StringOrSlice            `yaml:"integration_type,omitempty"` // *** UPDATED TYPE + omitempty ***
	Query           string                   `yaml:"query"`                      // Required
	PrimaryTable    string                   `yaml:"primary_table,omitempty"`    // Optional
	Metadata        map[string]string        `yaml:"metadata,omitempty"`         // Optional
	IsView          bool                     `yaml:"is_view"`                    // Optional, defaults false
	Parameters      []QueryParameter         `yaml:"parameters"`                 // Optional, defaults empty slice
	Tags            map[string]StringOrSlice `yaml:"tags,omitempty"`             // Optional, Using StringOrSlice
	Classification  [][]string               `yaml:"classification,omitempty"`   // Optional

	DetectedParams []string `yaml:"-" json:"-"` // Internal field
}

// --- Control Specific Structs (Placeholder) ---
type ControlSpecification struct {
	APIVersion string `yaml:"api_version"`
	Type       string `yaml:"type"`
	ID         string `yaml:"id"`

	Title          string                   `yaml:"title"`
	Description    string                   `yaml:"description,omitempty"`
	Severity       string                   `yaml:"severity"`
	Frameworks     []string                 `yaml:"frameworks,omitempty"`
	LogicSource    Component                `yaml:"logic_source"`
	Parameters     map[string]interface{}   `yaml:"parameters,omitempty"`
	Tags           map[string]StringOrSlice `yaml:"tags,omitempty"`           // Using StringOrSlice
	Classification [][]string               `yaml:"classification,omitempty"` // <<< Ensure Present & Optional
}
