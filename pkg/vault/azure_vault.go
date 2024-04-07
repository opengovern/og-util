package vault

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/Azure/azure-sdk-for-go/services/keyvault/v7.0/keyvault"
	"github.com/Azure/go-autorest/autorest"
	utils "github.com/kaytu-io/kaytu-util/pkg/pointer"
	"go.uber.org/zap"
	"golang.org/x/net/context"
	"sort"
	"strings"
)

type AzureVaultSourceConfig struct {
	logger       *zap.Logger
	vaultClient  keyvault.BaseClient
	vaultBaseURL string
}

func NewAzureVaultClient(logger *zap.Logger, username, password string, vaultBaseUrl string) (*AzureVaultSourceConfig, error) {
	// Create a new Azure Key Vault client
	autorestAuthorizer := autorest.NewBasicAuthorizer(username, password)
	client := keyvault.New()
	client.Authorizer = autorestAuthorizer

	sc := AzureVaultSourceConfig{
		logger:       logger,
		vaultClient:  client,
		vaultBaseURL: vaultBaseUrl,
	}

	return &sc, nil
}

func (sc *AzureVaultSourceConfig) Encrypt(ctx context.Context, cred map[string]any, keyId, keyVersion string) ([]byte, error) {
	bytes, err := json.Marshal(cred)
	if err != nil {
		sc.logger.Error("failed to marshal the credential", zap.Error(err))
		return nil, err
	}

	res, err := sc.vaultClient.Encrypt(ctx, sc.vaultBaseURL, keyId, keyVersion, keyvault.KeyOperationsParameters{
		Algorithm: keyvault.RSAOAEP256,
		Value:     utils.GetPointer(string(bytes)),
	})
	if err != nil {
		sc.logger.Error("failed to encrypt the credential", zap.Error(err), zap.String("keyId", keyId), zap.String("keyVersion", keyVersion))
		return nil, err
	}
	if res.Result == nil {
		sc.logger.Error("failed to encrypt the credential - result is null", zap.String("keyId", keyId), zap.String("keyVersion", keyVersion))
		return nil, err
	}

	return []byte(*res.Result), nil
}

func (sc *AzureVaultSourceConfig) Decrypt(ctx context.Context, cypherText string, keyId, keyVersion string) (map[string]any, error) {
	res, err := sc.vaultClient.Decrypt(ctx, sc.vaultBaseURL, keyId, keyVersion, keyvault.KeyOperationsParameters{
		Algorithm: keyvault.RSAOAEP256,
		Value:     &cypherText,
	})
	if err != nil {
		sc.logger.Error("failed to decrypt the credential", zap.Error(err), zap.String("keyId", keyId), zap.String("keyVersion", keyVersion))
		return nil, err
	}
	if res.Result == nil {
		sc.logger.Error("failed to decrypt the credential - result is null", zap.String("keyId", keyId), zap.String("keyVersion", keyVersion))
		return nil, err
	}

	decodedResult, err := base64.StdEncoding.DecodeString(*res.Result)
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
	res, err := sc.vaultClient.GetKeyVersions(ctx, sc.vaultBaseURL, keyId, nil)
	if err != nil {
		sc.logger.Error("failed to get key versions", zap.Error(err), zap.String("keyId", keyId))
		return "", err
	}

	keyVersions := make([]keyvault.KeyItem, 0)

	for {
		keyVersions = append(keyVersions, res.Values()...)
		if !res.NotDone() {
			break
		}
		if err := res.NextWithContext(ctx); err != nil {
			sc.logger.Error("failed to get next key version", zap.Error(err), zap.String("keyId", keyId))
			return "", err
		}
	}

	sort.Slice(keyVersions, func(i, j int) bool {
		if keyVersions[i].Attributes.Created == nil {
			return false
		}
		if keyVersions[j].Attributes.Created == nil {
			return true
		}
		return keyVersions[i].Attributes.Created.Duration() > keyVersions[j].Attributes.Created.Duration()
	})

	if len(keyVersions) == 0 {
		sc.logger.Error("no key versions found", zap.String("keyId", keyId))
		return "", errors.New("no key versions found")
	}
	kid := keyVersions[0].Kid
	if kid == nil {
		sc.logger.Error("no key id found", zap.String("keyId", keyId))
		return "", errors.New("no key id found")
	}

	parts := strings.Split(*kid, "/")
	return parts[len(parts)-1], nil
}
