package source

import (
	"fmt"
	"strings"
)

type AssetDiscoveryMethodType string

const (
	AssetDiscoveryMethodTypeScheduled AssetDiscoveryMethodType = "scheduled"
)

type HealthStatus string

const (
	HealthStatusNil       HealthStatus = ""
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
)

func ParseHealthStatus(str string) (HealthStatus, error) {
	switch strings.ToLower(str) {
	case "healthy":
		return HealthStatusHealthy, nil
	case "unhealthy":
		return HealthStatusUnhealthy, nil
	default:
		return HealthStatusNil, fmt.Errorf("invalid health status: %s", str)
	}
}

type SourceCreationMethod string

const (
	SourceCreationMethodManual      SourceCreationMethod = "manual"
	SourceCreationMethodAutoOnboard SourceCreationMethod = "auto-onboard"
)

type ConnectorDirectionType string

const (
	ConnectorDirectionTypeIngress ConnectorDirectionType = "ingress"
	ConnectorDirectionTypeEgress  ConnectorDirectionType = "egress"
	ConnectorDirectionTypeBoth    ConnectorDirectionType = "both"
)

type ConnectorStatus string

const (
	ConnectorStatusEnabled    ConnectorStatus = "enabled"
	ConnectorStatusDisabled   ConnectorStatus = "disabled"
	ConnectorStatusComingSoon ConnectorStatus = "coming_soon"
)
