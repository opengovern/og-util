package config

import "time"

type Redis struct {
	Address string
}

type ElasticSearch struct {
	Address           string
	Username          string
	Password          string
	IsOpenSearch      bool
	AwsRegion         string
	AssumeRoleArn     string `yaml:"assume_role_arn"`
	ExternalID        string `yaml:"external_id"`
	IngestionEndpoint string `yaml:"ingestion_endpoint"`
}

type Postgres struct {
	Host     string
	Port     string
	DB       string
	Username string
	Password string
	SSLMode  string

	MaxIdelConns    int
	MaxOpenConns    int
	ConnMaxIdleTime time.Duration
	ConnMaxLifetime time.Duration
}

type KMS struct {
	ARN    string
	Region string
}

type KaytuService struct {
	BaseURL string
}

type HttpServer struct {
	Address string
}

type RabbitMQ struct {
	Service  string
	Username string
	Password string
}

type Vault struct {
	Address string
	Role    string
	Token   string
	CaPath  string
	UseTLS  bool
}

type Kafka struct {
	Addresses string
	Topic     string
}
