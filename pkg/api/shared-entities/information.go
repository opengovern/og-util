package shared_entities

import "time"

// CspmUsageRequest is the request object for the call home feature data gathering
type CspmUsageRequest struct {
	WorkspaceId               string    `json:"workspace_id"`
	AwsOrganizationRootEmails []string  `json:"aws_organization_root_emails"`
	AwsAccountCount           int       `json:"aws_account_count"`
	AzureAdPrimaryDomains     []string  `json:"azure_ad_primary_domains"`
	AzureSubscriptionCount    int       `json:"azure_subscription_count"`
	Users                     []string  `json:"users"`
	GatherTimestamp           time.Time `json:"gather_timestamp"`
}
