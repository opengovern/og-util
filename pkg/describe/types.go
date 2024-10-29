package describe

import (
	"github.com/opengovern/og-util/pkg/describe/enums"
	"github.com/opengovern/og-util/pkg/integration"
	"github.com/opengovern/og-util/pkg/vault"
)

type DescribeJob struct {
	JobID        uint // DescribeResourceJob ID
	ResourceType string
	SourceID     string
	AccountID    string
	DescribedAt  int64
	SourceType   integration.Type
	CipherText   string
	TriggerType  enums.DescribeTriggerType
	RetryCounter uint
}

type DescribeWorkerInput struct {
	JobEndpoint               string `json:"jobEndpoint"`
	DeliverEndpoint           string `json:"describeEndpoint"`
	EndpointAuth              bool   `json:"describeEndpointAuth"`
	IngestionPipelineEndpoint string `json:"ingestionPipelineEndpoint"`
	UseOpenSearch             bool   `json:"useOpenSearch"`

	VaultConfig vault.Config

	DescribeJob DescribeJob `json:"describeJob"`
}

// Connector source.Type
//
//	ResourceName  string
//	ResourceLabel string
//	ServiceName   string
//
//	Tags map[string][]string
//
//	ListDescriber ResourceDescriber
//	GetDescriber  SingleResourceDescriber
//
//	TerraformName        []string
//	TerraformServiceName string
//
//	FastDiscovery bool
//	CostDiscovery bool
//	Summarize     bool
type ResourceType interface {
	GetConnector() integration.Type
	GetResourceName() string
	GetResourceLabel() string
	GetServiceName() string
	GetTags() map[string][]string
	GetTerraformName() []string
	GetTerraformServiceName() string
	IsFastDiscovery() bool
	IsCostDiscovery() bool
	IsSummarized() bool
}
