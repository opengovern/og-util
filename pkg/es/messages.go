package es

import (
	"fmt"
	"github.com/opengovern/og-util/pkg/integration"
	"regexp"
	"sort"
	"strings"
)

const (
	InventorySummaryIndex = "inventory_summary"
)

type ResourceSummaryType string

type Tag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type Resource struct {
	EsID    string `json:"es_id"`
	EsIndex string `json:"es_index"`

	// PlatformID is the unique Global ID of the resource inside the platform
	PlatformID string `json:"platform_id"`
	// ResourceID is the unique ID of the resource in the integration.
	ResourceID string `json:"resource_id"`
	// ResourceName is the name of the resource.
	ResourceName string `json:"resource_name"`
	// Description is the description of the resource based on the describe call.
	Description interface{} `json:"Description"`
	// IntegrationType is the type of the integration source of the resource, i.e. AWS Cloud, Azure Cloud.
	IntegrationType integration.Type `json:"integration_type"`
	// ResourceType is the type of the resource.
	ResourceType string `json:"resource_type"`
	// IntegrationID is the integration ID that the resource belongs to
	IntegrationID string `json:"integration_id"`
	// Metadata is arbitrary data associated with each resource
	Metadata map[string]string `json:"metadata"`
	// CanonicalTags is the list of tags associated with the resource
	CanonicalTags []Tag `json:"canonical_tags"`
	// DescribedBy is the resource describe job id
	DescribedBy string `json:"described_by"`
	// DescribedAt is when the DescribeSourceJob is created
	DescribedAt int64 `json:"described_at"`
}

func (r Resource) KeysAndIndex() ([]string, string) {
	return []string{
		r.ResourceID,
		r.IntegrationID,
	}, ResourceTypeToESIndex(r.ResourceType)
}

type LookupResourceMetadata struct {
	// Parameters parameters passed to describe job (converted map[string]string to string using ConvertMapToString function)
	Parameters string `json:"parameters"`
}

type LookupResource struct {
	EsID    string `json:"es_id"`
	EsIndex string `json:"es_index"`

	// PlatformID is the unique Global ID of the resource inside the platform
	PlatformID string `json:"platform_id"`
	// ResourceID is the globally unique ID of the resource.
	ResourceID string `json:"resource_id"`
	// ResourceName is the name of the resource.
	ResourceName string `json:"resource_name"`
	// IntegrationType is the type of the integration source of the resource, i.e. AWS Cloud, Azure Cloud.
	IntegrationType integration.Type `json:"integration_type"`
	// ResourceType is the type of the resource.
	ResourceType string `json:"resource_type"`
	// IntegrationID is aws account id or azure subscription id
	IntegrationID string `json:"integration_id"`
	// IsCommon
	IsCommon bool `json:"is_common"`
	// Tags
	Tags []Tag `json:"canonical_tags"`
	// Metadata describe job metadata
	Metadata LookupResourceMetadata `json:"metadata"`
	// DescribedBy is the resource describe job id
	DescribedBy string `json:"described_by"`
	// DescribedAt is when the DescribeSourceJob is created
	DescribedAt int64 `json:"described_at"`
}

// ConvertMapToString converts a map[string]string to a deterministic string representation.
func ConvertMapToString(input map[string]string) string {
	// Create a slice of keys
	keys := make([]string, 0, len(input))
	for k := range input {
		keys = append(keys, k)
	}

	// Sort the keys to ensure deterministic order
	sort.Strings(keys)

	// Build the string
	var sb strings.Builder
	for _, key := range keys {
		sb.WriteString(fmt.Sprintf("%s=%s;", key, input[key]))
	}

	return sb.String()
}

func (r LookupResource) KeysAndIndex() ([]string, string) {
	return []string{
		r.ResourceID,
		r.IntegrationID,
		string(r.IntegrationType),
		strings.ToLower(r.ResourceType),
		r.Metadata.Parameters,
	}, InventorySummaryIndex
}

var stopWordsRe = regexp.MustCompile(`\W+`)

func ResourceTypeToESIndex(t string) string {
	t = stopWordsRe.ReplaceAllString(t, "_")
	return strings.ToLower(t)
}

type TaskResult struct {
	EsID    string `json:"es_id"`
	EsIndex string `json:"es_index"`

	PlatformID   string            `json:"platform_id"`
	ResourceID   string            `json:"resource_id"`
	ResourceName string            `json:"resource_name"`
	Description  interface{}       `json:"Description"`
	TaskType     string            `json:"task_type"`
	ResultType   string            `json:"result_type"`
	Metadata     map[string]string `json:"metadata"`
	DescribedBy  string            `json:"described_by"`
	DescribedAt  int64             `json:"described_at"`
}

func (r TaskResult) KeysAndIndex() ([]string, string) {
	return []string{
		r.ResourceID,
		r.TaskType,
		r.ResultType,
	}, ResourceTypeToESIndex(r.ResultType)
}
