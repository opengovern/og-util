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
	Azure AzureVaultConfig `yaml:"azure" json:"azure" koanf:"azure"`
	KeyId string           `yaml:"key_id" json:"key_id" koanf:"key_id"`
}

type VaultSourceConfig interface {
	Encrypt(ctx context.Context, data map[string]any, keyId string) (string, error)
	Decrypt(ctx context.Context, cypherText string, keyId string) (map[string]any, error)
}
