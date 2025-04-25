package pluginvalidation

// --- Struct Definitions ---
// These structs map directly to the YAML/JSON manifest structure.

// Component represents a single functional part of the plugin.
type Component struct {
	URI           string `yaml:"uri,omitempty" json:"uri,omitempty"`
	ImageURI      string `yaml:"image-uri,omitempty" json:"image-uri,omitempty"`
	PathInArchive string `yaml:"path-in-archive,omitempty" json:"path-in-archive,omitempty"`
	// Checksum for verifying file integrity (e.g., "sha256:<hex_hash>")
	Checksum string `yaml:"checksum,omitempty" json:"checksum,omitempty"`
}

// Metadata holds descriptive information about the plugin.
type Metadata struct {
	Author        string `yaml:"author" json:"author"`
	PublishedDate string `yaml:"published-date" json:"published-date"`
	Description   string `yaml:"description,omitempty" json:"description,omitempty"`
	Website       string `yaml:"website,omitempty" json:"website,omitempty"`
	License       string `yaml:"license,omitempty" json:"license,omitempty"`
}

// Plugin defines the core details of the plugin.
type Plugin struct {
	Name                      string           `yaml:"name" json:"name"`
	Version                   string           `yaml:"version" json:"version"`
	SupportedPlatformVersions []string         `yaml:"supported-platform-versions" json:"supported-platform-versions"`
	Metadata                  Metadata         `yaml:"metadata" json:"metadata"`
	Components                PluginComponents `yaml:"components" json:"components"`
	SampleData                *Component       `yaml:"sample-data,omitempty" json:"sample-data,omitempty"`
}

// PluginComponents holds the different component definitions.
type PluginComponents struct {
	Discovery      Component `yaml:"discovery" json:"discovery"`
	PlatformBinary Component `yaml:"platform-binary" json:"platform-binary"`
	CloudQLBinary  Component `yaml:"cloudql-binary" json:"cloudql-binary"`
}

// PluginManifest is the top-level structure for the manifest file.
type PluginManifest struct {
	APIVersion string `yaml:"api-version" json:"api-version"`
	Type       string `yaml:"type" json:"type"`
	Plugin     Plugin `yaml:"plugin" json:"plugin"`
}
