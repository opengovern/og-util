package vault

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	vault "github.com/hashicorp/vault/api"
	kubernetesAuth "github.com/hashicorp/vault/api/auth/kubernetes"
	"go.uber.org/zap"
	"k8s.io/client-go/rest"
	"path"
	"strings"
)

const (
	secretMountPath    = "secrets"
	keyMapKey          = "key"
	vaultRoleName      = "workspace-creds-manager"
	kubernetesAuthPath = "auth/kubernetes"
)

type HashiCorpConfig struct {
	Address string `json:"address" yaml:"address" koanf:"address"`
}

type HashiCorpVaultSourceConfig struct {
	logger *zap.Logger
	AesKey []byte
}

func newHashiCorpCredential(ctx context.Context, logger *zap.Logger, config HashiCorpConfig, doAuth bool) (*vault.Client, error) {
	vaultCfg := vault.DefaultConfig()
	vaultCfg.Address = config.Address
	if err := vaultCfg.ConfigureTLS(&vault.TLSConfig{Insecure: true}); err != nil {
		logger.Error("failed to configure TLS", zap.Error(err))
		return nil, err
	}

	client, err := vault.NewClient(vaultCfg)
	if err != nil {
		logger.Error("failed to create HashiCorp Vault client", zap.Error(err))
		return nil, err
	}

	if doAuth {
		k8sAuth, err := kubernetesAuth.NewKubernetesAuth(
			vaultRoleName,
		)
		if err != nil {
			logger.Error("failed to create Kubernetes auth", zap.Error(err))
			return nil, err
		}

		authInfo, err := client.Auth().Login(ctx, k8sAuth)
		if err != nil {
			logger.Error("failed to login", zap.Error(err))
			return nil, err
		}
		if authInfo == nil {
			return nil, fmt.Errorf("authInfo is nil")
		}
	}

	return client, nil
}

func NewHashiCorpVaultClient(ctx context.Context, logger *zap.Logger, config HashiCorpConfig, secretId string) (*HashiCorpVaultSourceConfig, error) {
	client, err := newHashiCorpCredential(ctx, logger, config, true)
	if err != nil {
		logger.Error("failed to create HashiCorp Vault client", zap.Error(err))
		return nil, err
	}

	secret, err := client.KVv2(secretMountPath).Get(ctx, secretId)
	if err != nil {
		logger.Error("failed to get secret", zap.Error(err))
		return nil, err
	}
	if secret.Data == nil || secret.Data[keyMapKey] == nil {
		logger.Error("secret value is nil")
		return nil, errors.New("secret value is nil")
	}
	if _, ok := secret.Data[keyMapKey].(string); !ok {
		logger.Error("secret value is not a string")
		return nil, errors.New("secret value is not a string")
	}

	aesKey, err := base64.StdEncoding.DecodeString(secret.Data[keyMapKey].(string))
	if err != nil {
		logger.Error("failed to decode secret value", zap.Error(err))
		return nil, err
	}

	sc := HashiCorpVaultSourceConfig{
		logger: logger,
		AesKey: aesKey,
	}

	return &sc, nil
}

