package koanf

import "time"

type Redis struct {
	Address string `koanf:"address"`
}

type ElasticSearch struct {
	Address           string `koanf:"address"`
	Username          string `koanf:"username"`
	Password          string `koanf:"password"`
	IsOpenSearch      bool   `koanf:"is_open_search"`
	IsOnAks           bool   `koanf:"is_on_aks"`
	AWSRegion         string `koanf:"aws_region"`
	AssumeRoleARN     string `koanf:"assume_role_arn"`
	ExternalID        string `koanf:"external_id"`
	IngestionEndpoint string `koanf:"ingestion_endpoint"`
}

type Postgres struct {
	Host     string `koanf:"host"`
	Port     string `koanf:"port"`
	DB       string `koanf:"db"`
	Username string `koanf:"username"`
	Password string `koanf:"password"`
	SSLMode  string `koanf:"ssl_mode"`

	MaxIdleConns    int           `koanf:"max_idle_conns"`
	MaxOpenConns    int           `koanf:"max_open_conns"`
	ConnMaxIdleTime time.Duration `koanf:"conn_max_idle_time"`
	ConnMaxLifetime time.Duration `koanf:"conn_max_lifetime"`
}

type KMS struct {
	ARN    string `koanf:"arn"`
	Region string `koanf:"region"`
}

type OpenGovernanceService struct {
	BaseURL string `koanf:"base_url"`
}

type HttpServer struct {
	Address string `koanf:"address"`
}

type GrpcServer struct {
	Address string `koanf:"address"`
	AuthUri string `koanf:"auth_uri"`
}

type Vault struct {
	Address string `koanf:"address"`
	Role    string `koanf:"role"`
	Token   string `koanf:"token"`
	CaPath  string `koanf:"ca_path"`
	UseTLS  bool   `koanf:"use_tls"`
}

type NATS struct {
	URL          string        `koanf:"url"`
	PingInterval time.Duration `koanf:"ping_interval"`
}
