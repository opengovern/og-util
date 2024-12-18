package tasks

type TaskDefinition struct {
	RunID  uint              `json:"run_id"`
	Params map[string]string `json:"params"`
}

type TaskRequest struct {
	EsDeliverEndpoint         string `json:"esDeliverEndpoint"`
	IngestionPipelineEndpoint string `json:"ingestionPipelineEndpoint"`
	UseOpenSearch             bool   `json:"useOpenSearch"`

	TaskDefinition TaskDefinition `json:"taskDefinition"`

	ExtraInputs map[string][]string `json:"extraInputs"`
}

type TaskResponse struct {
}
