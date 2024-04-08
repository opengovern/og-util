package describe

import (
	"github.com/kaytu-io/kaytu-util/pkg/describe/enums"
	"github.com/kaytu-io/kaytu-util/pkg/source"
	"github.com/kaytu-io/kaytu-util/pkg/vault"
)

type DescribeJob struct {
	JobID           uint // DescribeResourceJob ID
	ScheduleJobID   uint
	ParentJobID     uint // DescribeSourceJob ID
	ResourceType    string
	SourceID        string
	AccountID       string
	DescribedAt     int64
	SourceType      source.Type
	CipherText      string
	VaultKeyVersion string
	TriggerType     enums.DescribeTriggerType
	RetryCounter    uint
}

type DescribeWorkerInput struct {
	WorkspaceId               string `json:"workspaceId"`
	WorkspaceName             string `json:"workspaceName"`
	DescribeEndpoint          string `json:"describeEndpoint"`
	IngestionPipelineEndpoint string `json:"ingestionPipelineEndpoint"`
	UseOpenSearch             bool   `json:"useOpenSearch"`
	KeyARN                    string `json:"keyARN"`
	KeyRegion                 string `json:"keyRegion"`

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
	GetConnector() source.Type
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
