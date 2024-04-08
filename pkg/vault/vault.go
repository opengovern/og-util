package vault

import "context"

type Provider string

const (
	AwsKMS        Provider = "aws-kms"
	AzureKeyVault Provider = "azure-keyvault"
)

type Config struct {
	Provider Provider `yaml:"provider" json:"provider"`
	Aws      struct {
		Region    string `yaml:"region" json:"region"`
		RoleArn   string `yaml:"role_arn" json:"role_arn"`
		AccessKey string `yaml:"access_key" json:"access_key"`
		SecretKey string `yaml:"secret_key" json:"secret_key"`
	} `yaml:"aws"`
	Azure struct {
		BaseUrl      string `yaml:"base_url" json:"base_url"`
		ClientId     string `yaml:"client_id" json:"client_id"`
		ClientSecret string `yaml:"client_secret" json:"client_secret"`
	} `yaml:"azure"`
}

type VaultSourceConfig interface {
	Encrypt(ctx context.Context, data map[string]any, keyId, keyVersion string) ([]byte, error)
	Decrypt(ctx context.Context, cypherText string, keyId, keyVersion string) (map[string]any, error)
	GetLatestVersion(ctx context.Context, keyId string) (string, error)
}
