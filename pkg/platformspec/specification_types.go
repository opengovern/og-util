// specification_types.go
package platformspec

import (
	"fmt"
	"log"

	"gopkg.in/yaml.v3" // Ensure yaml.v3 is imported
)

// --- Custom Type for Flexible Tag Values ---

// StringOrSlice is a slice of strings that can be unmarshalled from
// either a single YAML string or a YAML sequence of strings.
type StringOrSlice []string

// UnmarshalYAML implements the yaml.Unmarshaler interface for StringOrSlice.
// This allows the 'tags' field to accept either a single string or a list of strings in the YAML.

// UnmarshalYAML implements the yaml.Unmarshaler interface for StringOrSlice.
func (s *StringOrSlice) UnmarshalYAML(node *yaml.Node) error {
	// *** ADD THIS LINE FOR DEBUGGING ***
	log.Printf("DEBUG: StringOrSlice.UnmarshalYAML called - Node Kind: %v, Tag: %s, Value: %q", node.Kind, node.Tag, node.Value)

	if node.Kind == yaml.ScalarNode && node.Tag == "!!str" {
		// Handle single string value
		*s = StringOrSlice{node.Value} // Wrap the string in a slice
		return nil
	}
	if node.Kind == yaml.SequenceNode {
		// Handle sequence of strings
		var multi []string
		err := node.Decode(&multi) // Decode sequence into a standard string slice
		if err != nil {
			// Provide more context on sequence decoding failure
			for _, itemNode := range node.Content {
				if itemNode.Kind != yaml.ScalarNode || itemNode.Tag != "!!str" {
					return fmt.Errorf("cannot unmarshal YAML sequence element (kind %v, tag %s) into string within StringOrSlice", itemNode.Kind, itemNode.Tag)
				}
			}
			return fmt.Errorf("failed to decode YAML sequence into []string for StringOrSlice: %w", err)
		}
		*s = StringOrSlice(multi) // Assign the decoded slice
		return nil
	}
	if node.Kind == yaml.ScalarNode && node.Tag == "!!null" {
		*s = StringOrSlice{}
		return nil
	}

	return fmt.Errorf("cannot unmarshal YAML node (kind %v, tag %s) into StringOrSlice", node.Kind, node.Tag)
}

// --- BaseSpecification, Component, Metadata (Unchanged) ---
type BaseSpecification struct {
	APIVersion string `yaml:"api-version"`
	Type       string `yaml:"type"`
	ID         string `yaml:"id"`
}

type Component struct {
	URI           string `yaml:"uri,omitempty" json:"uri,omitempty"`
	ImageURI      string `yaml:"image-uri,omitempty" json:"image-uri,omitempty"` // Deprecated
	PathInArchive string `yaml:"path-in-archive,omitempty" json:"path-in-archive,omitempty"`
	Checksum      string `yaml:"checksum,omitempty" json:"checksum,omitempty"`
}

type Metadata struct {
	Author        string `yaml:"author" json:"author"`
	PublishedDate string `yaml:"published-date" json:"published-date"`
	Contact       string `yaml:"contact" json:"contact"`
	License       string `yaml:"license" json:"license"`
	Description   string `yaml:"description,omitempty" json:"description,omitempty"`
	Website       string `yaml:"website,omitempty" json:"website,omitempty"`
}

// --- Plugin Specific Structs ---
type DiscoveryComponent struct {
	TaskID   string             `yaml:"task-id,omitempty" json:"task-id,omitempty"`
	TaskSpec *TaskSpecification `yaml:"task-spec,omitempty" json:"task-spec,omitempty"`
}

type PluginComponents struct {
	Discovery      DiscoveryComponent `yaml:"discovery" json:"discovery"`
	PlatformBinary Component          `yaml:"platform-binary" json:"platform-binary"`
	CloudQLBinary  Component          `yaml:"cloudql-binary" json:"cloudql-binary"`
}

// PluginSpecification: Update Tags field type
type PluginSpecification struct {
	APIVersion string `yaml:"api-version"`
	Type       string `yaml:"type"`

	Name                      string                   `yaml:"name"`
	Version                   string                   `yaml:"version"`
	SupportedPlatformVersions []string                 `yaml:"supported-platform-versions"`
	Metadata                  Metadata                 `yaml:"metadata"`
	Components                PluginComponents         `yaml:"components"`
	SampleData                *Component               `yaml:"sample-data,omitempty"`
	Tags                      map[string]StringOrSlice `yaml:"tags,omitempty"` // *** UPDATED TYPE ***
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

// TaskSpecification: Update Tags field type
type TaskSpecification struct {
	APIVersion                string    `yaml:"api-version,omitempty"`
	Metadata                  *Metadata `yaml:"metadata,omitempty"`
	SupportedPlatformVersions []string  `yaml:"supported-platform-versions,omitempty"`

	ID          string                   `yaml:"id,omitempty"`
	Name        string                   `yaml:"name,omitempty"`
	Description string                   `yaml:"description,omitempty"`
	IsEnabled   bool                     `yaml:"is_enabled"`
	Type        string                   `yaml:"type,omitempty"`
	ImageURL    string                   `yaml:"image_url"`
	Command     []string                 `yaml:"command"`
	Timeout     string                   `yaml:"timeout"`
	ScaleConfig ScaleConfig              `yaml:"scale_config"`
	Params      []string                 `yaml:"params"`
	Configs     []interface{}            `yaml:"configs"`
	RunSchedule []RunScheduleEntry       `yaml:"run_schedule"`
	Tags        map[string]StringOrSlice `yaml:"tags,omitempty"` // *** UPDATED TYPE ***

}

// TaskDetails (Unchanged)
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
	Tags                      map[string]StringOrSlice `yaml:"tags,omitempty"` // *** UPDATED TYPE ***

}

// --- Query Specific Structs ---
type QueryParameter struct {
	Key   string `yaml:"key"`
	Value string `yaml:"value"`
}

// QuerySpecification: Update Tags field type
type QuerySpecification struct {
	APIVersion string `yaml:"api-version"`
	Type       string `yaml:"type"`
	ID         string `yaml:"id"`

	Title           string                   `yaml:"title"`
	Description     string                   `yaml:"description,omitempty"`
	IntegrationType []string                 `yaml:"integration_type"`
	Query           string                   `yaml:"query"`
	PrimaryTable    string                   `yaml:"primary_table,omitempty"`
	Metadata        map[string]string        `yaml:"metadata,omitempty"`
	IsView          bool                     `yaml:"is_view"`
	Parameters      []QueryParameter         `yaml:"parameters"`
	Tags            map[string]StringOrSlice `yaml:"tags,omitempty"` // *** UPDATED TYPE ***
	Classification  [][]string               `yaml:"classification,omitempty"`

	DetectedParams []string `yaml:"-" json:"-"`
}

// --- Control Specific Structs (Placeholder) ---

// ControlSpecification: Update Tags field type
type ControlSpecification struct {
	APIVersion string `yaml:"api-version"`
	Type       string `yaml:"type"`
	ID         string `yaml:"id"`

	Title       string                   `yaml:"title"`
	Description string                   `yaml:"description,omitempty"`
	Severity    string                   `yaml:"severity"`
	Frameworks  []string                 `yaml:"frameworks,omitempty"`
	LogicSource Component                `yaml:"logic-source"`
	Parameters  map[string]interface{}   `yaml:"parameters,omitempty"`
	Tags        map[string]StringOrSlice `yaml:"tags,omitempty"` // *** UPDATED TYPE ***
	// ... other control-specific fields ...
}
