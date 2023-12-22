package kaytu

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/opensearch-project/opensearch-go/v2/opensearchapi"
	"io"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/opensearch-project/opensearch-go/v2"
	signer "github.com/opensearch-project/opensearch-go/v2/signer/awsv2"
	"github.com/turbot/steampipe-plugin-sdk/v5/connection"
	"github.com/turbot/steampipe-plugin-sdk/v5/plugin"
	"github.com/turbot/steampipe-plugin-sdk/v5/plugin/schema"
	"net/http"
	"os"
)

type ResourceCollectionFilter struct {
	Connectors    []string          `json:"connectors"`
	AccountIDs    []string          `json:"account_ids"`
	Regions       []string          `json:"regions"`
	ResourceTypes []string          `json:"resource_types"`
	Tags          map[string]string `json:"tags"`
}

type ClientConfig struct {
	Addresses []string `cty:"addresses"`
	Username  *string  `cty:"username"`
	Password  *string  `cty:"password"`

	IsOpenSearch  *bool   `cty:"is_open_search"`
	AwsRegion     *string `cty:"aws_region"`
	AssumeRoleArn *string `cty:"assume_role_arn"`
	ExternalID    *string `cty:"external_id"`
}

func ConfigSchema() map[string]*schema.Attribute {
	return map[string]*schema.Attribute{
		"addresses": {
			Type: schema.TypeList,
			Elem: &schema.Attribute{Type: schema.TypeString},
		},
		"username": {
			Type: schema.TypeString,
		},
		"password": {
			Type: schema.TypeString,
		},
		"is_open_search": {
			Type:     schema.TypeBool,
			Required: false,
		},
		"aws_region": {
			Type:     schema.TypeString,
			Required: false,
		},
		"assume_role_arn": {
			Type:     schema.TypeString,
			Required: false,
		},
		"external_id": {
			Type:     schema.TypeString,
			Required: false,
		},
	}
}

func ConfigInstance() interface{} {
	return &ClientConfig{}
}

func GetConfig(connection *plugin.Connection) ClientConfig {
	if connection == nil || connection.Config == nil {
		return ClientConfig{}
	}
	config, _ := connection.Config.(ClientConfig)
	return config
}

type Client struct {
	es *opensearch.Client
}

func NewClientCached(c ClientConfig, cache *connection.ConnectionCache, ctx context.Context) (Client, error) {
	value, ok := cache.Get(ctx, "kaytu-es-client")
	if ok {
		return value.(Client), nil
	}

	plugin.Logger(ctx).Warn("client is not cached, creating a new one")

	client, err := NewClient(c)
	if err != nil {
		return Client{}, err
	}

	cache.Set(ctx, "kaytu-es-client", client)

	return client, nil
}

func NewClient(c ClientConfig) (Client, error) {
	if c.Addresses == nil || len(c.Addresses) == 0 {
		address := os.Getenv("ELASTICSEARCH_ADDRESS")
		c.Addresses = []string{address}
	}

	if c.Username == nil || len(*c.Username) == 0 {
		username := os.Getenv("ELASTICSEARCH_USERNAME")
		c.Username = &username
	}

	if c.Password == nil || len(*c.Password) == 0 {
		password := os.Getenv("ELASTICSEARCH_PASSWORD")
		c.Password = &password
	}

	if c.IsOpenSearch == nil {
		isOpenSearch := os.Getenv("ELASTICSEARCH_ISOPENSEARCH")
		if len(isOpenSearch) > 0 {
			b, _ := strconv.ParseBool(isOpenSearch)
			c.IsOpenSearch = &b
		}
	}

	if c.AwsRegion == nil || len(*c.AwsRegion) == 0 {
		awsRegion := os.Getenv("ELASTICSEARCH_AWSREGION")
		if len(awsRegion) > 0 {
			c.AwsRegion = &awsRegion
		}
	}

	if c.AssumeRoleArn == nil || len(*c.AssumeRoleArn) == 0 {
		assumeRoleArn := os.Getenv("ELASTICSEARCH_ASSUME_ROLE_ARN")
		if len(assumeRoleArn) > 0 {
			c.AssumeRoleArn = &assumeRoleArn
		}
	}

	if c.ExternalID == nil || len(*c.ExternalID) == 0 {
		externalID := os.Getenv("ELASTICSEARCH_EXTERNAL_ID")
		if len(externalID) > 0 {
			c.ExternalID = &externalID
		}
	}

	fmt.Println("ES Addresses:", c.Addresses)
	fmt.Println("ES Username:", c.Username)
	fmt.Println("ES IsOpenSearch:", c.IsOpenSearch)
	fmt.Println("ES AwsRegion:", c.AwsRegion)
	fmt.Println("ES AssumeRoleArn:", c.AssumeRoleArn)
	fmt.Println("ES ExternalID:", c.ExternalID)
	cfg := opensearch.Config{
		Addresses:           c.Addresses,
		Username:            *c.Username,
		Password:            *c.Password,
		CompressRequestBody: true,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, //nolint,gosec
			},
		},
	}

	if c.IsOpenSearch != nil && *c.IsOpenSearch {
		awsConfig, err := config.LoadDefaultConfig(context.Background())
		if err != nil {
			return Client{}, err
		}
		if c.AwsRegion != nil {
			awsConfig.Region = *c.AwsRegion
		}

		if c.AssumeRoleArn != nil && len(*c.AssumeRoleArn) > 0 {
			awsConfig, err = config.LoadDefaultConfig(
				context.Background(),
				config.WithCredentialsProvider(
					stscreds.NewAssumeRoleProvider(
						sts.NewFromConfig(awsConfig),
						*c.AssumeRoleArn,
						func(o *stscreds.AssumeRoleOptions) {
							o.ExternalID = c.ExternalID
						},
					),
				),
			)
			if err != nil {
				return Client{}, err
			}
		}

		awsSigner, err := signer.NewSigner(awsConfig)
		if err != nil {
			return Client{}, err
		}
		cfg.Signer = awsSigner
	}

	es, err := opensearch.NewClient(cfg)
	if err != nil {
		return Client{}, err
	}

	return Client{es: es}, nil
}

func (c Client) ES() *opensearch.Client {
	return c.es
}

func (c *Client) SetES(es *opensearch.Client) {
	c.es = es
}

func (c *Client) Delete(docID, index string) error {
	opts := []func(*opensearchapi.DeleteRequest){
		c.es.Delete.WithContext(context.Background()),
	}

	res, err := c.es.Delete(index, docID, opts...)
	defer CloseSafe(res)
	if err != nil {
		var b []byte
		if res != nil {
			b, _ = io.ReadAll(res.Body)
		}
		fmt.Printf("failure while querying es: %v\n%s\n", err, string(b))
		return err
	} else if err := CheckError(res); err != nil {
		var b []byte
		if res != nil {
			b, _ = io.ReadAll(res.Body)
		}
		fmt.Printf("failure while querying es: %v\n%s\n", err, string(b))
		return err
	}

	return nil
}
