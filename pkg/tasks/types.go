package tasks

import "fmt"

type TaskDefinition struct {
	RunID    uint           `json:"runId"`
	TaskType string         `json:"taskType"`
	Params   map[string]any `json:"params"`
}

type TaskRequest struct {
	EsDeliverEndpoint         string `json:"esDeliverEndpoint"`
	IngestionPipelineEndpoint string `json:"ingestionPipelineEndpoint"`
	UseOpenSearch             bool   `json:"useOpenSearch"`

	TaskDefinition TaskDefinition `json:"taskDefinition"`

	ExtraInputs map[string][]string `json:"extraInputs"`
}

func GetTaskRunCancelSubject(natsTopicName string, runId uint) string {
	return fmt.Sprintf("cancel.%s.%d", natsTopicName, runId)
}
