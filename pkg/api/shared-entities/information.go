package shared_entities

import "time"

// CspmUsageRequest is the request object for the call home feature data gathering
type CspmUsageRequest struct {
	InstallId       string    `json:"install_id"`
	GatherTimestamp time.Time `json:"gather_timestamp"`

	Hostname             string         `json:"hostname"`
	NumberOfUsers        int64          `json:"number_of_users"`
	IntegrationTypeCount map[string]int `json:"integration_type_count"`
}
