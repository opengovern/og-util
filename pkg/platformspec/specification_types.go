// Package platformspec provides utilities for loading, validating, and verifying
// various specification types (plugin, task, query, control, etc.).
package platformspec

// BaseSpecification is used for initial YAML parsing to determine the type, ID, and API version.
type BaseSpecification struct {
	APIVersion string `yaml:"api-version"` // Optional during initial parse, defaults to v1 if empty for non-plugin types.
	Type       string `yaml:"type"`        // Required for type dispatching.
	ID         string `yaml:"id"`          // Required for most types, optional for embedded task.
}

// --- Common Component/Metadata Structs ---

// Component represents a downloadable artifact or an image reference within a specification.
type Component struct {
	URI           string `yaml:"uri,omitempty" json:"uri,omitempty"`                         // URI for downloadable artifacts (e.g., binaries, sample data)
	ImageURI      string `yaml:"image-uri,omitempty" json:"image-uri,omitempty"`             // Deprecated: TaskSpecification.ImageURL is used.
	PathInArchive string `yaml:"path-in-archive,omitempty" json:"path-in-archive,omitempty"` // Path to the specific file within a downloaded archive (if URI points to an archive)
	Checksum      string `yaml:"checksum,omitempty" json:"checksum,omitempty"`               // Checksum for verifying downloaded artifact integrity (e.g., "sha256:...")
}

// Metadata holds descriptive information common ONLY to Plugins and standalone Tasks.
// Other specification types (like Query, Control) will have different metadata structures or none at all.
type Metadata struct {
	Author        string `yaml:"author" json:"author"`                               // Required: Author of the plugin/task.
	PublishedDate string `yaml:"published-date" json:"published-date"`               // Required: Date the version was published in YYYY-MM-DD format.
	Contact       string `yaml:"contact" json:"contact"`                             // Required: Contact information (e.g., email, website).
	License       string `yaml:"license" json:"license"`                             // Required: Valid SPDX license identifier (e.g., "Apache-2.0", "MIT"). See https://spdx.org/licenses/
	Description   string `yaml:"description,omitempty" json:"description,omitempty"` // Optional: Brief description.
	Website       string `yaml:"website,omitempty" json:"website,omitempty"`         // Optional: URL to the website or repository.
}

// --- Plugin Specific Structs ---

// DiscoveryComponent defines how a plugin specifies its discovery mechanism:
// either by referencing an existing task ID or embedding a full task specification.
type DiscoveryComponent struct {
	TaskID   string             `yaml:"task-id,omitempty" json:"task-id,omitempty"`     // Reference to a standalone TaskSpecification ID. Mutually exclusive with TaskSpec.
	TaskSpec *TaskSpecification `yaml:"task-spec,omitempty" json:"task-spec,omitempty"` // Embedded TaskSpecification details. Mutually exclusive with TaskID.
}

// PluginComponents holds the different component definitions specified for a 'plugin'.
type PluginComponents struct {
	// Discovery specifies how the plugin discovers resources. Uses DiscoveryComponent struct.
	Discovery      DiscoveryComponent `yaml:"discovery" json:"discovery"`
	PlatformBinary Component          `yaml:"platform-binary" json:"platform-binary"` // Required downloadable artifact.
	CloudQLBinary  Component          `yaml:"cloudql-binary" json:"cloudql-binary"`   // Required downloadable artifact.
}

// PluginSpecification is the top-level structure for the 'plugin' type specification file.
// Fields previously under 'plugin:' are now direct fields of this struct.
type PluginSpecification struct {
	APIVersion string `yaml:"api-version"` // Required: Must be "v1".
	Type       string `yaml:"type"`        // Required: Must be "plugin".

	// --- Fields moved from nested Plugin struct ---
	Name                      string           `yaml:"name"`                        // Required: Name of the plugin. Used as default ID/name base for embedded discovery task if omitted.
	Version                   string           `yaml:"version"`                     // Required: Semantic version of the plugin (e.g., "1.2.3").
	SupportedPlatformVersions []string         `yaml:"supported-platform-versions"` // Required: List of platform version constraints (e.g., ">=1.0.0, <2.0.0").
	Metadata                  Metadata         `yaml:"metadata"`                    // Required: Metadata about the plugin.
	Components                PluginComponents `yaml:"components"`                  // Required: Defines the core functional parts of the plugin.
	SampleData                *Component       `yaml:"sample-data,omitempty"`       // Optional: Reference to downloadable sample data.
}

