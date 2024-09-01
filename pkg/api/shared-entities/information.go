package shared_entities

import "time"

// CspmUsageRequest is the request object for the call home feature data gathering
type CspmUsageRequest struct {
	WorkspaceId     string    `json:"workspace_id"`
	GatherTimestamp time.Time `json:"gather_timestamp"`

	AwsAccountCount        int `json:"aws_account_count"`
	AzureSubscriptionCount int `json:"azure_subscription_count"`
	ApproximateSpend       int `json:"approximate_spend"`
}
