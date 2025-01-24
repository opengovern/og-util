package interfaces

import (
	"github.com/hashicorp/go-plugin"
	"github.com/opengovern/og-util/pkg/integration"
	"net/rpc"
)

var HandshakeConfig = plugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "platform-integration-plugin",
	MagicCookieValue: "integration",
}

type IntegrationConfiguration struct {
	NatsScheduledJobsTopic   string
	NatsManualJobsTopic      string
	NatsStreamName           string
	NatsConsumerGroup        string
	NatsConsumerGroupManuals string

	SteampipePluginName string

	UISpec   []byte
	Manifest []byte
	SetupMD  []byte

	DescriberDeploymentName string
	DescriberRunCommand     string
}

type CloudQLColumn struct {
	Name string
	Type string
}

type IntegrationType interface {
	GetIntegrationType() (integration.Type, error)
	GetConfiguration() (IntegrationConfiguration, error)
	GetResourceTypesByLabels(map[string]string) ([]ResourceTypeConfiguration, error)
	HealthCheck(jsonData []byte, providerId string, labels map[string]string, annotations map[string]string) (bool, error)
	DiscoverIntegrations(jsonData []byte) ([]integration.Integration, error)
	GetResourceTypeFromTableName(tableName string) (string, error)
	ListAllTables() (map[string][]CloudQLColumn, error)
	Ping() error
}

// IntegrationCreator IntegrationType interface, credentials, error
type IntegrationCreator func() IntegrationType

type IntegrationTypeRPC struct {
	client *rpc.Client
}

func (i *IntegrationTypeRPC) GetIntegrationType() (integration.Type, error) {
	var integrationType integration.Type
	err := i.client.Call("Plugin.GetIntegrationType", struct{}{}, &integrationType)
	if err != nil {
		return "", err
	}
	return integrationType, nil
}

func (i *IntegrationTypeRPC) GetConfiguration() (IntegrationConfiguration, error) {
	var configuration IntegrationConfiguration
	err := i.client.Call("Plugin.GetConfiguration", struct{}{}, &configuration)
	if err != nil {
		return IntegrationConfiguration{}, err
	}
	return configuration, nil
}

func (i *IntegrationTypeRPC) GetResourceTypesByLabels(labels map[string]string) ([]ResourceTypeConfiguration, error) {
	var resourceTypes []ResourceTypeConfiguration
	err := i.client.Call("Plugin.GetResourceTypesByLabels", labels, &resourceTypes)
	if err != nil {
		return nil, err
	}
	return resourceTypes, err
}

type HealthCheckRequest struct {
	JsonData    []byte
	ProviderId  string
	Labels      map[string]string
	Annotations map[string]string
}

func (i *IntegrationTypeRPC) HealthCheck(jsonData []byte, providerId string, labels map[string]string, annotations map[string]string) (bool, error) {
	var result bool
	err := i.client.Call("Plugin.HealthCheck", HealthCheckRequest{
		JsonData:    jsonData,
		ProviderId:  providerId,
		Labels:      labels,
		Annotations: annotations,
	}, &result)
	if err != nil {
		return false, err
	}
	return result, err
}

func (i *IntegrationTypeRPC) DiscoverIntegrations(jsonData []byte) ([]integration.Integration, error) {
	var integrations []integration.Integration
	err := i.client.Call("Plugin.DiscoverIntegrations", jsonData, &integrations)
	return integrations, err
}

func (i *IntegrationTypeRPC) GetResourceTypeFromTableName(tableName string) (string, error) {
	var resourceType string
	err := i.client.Call("Plugin.GetResourceTypeFromTableName", tableName, &resourceType)
	if err != nil {
		return "", err
	}
	return resourceType, nil
}

func (i *IntegrationTypeRPC) ListAllTables() (map[string][]CloudQLColumn, error) {
	var tables map[string][]CloudQLColumn
	err := i.client.Call("Plugin.ListAllTables", struct{}{}, &tables)
	if err != nil {
		return nil, err
	}
	return tables, nil
}

func (i *IntegrationTypeRPC) Ping() error {
	return i.client.Call("Plugin.Ping", struct{}{}, nil)
}

type IntegrationTypeRPCServer struct {
	Impl IntegrationType
}

func (i *IntegrationTypeRPCServer) GetIntegrationType(_ struct{}, integrationType *integration.Type) error {
	var err error
	*integrationType, err = i.Impl.GetIntegrationType()
	return err
}

func (i *IntegrationTypeRPCServer) GetConfiguration(_ struct{}, configuration *IntegrationConfiguration) error {
	var err error
	*configuration, err = i.Impl.GetConfiguration()
	return err
}

func (i *IntegrationTypeRPCServer) GetResourceTypesByLabels(labels map[string]string, resourceTypes *[]ResourceTypeConfiguration) error {
	var err error
	*resourceTypes, err = i.Impl.GetResourceTypesByLabels(labels)
	return err
}

func (i *IntegrationTypeRPCServer) HealthCheck(request HealthCheckRequest, result *bool) error {
	var err error
	*result, err = i.Impl.HealthCheck(request.JsonData, request.ProviderId, request.Labels, request.Annotations)
	return err
}

func (i *IntegrationTypeRPCServer) DiscoverIntegrations(jsonData []byte, integrations *[]integration.Integration) error {
	var err error
	*integrations, err = i.Impl.DiscoverIntegrations(jsonData)
	return err
}

func (i *IntegrationTypeRPCServer) GetResourceTypeFromTableName(tableName string, resourceType *string) error {
	var err error
	*resourceType, err = i.Impl.GetResourceTypeFromTableName(tableName)
	return err
}

func (i *IntegrationTypeRPCServer) ListAllTables(_ struct{}, tables *map[string][]CloudQLColumn) error {
	var err error
	*tables, err = i.Impl.ListAllTables()
	return err
}

func (i *IntegrationTypeRPCServer) Ping(_ struct{}, _ *struct{}) error {
	return i.Impl.Ping()
}

type IntegrationTypePlugin struct {
	Impl IntegrationType
}

func (p *IntegrationTypePlugin) Server(*plugin.MuxBroker) (any, error) {
	return &IntegrationTypeRPCServer{Impl: p.Impl}, nil
}

func (IntegrationTypePlugin) Client(b *plugin.MuxBroker, c *rpc.Client) (any, error) {
	return &IntegrationTypeRPC{client: c}, nil
}
