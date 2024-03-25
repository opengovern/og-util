package steampipe

import (
	"strings"
)

var AzureADKeys = map[string]struct{}{
	strings.ToLower("Microsoft.Resources/users"):                     {},
	strings.ToLower("Microsoft.Resources/groups"):                    {},
	strings.ToLower("Microsoft.Resources/serviceprincipals"):         {},
	strings.ToLower("Microsoft.Resources/applications"):              {},
	strings.ToLower("Microsoft.Resources/devices"):                   {},
	strings.ToLower("Microsoft.Resources/signInReports"):             {},
	strings.ToLower("Microsoft.Resources/domains"):                   {},
	strings.ToLower("Microsoft.Resources/identityproviders"):         {},
	strings.ToLower("Microsoft.Resources/securitydefaultspolicy"):    {},
	strings.ToLower("Microsoft.Resources/authorizationpolicy"):       {},
	strings.ToLower("Microsoft.Resources/conditionalaccesspolicy"):   {},
	strings.ToLower("Microsoft.Resources/directoryroles"):            {},
	strings.ToLower("Microsoft.Resources/directorysettings"):         {},
	strings.ToLower("Microsoft.Resources/directoryauditreport"):      {},
	strings.ToLower("Microsoft.Resources/adminconsentrequestpolicy"): {},
	strings.ToLower("Microsoft.Resources/userregistrationdetails"):   {},
	strings.ToLower("Microsoft.Resources/groupMemberships"):          {},
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
