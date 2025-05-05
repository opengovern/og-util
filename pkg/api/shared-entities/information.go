package shared_entities

import (
	"time"
)

// CspmUsageRequest is the request object for the call home feature data gathering
type CspmUsageRequest struct {
	InstallId       string    `json:"install_id"`
	GatherTimestamp time.Time `json:"gather_timestamp"`

	Hostname             string         `json:"hostname"`
	NumberOfUsers        int64          `json:"number_of_users"`
	IntegrationTypeCount map[string]int `json:"integration_type_count"`
}

type UsageTrackerPluginInfo struct {
	Name             string `json:"name"`
	Version          string `json:"version"`
	IntegrationCount int    `json:"integration_count"`
}

type UsageTrackerRequest struct {
	InstanceID      string                   `json:"instance_id"`
	Time            time.Time                `json:"time"`
	Version         string                   `json:"version"`
	Hostname        string                   `json:"hostname"`
	IsSsoConfigured bool                     `json:"is_sso_configured"`
	UserCount       int64                    `json:"user_count"`
	ApiKeyCount     int64                    `json:"api_key_count"`
	Plugins         []UsageTrackerPluginInfo `json:"plugins"`
}
