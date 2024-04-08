package vault

import "context"

type Provider string

const (
	AwsKMS        Provider = "aws-kms"
	AzureKeyVault Provider = "azure-keyvault"
)

type Config struct {
	Provider Provider `yaml:"provider" json:"provider" koanf:"provider"`
	Aws      struct {
		Region    string `yaml:"region" json:"region" koanf:"region"`
		RoleArn   string `yaml:"role_arn" json:"role_arn" koanf:"role_arn"`
		AccessKey string `yaml:"access_key" json:"access_key" koanf:"access_key"`
		SecretKey string `yaml:"secret_key" json:"secret_key" koanf:"secret_key"`
	} `yaml:"aws" json:"aws" koanf:"aws"`
	Azure struct {
		BaseUrl      string `yaml:"base_url" json:"base_url" koanf:"base_url"`
		ClientId     string `yaml:"client_id" json:"client_id" koanf:"client_id"`
		ClientSecret string `yaml:"client_secret" json:"client_secret" koanf:"client_secret"`
	} `yaml:"azure" json:"azure" koanf:"azure"`
	KeyId string `yaml:"key_id" json:"key_id" koanf:"key_id"`
}

type VaultSourceConfig interface {
	Encrypt(ctx context.Context, data map[string]any, keyId, keyVersion string) ([]byte, error)
	Decrypt(ctx context.Context, cypherText string, keyId, keyVersion string) (map[string]any, error)
	GetLatestVersion(ctx context.Context, keyId string) (string, error)
}