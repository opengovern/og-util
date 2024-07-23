package vault

import "context"

type Provider string

const (
	AwsKMS         Provider = "aws-kms"
	AzureKeyVault  Provider = "azure-keyvault"
	HashiCorpVault Provider = "hashicorp-vault"
)

type Config struct {
	Provider  Provider         `yaml:"provider" json:"provider" koanf:"provider"`
	Aws       AwsVaultConfig   `yaml:"aws" json:"aws" koanf:"aws"`
	Azure     AzureVaultConfig `yaml:"azure" json:"azure" koanf:"azure"`
	HashiCorp HashiCorpConfig  `yaml:"hashicorp" json:"hashicorp" koanf:"hashicorp"`
	KeyId     string           `yaml:"key_id" json:"key_id" koanf:"key_id"`
}

type VaultSourceConfig interface {
	Encrypt(ctx context.Context, data map[string]any) (string, error)
	Decrypt(ctx context.Context, cypherText string) (map[string]any, error)
}
