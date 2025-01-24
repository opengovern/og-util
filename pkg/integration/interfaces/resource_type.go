package interfaces

import (
	"github.com/opengovern/og-util/pkg/integration"
)

type ResourceTypeConfiguration struct {
	Name            string           `json:"name"`
	IntegrationType integration.Type `json:"integration_type"`
	Description     string           `json:"description"`
	Params          []Param          `json:"params"`
	Table 			string			`json:"table"`
}

type Param struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Required    bool    `json:"required"`
	Default     *string `json:"default"`
}

func (p *ResourceTypeConfiguration) IsEmpty() bool {
	return p.Name == "" && p.Description == "" && len(p.Params) == 0 && p.IntegrationType == ""
}
