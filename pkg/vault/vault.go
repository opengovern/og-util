package vault

import "context"

type VaultSourceConfig interface {
	Encrypt(ctx context.Context, data map[string]any, keyId, keyVersion string) ([]byte, error)
	Decrypt(ctx context.Context, cypherText string, keyId, keyVersion string) (map[string]any, error)
	GetLatestVersion(ctx context.Context, keyId string) (string, error)
}
