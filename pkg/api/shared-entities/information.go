package shared_entities

import "time"

// CspmUsageRequest is the request object for the call home feature data gathering
type CspmUsageRequest struct {
	GatherTimestamp time.Time `json:"gather_timestamp"`

	Hostname             string         `json:"hostname"`
	IntegrationTypeCount map[string]int `json:"integration_type_count"`
	ApproximateSpend     int            `json:"approximate_spend"`
}
