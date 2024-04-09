package vault

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azkeys"
	utils "github.com/kaytu-io/kaytu-util/pkg/pointer"
	"go.uber.org/zap"
	"golang.org/x/net/context"
	"sort"
)

type AzureVaultConfig struct {
	TenantId     string `json:"tenant_id" yaml:"tenant_id" koanf:"tenant_id"`
	ClientId     string `json:"client_id" yaml:"client_id" koanf:"client_id"`
	ClientSecret string `json:"client_secret" yaml:"client_secret" koanf:"client_secret"`
	BaseUrl      string `json:"base_url" yaml:"base_url" koanf:"base_url"`
}

type AzureVaultSourceConfig struct {
	logger      *zap.Logger
	vaultClient *azkeys.Client
}

func NewAzureVaultClient(logger *zap.Logger, config AzureVaultConfig) (*AzureVaultSourceConfig, error) {
	cred, err := azidentity.NewClientSecretCredential(config.TenantId, config.ClientId, config.ClientSecret, nil)
	if err != nil {
		logger.Error("failed to create Azure Key Vault credential", zap.Error(err))
		return nil, err
	}
	client, err := azkeys.NewClient(config.BaseUrl, cred, nil)
	if err != nil {
		logger.Error("failed to create Azure Key Vault client", zap.Error(err))
		return nil, err
	}

	sc := AzureVaultSourceConfig{
		logger:      logger,
		vaultClient: client,
	}

	return &sc, nil
}

func (sc *AzureVaultSourceConfig) Encrypt(ctx context.Context, cred map[string]any, keyId, keyVersion string) ([]byte, error) {
	bytes, err := json.Marshal(cred)
	if err != nil {
		sc.logger.Error("failed to marshal the credential", zap.Error(err))
		return nil, err
	}

	res, err := sc.vaultClient.Encrypt(ctx, keyId, keyVersion, azkeys.KeyOperationParameters{
		Algorithm: utils.GetPointer(azkeys.EncryptionAlgorithmRSAOAEP256),
		Value:     bytes,
	}, nil)
	if err != nil {
		sc.logger.Error("failed to encrypt the credential", zap.Error(err), zap.String("keyId", keyId), zap.String("keyVersion", keyVersion))
		return nil, err
	}

	return res.Result, nil
}

func (sc *AzureVaultSourceConfig) Decrypt(ctx context.Context, cypherText string, keyId, keyVersion string) (map[string]any, error) {
	res, err := sc.vaultClient.Decrypt(ctx, keyId, keyVersion, azkeys.KeyOperationParameters{
		Algorithm: utils.GetPointer(azkeys.EncryptionAlgorithmRSAOAEP256),
		Value:     []byte(cypherText),
	}, nil)
	if err != nil {
		sc.logger.Error("failed to decrypt the credential", zap.Error(err), zap.String("keyId", keyId), zap.String("keyVersion", keyVersion))
		return nil, err
	}
	if res.Result == nil {
		sc.logger.Error("failed to decrypt the credential - result is null", zap.String("keyId", keyId), zap.String("keyVersion", keyVersion))
		return nil, err
	}

	decodedResult, err := base64.StdEncoding.DecodeString(string(res.Result))
	if err != nil {
		sc.logger.Error("failed to decode the decrypted credential", zap.Error(err), zap.String("keyId", keyId), zap.String("keyVersion", keyVersion))
		return nil, err
	}

	conf := make(map[string]any)
	err = json.Unmarshal(decodedResult, &conf)
	if err != nil {
		sc.logger.Error("failed to unmarshal the decrypted credential", zap.Error(err), zap.String("keyId", keyId), zap.String("keyVersion", keyVersion))
		return nil, err
	}

	return conf, nil
}

func (sc *AzureVaultSourceConfig) GetLatestVersion(ctx context.Context, keyId string) (string, error) {
	pager := sc.vaultClient.NewListKeyPropertiesVersionsPager(keyId, nil)
	keyVersions := make([]*azkeys.KeyProperties, 0)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			sc.logger.Error("failed to get key versions", zap.Error(err), zap.String("keyId", keyId))
			return "", err
		}
		for _, props := range page.Value {
			if props == nil {
				continue
			}
			keyVersions = append(keyVersions, props)
		}
	}

	sort.Slice(keyVersions, func(i, j int) bool {
		if keyVersions[i].Attributes.Created == nil {
			return false
		}
		if keyVersions[j].Attributes.Created == nil {
			return true
		}
		return keyVersions[i].Attributes.Created.After(*keyVersions[j].Attributes.Created)
	})

	if len(keyVersions) == 0 {
		sc.logger.Error("no key versions found", zap.String("keyId", keyId))
		return "", errors.New("no key versions found")
	}
	kid := keyVersions[0].KID
	if kid == nil {
		sc.logger.Error("no key id found", zap.String("keyId", keyId))
		return "", errors.New("no key id found")
	}

	return kid.Version(), nil
}
