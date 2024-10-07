package steampipe

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/turbot/steampipe-plugin-sdk/v5/connection"
	"github.com/turbot/steampipe-plugin-sdk/v5/plugin"
	"time"
)

type KaytuConfigKey string

const (
	KaytuConfigKeyAccountID                 KaytuConfigKey = "account_id"
	KaytuConfigKeyResourceCollectionFilters KaytuConfigKey = "resource_collection_filters"
	KaytuConfigKeyClientType                KaytuConfigKey = "client_type"
)

type SelfClient struct {
	createdAt time.Time
	conn      *pgxpool.Pool
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
	defaultOption := GetDefaultSteampipeOption()
	connString := fmt.Sprintf(`host=%s port=%s user=%s password=%s dbname=%s sslmode=disable TimeZone=GMT`,
		defaultOption.Host,
		defaultOption.Port,
		defaultOption.User,
		defaultOption.Pass,
		defaultOption.Db,
	)

	conn, err := pgxpool.New(ctx, connString)
	if err != nil {
		return nil, err
	}

	return &SelfClient{conn: conn, createdAt: time.Now()}, nil
}

func (sc *SelfClient) GetConnection() *pgxpool.Pool {
	return sc.conn
}

func (sc *SelfClient) GetConfigTableValueOrNil(ctx context.Context, key KaytuConfigKey) (*string, error) {
	var value *string
	// Create table if not exists
	_, err := sc.conn.Exec(ctx, "CREATE TABLE IF NOT EXISTS kaytu_configs(key TEXT PRIMARY KEY, value TEXT)")
	if err != nil {
		return nil, err
	}

	err = sc.conn.QueryRow(ctx, "SELECT value FROM kaytu_configs WHERE key = $1", string(key)).Scan(&value)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		// if table does not exist, return nil check sql state to verify
		if err, ok := err.(*pgconn.PgError); ok {
			switch err.Code {
			case "42P01":
				return nil, nil
			case "HY000":
				if sc.createdAt.Add(5 * time.Minute).Before(time.Now()) {
					return nil, err
				}
				err := sc.reconnect(ctx)
				if err != nil {
					return nil, err
				}
				return sc.GetConfigTableValueOrNil(ctx, key)
			}
		}
		return nil, err
	}

	return value, nil
}

func (sc *SelfClient) reconnect(ctx context.Context) error {
	sc.conn.Close()

	newClient, err := NewSelfClient(ctx)
	if err != nil {
		return err
	}
	sc.conn = newClient.conn
	sc.createdAt = newClient.createdAt

	return nil
}