// --- Task Specific Structs ---

// ScaleConfig defines the scaling parameters specified for a task.
type ScaleConfig struct {
	LagThreshold string `yaml:"lag_threshold" json:"lag_threshold"` // Required: String representing a positive integer threshold for scaling.
	MinReplica   int    `yaml:"min_replica" json:"min_replica"`     // Required: Minimum number of task replicas (>= 0).
	MaxReplica   int    `yaml:"max_replica" json:"max_replica"`     // Required: Maximum number of task replicas (>= MinReplica).
}

// RunScheduleEntry defines a single scheduled run configuration specified for a task.
type RunScheduleEntry struct {
	ID        string            `yaml:"id" json:"id"`               // Required: Unique identifier for the schedule entry (e.g., "daily-report", "default").
	Params    map[string]string `yaml:"params" json:"params"`       // Required: Parameters specific to this scheduled run. Must cover required top-level params if ID is "default".
	Frequency string            `yaml:"frequency" json:"frequency"` // Required: How often the task should run (format depends on scheduler implementation, e.g., cron string, interval).
}

// TaskSpecification defines the structure for a task, used standalone or embedded within a DiscoveryComponent.
type TaskSpecification struct {
	// --- Fields used ONLY by Standalone Task Specifications ---
	APIVersion                string    `yaml:"api-version,omitempty"`                 // Required for standalone (defaults to v1 if omitted). Must be ABSENT for embedded.
	Metadata                  *Metadata `yaml:"metadata,omitempty"`                    // Required for standalone. Must be ABSENT for embedded.
	SupportedPlatformVersions []string  `yaml:"supported-platform-versions,omitempty"` // Required for standalone. Must be ABSENT for embedded.

	// --- Common Task Fields (Standalone and Embedded) ---
	ID          string             `yaml:"id,omitempty"`          // Optional for embedded (defaults to plugin name + "-task"). Required for standalone.
	Name        string             `yaml:"name,omitempty"`        // Optional for embedded (defaults to plugin name + "-task"). Required for standalone.
	Description string             `yaml:"description,omitempty"` // Optional for embedded (defaults to plugin name + " Task"). Required for standalone.
	IsEnabled   bool               `yaml:"is_enabled"`            // Required.
	Type        string             `yaml:"type,omitempty"`        // Optional for embedded (defaults to "task"). Required ("task") for standalone.
	ImageURL    string             `yaml:"image_url"`             // Required (digest format).
	Command     []string           `yaml:"command"`               // Required: Command and args (Docker exec form: ["/executable", "arg1", "arg2"]).
	Timeout     string             `yaml:"timeout"`               // Required (< 24h).
	ScaleConfig ScaleConfig        `yaml:"scale_config"`          // Required.
	Params      []string           `yaml:"params"`                // Required (can be empty []).
	Configs     []interface{}      `yaml:"configs"`               // Required (can be empty []).
	RunSchedule []RunScheduleEntry `yaml:"run_schedule"`          // Required (min 1 entry).
}

