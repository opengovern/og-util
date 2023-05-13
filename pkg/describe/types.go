package describe

import (
	"github.com/kaytu-io/kaytu-util/pkg/describe/enums"
	"github.com/kaytu-io/kaytu-util/pkg/source"
)

type DescribeJob struct {
	JobID         uint // DescribeResourceJob ID
	ScheduleJobID uint
	ParentJobID   uint // DescribeSourceJob ID
	ResourceType  string
	SourceID      string
	AccountID     string
	DescribedAt   int64
	SourceType    source.Type
	CipherText    string
	TriggerType   enums.DescribeTriggerType
	RetryCounter  uint
}

type LambdaDescribeWorkerInput struct {
	WorkspaceId      string      `json:"workspaceId"`
	WorkspaceName    string      `json:"workspaceName"`
	DescribeEndpoint string      `json:"describeEndpoint"`
	KeyARN           string      `json:"keyARN"`
	KeyRegion        string      `json:"keyRegion"`
	DescribeJob      DescribeJob `json:"describeJob"`
}
