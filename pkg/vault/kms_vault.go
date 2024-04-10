package vault

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

type AwsVaultConfig struct {
	Region    string `yaml:"region" json:"region" koanf:"region"`
	RoleArn   string `yaml:"role_arn" json:"role_arn" koanf:"role_arn"`
	AccessKey string `yaml:"access_key" json:"access_key" koanf:"access_key"`
	SecretKey string `yaml:"secret_key" json:"secret_key" koanf:"secret_key"`
}

func getAWSConfig(ctx context.Context, awsAccessKey, awsSecretKey, awsSessionToken, assumeRoleArn string) (aws.Config, error) {
	opts := make([]func(*config.LoadOptions) error, 0)

	if awsAccessKey != "" {
		opts = append(opts, config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(awsAccessKey, awsSecretKey, awsSessionToken)))
	}

	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return aws.Config{}, fmt.Errorf("failed to load AWS config: %w", err)
	}

	if assumeRoleArn != "" {
		cfg, err = config.LoadDefaultConfig(context.Background(), config.WithCredentialsProvider(stscreds.NewAssumeRoleProvider(sts.NewFromConfig(cfg), assumeRoleArn)))
		if err != nil {
			return aws.Config{}, fmt.Errorf("failed to assume role: %w", err)
		}
	}

	return cfg, nil
}

type KMSVaultSourceConfig struct {
	kmsClient *kms.Client
	keyArn    string
}

func NewKMSVaultSourceConfig(ctx context.Context, awsConfig AwsVaultConfig, keyArn string) (*KMSVaultSourceConfig, error) {
	var err error
	cfg, err := config.LoadDefaultConfig(ctx)
	// if the keys are not provided, the default credentials from service account will be used
	if awsConfig.AccessKey != "" && awsConfig.SecretKey != "" {
		cfg, err = getAWSConfig(ctx, awsConfig.AccessKey, awsConfig.SecretKey, "", "")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load SDK configuration: %v", err)
	}
	cfg.Region = awsConfig.Region
	// Create KMS client with loaded configuration
	svc := kms.NewFromConfig(cfg)

	return &KMSVaultSourceConfig{
		kmsClient: svc,
		keyArn:    keyArn,
	}, nil
}

func (v *KMSVaultSourceConfig) Encrypt(ctx context.Context, cred map[string]any) (string, error) {
	bytes, err := json.Marshal(cred)
	if err != nil {
		return "", err
	}

	result, err := v.kmsClient.Encrypt(ctx, &kms.EncryptInput{
		KeyId:               &v.keyArn,
		Plaintext:           bytes,
		EncryptionAlgorithm: types.EncryptionAlgorithmSpecSymmetricDefault,
		EncryptionContext:   nil, //TODO-Saleh use workspaceID
		GrantTokens:         nil,
	})
	if err != nil {
		return "", fmt.Errorf("failed to encrypt ciphertext: %v", err)
	}
	encoded := base64.StdEncoding.EncodeToString(result.CiphertextBlob)
	return encoded, nil
}

func (v *KMSVaultSourceConfig) Decrypt(ctx context.Context, cypherText string) (map[string]any, error) {
	bytes, err := base64.StdEncoding.DecodeString(cypherText)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ciphertext: %v", err)
	}

	result, err := v.kmsClient.Decrypt(ctx, &kms.DecryptInput{
		CiphertextBlob:      bytes,
		EncryptionAlgorithm: types.EncryptionAlgorithmSpecSymmetricDefault,
		KeyId:               &v.keyArn,
		EncryptionContext:   nil, //TODO-Saleh use workspaceID
	})
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt ciphertext: %v", err)
	}

	conf := make(map[string]any)
	err = json.Unmarshal(result.Plaintext, &conf)
	if err != nil {
		return nil, err
	}

	return conf, nil
}
