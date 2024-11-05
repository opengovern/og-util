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

	ExtraInputs map[string][]string `json:"extraInputs"`
}

type ResourceType interface {
	GetIntegrationType() integration.Type
	GetResourceName() string
	GetTags() map[string][]string
}
