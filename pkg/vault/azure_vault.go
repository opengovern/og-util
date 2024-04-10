package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
	"go.uber.org/zap"
	"golang.org/x/net/context"
)

type AzureVaultConfig struct {
	TenantId     string `json:"tenant_id" yaml:"tenant_id" koanf:"tenant_id"`
	ClientId     string `json:"client_id" yaml:"client_id" koanf:"client_id"`
	ClientSecret string `json:"client_secret" yaml:"client_secret" koanf:"client_secret"`
	BaseUrl      string `json:"base_url" yaml:"base_url" koanf:"base_url"`
}

type AzureVaultSourceConfig struct {
	logger *zap.Logger
	AesKey []byte
}

func NewAzureVaultClient(ctx context.Context, logger *zap.Logger, config AzureVaultConfig, secretId string) (*AzureVaultSourceConfig, error) {
	cred, err := azidentity.NewClientSecretCredential(config.TenantId, config.ClientId, config.ClientSecret, nil)
	if err != nil {
		logger.Error("failed to create Azure Key Vault credential", zap.Error(err))
		return nil, err
	}
	client, err := azsecrets.NewClient(config.BaseUrl, cred, nil)
	if err != nil {
		logger.Error("failed to create Azure Key Vault client", zap.Error(err))
		return nil, err
	}

	secret, err := client.GetSecret(ctx, secretId, "", nil)
	if err != nil {
		logger.Error("failed to get secret", zap.Error(err))
		return nil, err
	}
	if secret.Value == nil {
		logger.Error("secret value is nil")
		return nil, errors.New("secret value is nil")
	}

	aesKey, err := base64.StdEncoding.DecodeString(*secret.Value)
	if err != nil {
		logger.Error("failed to decode secret value", zap.Error(err))
		return nil, err
	}

	sc := AzureVaultSourceConfig{
		logger: logger,
		AesKey: aesKey,
	}

	return &sc, nil
}

func (sc *AzureVaultSourceConfig) Encrypt(ctx context.Context, cred map[string]any) (string, error) {
	bytes, err := json.Marshal(cred)
	if err != nil {
		sc.logger.Error("failed to marshal the credential", zap.Error(err))
		return "", err
	}

	aesCipher, err := aes.NewCipher(sc.AesKey)
	if err != nil {
		sc.logger.Error("failed to create cipher", zap.Error(err))
		return "", err
	}
	gcm, err := cipher.NewGCM(aesCipher)
	if err != nil {
		sc.logger.Error("failed to create gcm", zap.Error(err))
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	_, err = rand.Read(nonce)
	if err != nil {
		sc.logger.Error("failed to generate nonce", zap.Error(err))
		return "", err
	}

	cipherText := gcm.Seal(nonce, nonce, bytes, nil)

	return base64.StdEncoding.EncodeToString(cipherText), nil
}

func (sc *AzureVaultSourceConfig) Decrypt(ctx context.Context, cypherText string) (map[string]any, error) {
	aesCipher, err := aes.NewCipher(sc.AesKey)
	if err != nil {
		sc.logger.Error("failed to create cipher", zap.Error(err))
		return nil, err
	}
	gcm, err := cipher.NewGCM(aesCipher)
	if err != nil {
		sc.logger.Error("failed to create gcm", zap.Error(err))
		return nil, err
	}

	decodedCipherText, err := base64.StdEncoding.DecodeString(cypherText)
	if err != nil {
		sc.logger.Error("failed to decode the cypher text", zap.Error(err))
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(decodedCipherText) < nonceSize {
		sc.logger.Error("cipher text is too short", zap.Int("length", len(decodedCipherText)), zap.Int("nonceSize", nonceSize))
		return nil, errors.New("cipher text is too short")
	}

	nonce, cipherText := decodedCipherText[:nonceSize], decodedCipherText[nonceSize:]
	plainText, err := gcm.Open(nil, nonce, cipherText, nil)
	if err != nil {
		sc.logger.Error("failed to decrypt the credential", zap.Error(err))
		return nil, err
	}

	conf := make(map[string]any)
	err = json.Unmarshal(plainText, &conf)
	if err != nil {
		sc.logger.Error("failed to unmarshal the decrypted credential", zap.Error(err))
		return nil, err
	}

	return conf, nil
}

type AzureVaultSecretHandler struct {
	logger *zap.Logger
	client *azsecrets.Client
}

func NewAzureVaultSecretHandler(logger *zap.Logger, config AzureVaultConfig) (*AzureVaultSecretHandler, error) {
	cred, err := azidentity.NewClientSecretCredential(config.TenantId, config.ClientId, config.ClientSecret, nil)
	if err != nil {
		logger.Error("failed to create Azure Key Vault credential", zap.Error(err))
		return nil, err
	}
	client, err := azsecrets.NewClient(config.BaseUrl, cred, nil)
	if err != nil {
		logger.Error("failed to create Azure Key Vault client", zap.Error(err))
		return nil, err
	}

	return &AzureVaultSecretHandler{
		logger: logger,
		client: client,
	}, err
}

func (a *AzureVaultSecretHandler) GetSecret(ctx context.Context, secretId string) (string, error) {
	secret, err := a.client.GetSecret(ctx, secretId, "", nil)
	if err != nil {
		a.logger.Error("failed to get secret", zap.Error(err))
		return "", err
	}
	if secret.Value == nil {
		a.logger.Error("secret value is nil")
		return "", errors.New("secret value is nil")
	}

	return *secret.Value, nil
}

func (a *AzureVaultSecretHandler) SetSecret(ctx context.Context, secretName string, secretValue []byte) (string, error) {
	base64SecretValue := base64.StdEncoding.EncodeToString(secretValue)
	res, err := a.client.SetSecret(ctx, secretName, azsecrets.SetSecretParameters{
		Value: &base64SecretValue,
	}, nil)
	if err != nil {
		a.logger.Error("failed to set secret", zap.Error(err))
		return "", err
	}
	if res.ID == nil {
		a.logger.Error("secret id is nil")
		return "", errors.New("secret id is nil")
	}

	return res.ID.Name(), nil
}

func (a *AzureVaultSecretHandler) DeleteSecret(ctx context.Context, secretId string) error {
	_, err := a.client.DeleteSecret(ctx, secretId, nil)
	if err != nil {
		a.logger.Error("failed to delete secret", zap.Error(err))
		return err
	}

	return nil
}
