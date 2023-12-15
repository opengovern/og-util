package koanf

import "time"

type Redis struct {
	Address string `koanf:"address"`
}

type ElasticSearch struct {
	Address      string `koanf:"address"`
	Username     string `koanf:"username"`
	Password     string `koanf:"password"`
	IsOpenSearch bool   `koanf:"is_open_search"`
	AwsRegion    string `koanf:"aws_region"`
}

type Postgres struct {
	Host     string `koanf:"host"`
	Port     string `koanf:"port"`
	DB       string `koanf:"db"`
	Username string `koanf:"username"`
	Password string `koanf:"password"`
	SSLMode  string `koanf:"ssl_mode"`

	MaxIdelConns    int           `koanf:"max_idel_conns"`
	MaxOpenConns    int           `koanf:"max_open_conns"`
	ConnMaxIdleTime time.Duration `koanf:"conn_max_idle_time"`
	ConnMaxLifetime time.Duration `koanf:"conn_max_lifetime"`
}

type KMS struct {
	ARN    string `koanf:"arn"`
	Region string `koanf:"region"`
}

type KaytuService struct {
	BaseURL string `koanf:"base_url"`
}

type HttpServer struct {
	Address string `koanf:"address"`
}

type RabbitMQ struct {
	Service  string `koanf:"service"`
	Username string `koanf:"username"`
	Password string `koanf:"password"`
}

type Vault struct {
	Address string `koanf:"address"`
	Role    string `koanf:"role"`
	Token   string `koanf:"token"`
	CaPath  string `koanf:"ca_path"`
	UseTLS  bool   `koanf:"use_tls"`
}

type Kafka struct {
	Addresses string `koanf:"addresses"`
	Topic     string `koanf:"topic"`
}