func (sc *HashiCorpVaultSourceConfig) Encrypt(ctx context.Context, cred map[string]any) (string, error) {
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

func (sc *HashiCorpVaultSourceConfig) Decrypt(ctx context.Context, cypherText string) (map[string]any, error) {
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

type HashiCorpVaultSecretHandler struct {
	logger *zap.Logger
	client *vault.Client
}

func NewHashiCorpVaultSecretHandler(ctx context.Context, logger *zap.Logger, config HashiCorpConfig) (*HashiCorpVaultSecretHandler, error) {
	client, err := newHashiCorpCredential(ctx, logger, config, true)
	if err != nil {
		logger.Error("failed to create HashiCorp Vault client", zap.Error(err))
		return nil, err
	}

	return &HashiCorpVaultSecretHandler{
		logger: logger,
		client: client,
	}, nil
}

func (a *HashiCorpVaultSecretHandler) GetSecret(ctx context.Context, secretId string) (string, error) {
	secret, err := a.client.KVv2(secretId).Get(ctx, secretId)
	if err != nil {
		a.logger.Error("failed to get secret", zap.Error(err))
		return "", err
	}
	if secret.Data == nil || secret.Data[keyMapKey] == nil {
		a.logger.Error("secret value is nil")
		return "", errors.New("secret value is nil")
	}
	if _, ok := secret.Data[keyMapKey].(string); !ok {
		a.logger.Error("secret value is not a string")
		return "", errors.New("secret value is not a string")
	}

	return secret.Data[keyMapKey].(string), nil
}

func (a *HashiCorpVaultSecretHandler) SetSecret(ctx context.Context, secretName string, secretValue []byte) (string, error) {
	base64SecretValue := base64.StdEncoding.EncodeToString(secretValue)
	_, err := a.client.KVv2(secretMountPath).Put(ctx, secretName, map[string]any{
		keyMapKey: base64SecretValue,
	})
	if err != nil {
		a.logger.Error("failed to set secret", zap.Error(err))
		return "", err
	}

	return secretName, nil
}

func (a *HashiCorpVaultSecretHandler) DeleteSecret(ctx context.Context, secretId string) error {
	err := a.client.KVv2(secretMountPath).Delete(ctx, secretId)
	if err != nil {
		a.logger.Error("failed to delete secret", zap.Error(err))
		return err
	}

	return nil
}

type HashiCorpVaultSealHandler struct {
	logger *zap.Logger
	client *vault.Client
}

func NewHashiCorpVaultSealHandler(ctx context.Context, logger *zap.Logger, config HashiCorpConfig) (*HashiCorpVaultSealHandler, error) {
	client, err := newHashiCorpCredential(ctx, logger, config, false)
	if err != nil {
		logger.Error("failed to create HashiCorp Vault client", zap.Error(err))
		return nil, err
	}

	return &HashiCorpVaultSealHandler{
		logger: logger,
		client: client,
	}, nil
}

func (a *HashiCorpVaultSealHandler) TryInit(ctx context.Context) (*vault.InitResponse, error) {
	isInited, err := a.client.Sys().InitStatusWithContext(ctx)
	if err != nil {
		a.logger.Error("failed to get init status", zap.Error(err))
		return nil, err
	}

	if isInited {
		return nil, nil
	}

	return a.client.Sys().InitWithContext(ctx, &vault.InitRequest{
		SecretShares:    5,
		SecretThreshold: 3,
	})
}

func (a *HashiCorpVaultSealHandler) enableKuberAuth(ctx context.Context, rootToken string) error {
	a.client.SetToken(rootToken)

	listAuthRes, err := a.client.Sys().ListAuthWithContext(ctx)
	if err != nil {
		a.logger.Error("failed to list auth", zap.Error(err))
		return err
	}
	for mountPath, authType := range listAuthRes {
		if strings.Contains(strings.ToLower(mountPath), "kubernetes") {
			return nil
		}
		if strings.Contains(strings.ToLower(authType.Type), "kubernetes") {
			return nil
		}
	}

	err = a.client.Sys().EnableAuthWithOptionsWithContext(ctx, "kubernetes", &vault.EnableAuthOptions{
		Type: "kubernetes",
	})
	if err != nil {
		a.logger.Error("failed to enable kubernetes auth", zap.Error(err))
		return err
	}

	kubernetesConfig, err := rest.InClusterConfig()
	if err != nil {
		a.logger.Error("failed to get kubernetes config", zap.Error(err))
		return err
	}

	_, err = a.client.Logical().Write("auth/kubernetes/config", map[string]any{
		"kubernetes_host": kubernetesConfig.Host,
	})
	if err != nil {
		a.logger.Error("failed to set kubernetes config", zap.Error(err))
		return err
	}

	return nil
}

func (a *HashiCorpVaultSealHandler) SetupKuberAuth(ctx context.Context, rootToken string) error {
	err := a.enableKuberAuth(ctx, rootToken)
	if err != nil {
		return err
	}

	policy := fmt.Sprintf(`
path "%s/*" {
	  capabilities = ["read", "list", "create", "update", "delete"]
}
`, secretMountPath)

	_, err = a.client.Logical().WriteWithContext(ctx, path.Join("sys/policy", vaultRoleName), map[string]any{
		"policy": policy,
	})
	if err != nil {
		a.logger.Error("failed to set policy", zap.Error(err))
		return err
	}

	_, err = a.client.Logical().WriteWithContext(ctx, path.Join(kubernetesAuthPath, "role", vaultRoleName), map[string]any{
		"bound_service_account_names":      "*",
		"bound_service_account_namespaces": "*",
		"policies":                         vaultRoleName,
		"ttl":                              "8640h",
	})
	if err != nil {
		a.logger.Error("failed to set role", zap.Error(err))
		return err
	}

	return nil
}

func (a *HashiCorpVaultSealHandler) TryUnseal(ctx context.Context, keys []string) error {
	sealStatusResponse, err := a.client.Sys().SealStatusWithContext(ctx)
	if err != nil {
		a.logger.Error("failed to get seal status", zap.Error(err))
		return err
	}
	if !sealStatusResponse.Sealed {
		return nil
	}

	for _, key := range keys {
		res, err := a.client.Sys().UnsealWithContext(ctx, key)
		if err != nil {
			a.logger.Error("failed to unseal", zap.Error(err))
			return err
		}
		if !res.Sealed {
			return nil
		}
	}

	return nil
}
