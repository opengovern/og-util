package steampipe

import (
	"strings"
)

var AzureADKeys = map[string]struct{}{
	strings.ToLower("Microsoft.Resources/users"):             {},
	strings.ToLower("Microsoft.Resources/groups"):            {},
	strings.ToLower("Microsoft.Resources/serviceprincipals"): {},
}

type SteampipePlugin string

const (
	SteampipePluginAWS     = "aws"
	SteampipePluginAzure   = "azure"
	SteampipePluginAzureAD = "azuread"
	SteampipePluginUnknown = ""
)

func ExtractPlugin(resourceType string) SteampipePlugin {
	resourceType = strings.ToLower(resourceType)
	if strings.HasPrefix(resourceType, "aws::") {
		return SteampipePluginAWS
	} else if strings.HasPrefix(resourceType, "microsoft") {
		if _, ok := AzureADKeys[strings.ToLower(resourceType)]; ok {
			return SteampipePluginAzureAD
		}
		return SteampipePluginAzure
	}
	return SteampipePluginUnknown
}
