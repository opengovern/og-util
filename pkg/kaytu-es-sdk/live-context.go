package kaytu

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/kaytu-io/kaytu-util/pkg/steampipe"
	"github.com/turbot/steampipe-plugin-sdk/v5/connection"
	"github.com/turbot/steampipe-plugin-sdk/v5/plugin"
	"strconv"
)

type KaytuConfigKey string

const (
	KaytuConfigKeyAccountID                 KaytuConfigKey = "account_id"
	KaytuConfigKeyResourceCollectionFilters KaytuConfigKey = "resource_collection_filters"
)

type SelfClient struct {
	conn *pgx.Conn
}

func NewSelfClientCached(ctx context.Context, cache *connection.ConnectionCache) (*SelfClient, error) {
	value, ok := cache.Get(ctx, "kaytu-steampipe-self-client")
	if ok {
		return value.(*SelfClient), nil
	}

	plugin.Logger(ctx).Warn("client is not cached, creating a new one")

	client, err := NewSelfClient(ctx)
	if err != nil {
		return nil, err
	}

	cache.Set(ctx, "kaytu-steampipe-self-client", client)

	return client, nil
}

func NewSelfClient(ctx context.Context) (*SelfClient, error) {
	defaultOption := steampipe.GetDefaultSteampipeOption()
	uintPort, err := strconv.ParseUint(defaultOption.Port, 10, 16)
	if err != nil {
		return nil, err
	}
	conn, err := pgx.ConnectConfig(ctx, &pgx.ConnConfig{
		Config: pgconn.Config{
			Host:     "localhost",
			Port:     uint16(uintPort),
			Database: defaultOption.Db,
			User:     defaultOption.User,
			Password: defaultOption.Pass,
		},
	})
	if err != nil {
		return nil, err
	}

	return &SelfClient{conn: conn}, nil
}

func (sc *SelfClient) GetConfigTableValueOrNil(ctx context.Context, key KaytuConfigKey) (*string, error) {
	var value *string
	err := sc.conn.QueryRow(ctx, "SELECT value FROM kaytu_configs WHERE key = $1", string(key)).Scan(&value)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		// if table does not exist, return nil check sql state to verify
		if err, ok := err.(*pgconn.PgError); ok {
			if err.Code == "42P01" {
				return nil, nil
			}
		}
		return nil, err
	}

	return value, nil
}