// TaskDetails holds extracted and validated details for a specific task,
// typically retrieved via GetTaskDetailsFromPluginSpecification.
// Includes fields inherited from the parent PluginSpecification for context.
// Will contain an error if the discovery component was a task-id reference.
type TaskDetails struct {
	// Fields specific to the task definition
	TaskID            string             // The unique ID of the task (includes defaults if embedded).
	TaskName          string             // The human-readable name of the task (includes defaults if embedded).
	TaskDescription   string             // The description of the task (includes defaults if embedded).
	ValidatedImageURI string             // The container image URI, validated for format and registry existence.
	Command           []string           // The command executed by the task container (Docker exec form).
	Timeout           string             // The execution timeout duration string.
	ScaleConfig       ScaleConfig        // The task's scaling configuration.
	Params            []string           // List of expected parameter names.
	Configs           []interface{}      // List of configuration items.
	RunSchedule       []RunScheduleEntry // List of scheduled runs.

	// Fields inherited from the parent PluginSpecification
	PluginName                string   // Name of the plugin this task belongs to.
	APIVersion                string   // API version from the parent plugin specification.
	SupportedPlatformVersions []string // Supported platform versions from the parent plugin.
	Metadata                  Metadata // Metadata from the parent plugin.

	// Flag indicating if the details came from a reference
	IsReference      bool   // True if discovery used task-id, meaning details above are incomplete.
	ReferencedTaskID string // The ID used in task-id if IsReference is true.
}

// --- Query Specific Structs ---

// QueryParameter defines the structure for an entry in the 'parameters' list of a query.
type QueryParameter struct {
	Key   string `yaml:"key"`   // Required: Name of the parameter used in the query template.
	Value string `yaml:"value"` // Required: Default value for the parameter. Can be empty string "".
	// Add Description string `yaml:"description,omitempty"` if needed later.
}

// QuerySpecification represents a 'query' type specification.
type QuerySpecification struct {
	APIVersion string `yaml:"api-version"` // Defaults to v1 if omitted.
	Type       string `yaml:"type"`        // Must be 'query'.
	ID         string `yaml:"id"`          // Required.

	// --- Query Specific Fields ---
	Title           string              `yaml:"title"`                    // Required: Human-readable title.
	Description     string              `yaml:"description,omitempty"`    // Optional: More detailed description.
	IntegrationType []string            `yaml:"integration_type"`         // Required: List of applicable integration types (e.g., ["aws_cloud_account"]). Cannot be empty.
	Query           string              `yaml:"query"`                    // Required: The actual query text (e.g., SQL). Cannot be empty.
	PrimaryTable    string              `yaml:"primary_table,omitempty"`  // Optional: Main table the query targets.
	Metadata        map[string]string   `yaml:"metadata,omitempty"`       // Optional: Key/value pairs (e.g., reasoning, value). Defaults nil. Keys/values must be non-empty if present.
	IsView          bool                `yaml:"is_view"`                  // Optional: Defaults to false.
	Parameters      []QueryParameter    `yaml:"parameters"`               // Optional: List of parameters. Defaults []. Key/Value required if parameter entry exists.
	Tags            map[string][]string `yaml:"tags,omitempty"`           // Optional: Key/value list pairs. Defaults nil. Keys & list values must be non-empty if present.
	Classification  [][]string          `yaml:"classification,omitempty"` // Optional: List of string lists. Defaults nil. Inner lists & strings must be non-empty if present.

	// --- Derived Data (Not from YAML) ---
	DetectedParams []string `yaml:"-" json:"-"` // Parameters detected within the Query string (e.g., {{.ParamName}})
}

// --- Control Specific Structs (Placeholder) ---

// ControlSpecification represents a future 'control' type specification.
type ControlSpecification struct {
	APIVersion string `yaml:"api-version"` // Defaults to v1 if omitted.
	Type       string `yaml:"type"`        // Must be 'control'.
	ID         string `yaml:"id"`          // Required.
	// --- Control Specific Fields ---
	Title       string                 `yaml:"title"` // Likely required
	Description string                 `yaml:"description,omitempty"`
	Severity    string                 `yaml:"severity"` // e.g., "high", "medium"
	Frameworks  []string               `yaml:"frameworks,omitempty"`
	LogicSource Component              `yaml:"logic-source"` // Reference to where the control logic lives
	Parameters  map[string]interface{} `yaml:"parameters,omitempty"`
	// ... other control-specific fields ...
}
