package steampipe

import (
	"strings"
)

var AzureADKeys = map[string]struct{}{
	strings.ToLower("Microsoft.Entra/users"):                     {},
	strings.ToLower("Microsoft.Entra/groups"):                    {},
	strings.ToLower("Microsoft.Entra/serviceprincipals"):         {},
	strings.ToLower("Microsoft.Entra/applications"):              {},
	strings.ToLower("Microsoft.Entra/devices"):                   {},
	strings.ToLower("Microsoft.Entra/signInReports"):             {},
	strings.ToLower("Microsoft.Entra/domains"):                   {},
	strings.ToLower("Microsoft.Entra/identityproviders"):         {},
	strings.ToLower("Microsoft.Entra/securitydefaultspolicy"):    {},
	strings.ToLower("Microsoft.Entra/authorizationpolicy"):       {},
	strings.ToLower("Microsoft.Entra/conditionalaccesspolicy"):   {},
	strings.ToLower("Microsoft.Entra/directoryroles"):            {},
	strings.ToLower("Microsoft.Entra/directorysettings"):         {},
	strings.ToLower("Microsoft.Entra/directoryauditreport"):      {},
	strings.ToLower("Microsoft.Entra/adminconsentrequestpolicy"): {},
	strings.ToLower("Microsoft.Entra/userregistrationdetails"):   {},
	strings.ToLower("Microsoft.Entra/groupmemberships"):          {},
	strings.ToLower("Microsoft.Entra/appregistrations"):          {},
	strings.ToLower("Microsoft.Entra/enterpriseApplication"):     {},
	strings.ToLower("Microsoft.Entra/managedIdentity"):           {},
	strings.ToLower("Microsoft.Entra/microsoftApplication"):      {},
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
