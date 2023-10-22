package kaytu

import (
	"context"
	"crypto/tls"
	"fmt"
	elasticsearchv7 "github.com/elastic/go-elasticsearch/v7"
	"github.com/turbot/steampipe-plugin-sdk/v5/connection"
	"github.com/turbot/steampipe-plugin-sdk/v5/plugin"
	"github.com/turbot/steampipe-plugin-sdk/v5/plugin/schema"
	"net/http"
	"os"
)

type ResourceCollectionFilter struct {
	AccountIDs    []string          `json:"account_ids"`
	Regions       []string          `json:"regions"`
	ResourceTypes []string          `json:"resource_types"`
	Tags          map[string]string `json:"tags"`
}

type ClientConfig struct {
	Addresses []string `cty:"addresses"`
	Username  *string  `cty:"username"`
	Password  *string  `cty:"password"`
	AccountID *string  `cty:"accountID"`
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
		"accountID": {
			Type: schema.TypeString,
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
	es *elasticsearchv7.Client
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
	if c.AccountID == nil || len(*c.AccountID) == 0 {
		accountID := os.Getenv("STEAMPIPE_ACCOUNT_ID")
		if len(accountID) == 0 {
			return Client{}, fmt.Errorf("accountID is either null or empty: %v", c.AccountID)
		}
		c.AccountID = &accountID
	}

	if c.Addresses == nil || len(c.Addresses) == 0 {
		address := os.Getenv("ES_ADDRESS")
		c.Addresses = []string{address}
	}

	if c.Username == nil || len(*c.Username) == 0 {
		username := os.Getenv("ES_USERNAME")
		c.Username = &username
	}

	if c.Password == nil || len(*c.Password) == 0 {
		password := os.Getenv("ES_PASSWORD")
		c.Password = &password
	}

	cfg := elasticsearchv7.Config{
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

	es, err := elasticsearchv7.NewClient(cfg)
	if err != nil {
		return Client{}, err
	}

	return Client{es: es}, nil
}

func (c Client) ES() *elasticsearchv7.Client {
	return c.es
}

func (c *Client) SetES(es *elasticsearchv7.Client) {
	c.es = es
}
