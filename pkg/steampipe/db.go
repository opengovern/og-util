package steampipe

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/jackc/pgx/v4/pgxpool"
	pg_query_go "github.com/pganalyze/pg_query_go/v4"
)

type DirectionType string

const (
	DirectionAscending  DirectionType = "asc"
	DirectionDescending DirectionType = "desc"
)

type Option struct {
	Host string
	Port string
	User string
	Pass string
	Db   string
}

type Database struct {
	conn *pgxpool.Pool
}

type Result struct {
	Headers []string
	Data    [][]interface{}
}

func GetDefaultSteampipeOption() Option {
	return Option{
		Host: "localhost",
		Port: "9193",
		User: "steampipe",
		Pass: "abcd",
		Db:   "steampipe",
	}
}

func NewSteampipeDatabase(option Option) (*Database, error) {
	var err error
	connString := fmt.Sprintf(`host=%s port=%s user=%s password=%s dbname=%s sslmode=disable TimeZone=GMT`,
		option.Host,
		option.Port,
		option.User,
		option.Pass,
		option.Db,
	)

	conn, err := pgxpool.Connect(context.Background(), connString)
	if err != nil {
		return nil, err
	}
	if err := conn.Ping(context.Background()); err != nil {
		return nil, err
	}

	return &Database{conn: conn}, nil
}

func (s *Database) Conn() *pgxpool.Pool {
	return s.conn
}

func (s *Database) Query(ctx context.Context, query string, from, size *int, orderBy string,
	orderDir DirectionType,
) (*Result, error) {
	// parameterize order by is not supported by steampipe.
	// in order to prevent SQL Injection, we ensure that orderby field is only consists of
	// characters and underline.
	if ok, err := regexp.Match("(\\w|_)+", []byte(orderBy)); err != nil || orderBy != "" && !ok {
		if err != nil {
			return nil, err
		}
		return nil, errors.New("invalid orderby field:" + orderBy)
	}
	statements, err := pg_query_go.Parse(query)
	if err != nil {
		return nil, fmt.Errorf("failed to parse query: %w", err)
	}
	if len(statements.GetStmts()) != 1 {
		return nil, errors.New("only one statement is supported")
	}
	statement := statements.GetStmts()[0].GetStmt().GetSelectStmt()
	if statement == nil {
		return nil, errors.New("only select statement is supported")
	}
	if (statement.GetSortClause() == nil || len(statement.GetSortClause()) == 0) && orderBy != "" {
		direction := pg_query_go.SortByDir_SORTBY_DEFAULT
		switch orderDir {
		case DirectionAscending:
			direction = pg_query_go.SortByDir_SORTBY_ASC
		case DirectionDescending:
			direction = pg_query_go.SortByDir_SORTBY_DESC
		default:
			direction = pg_query_go.SortByDir_SORTBY_DEFAULT
		}
		statement.SortClause = append(statement.SortClause,
			pg_query_go.MakeSortByNode(
				pg_query_go.MakeColumnRefNode(
					[]*pg_query_go.Node{
						pg_query_go.MakeStrNode(orderBy),
					},
					0,
				),
				direction,
				pg_query_go.SortByNulls_SORTBY_NULLS_DEFAULT,
				0,
			),
		)
	} /* else if statement.GetSortClause() == nil || len(statement.GetSortClause()) == 0 {
		statement.SortClause = append(statement.SortClause,
			pg_query_go.MakeSortByNode(
				pg_query_go.MakeAConstIntNode(1, 0),
				pg_query_go.SortByDir_SORTBY_ASC,
				pg_query_go.SortByNulls_SORTBY_NULLS_DEFAULT,
				0,
			),
		)
	} */
	if statement.GetLimitCount() == nil && size != nil {
		statement.LimitOption = pg_query_go.LimitOption_LIMIT_OPTION_COUNT
		statement.LimitCount = pg_query_go.MakeAConstIntNode(int64(*size), 0)
		if from != nil {
			statement.LimitOffset = pg_query_go.MakeAConstIntNode(int64(*from), 0)
		}
	}

	query, err = pg_query_go.Deparse(statements)
	if err != nil {
		return nil, fmt.Errorf("failed to deparse query: %w", err)
	}

	fmt.Println("query is: ", query)
	fmt.Println("size: ", size, "from:", from)

	r, err := s.conn.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	if r.Err() != nil {
		return nil, r.Err()
	}
	defer r.Close()

	var headers []string
	ctxIdx := -1
	for idx, field := range r.FieldDescriptions() {
		if string(field.Name) == "_ctx" {
			ctxIdx = idx
			continue
		}
		headers = append(headers, string(field.Name))
	}
	var result [][]interface{}
	for r.Next() {
		v, err := r.Values()
		if err != nil {
			return nil, err
		}

		var record []interface{}
		for idx, c := range v {
			if idx == ctxIdx {
				continue
			}
			record = append(record, c)
		}

		result = append(result, record)
	}

	return &Result{
		Headers: headers,
		Data:    result,
	}, nil
}

func (s *Database) QueryAll(ctx context.Context, query string) (*Result, error) {
	r, err := s.conn.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	if r.Err() != nil {
		return nil, r.Err()
	}
	defer r.Close()

	var headers []string
	for _, field := range r.FieldDescriptions() {
		headers = append(headers, string(field.Name))
	}
	var result [][]interface{}
	for r.Next() {
		v, err := r.Values()
		if err != nil {
			return nil, err
		}

		result = append(result, v)
	}

	return &Result{
		Headers: headers,
		Data:    result,
	}, nil
}

func (s *Database) Count(query string) (*Result, error) {
	r, err := s.conn.Query(context.Background(), query)
	if err != nil {
		return nil, err
	}
	if r.Err() != nil {
		return nil, r.Err()
	}
	defer r.Close()

	var headers []string
	for _, field := range r.FieldDescriptions() {
		headers = append(headers, string(field.Name))
	}
	var result [][]interface{}
	for r.Next() {
		v, err := r.Values()
		if err != nil {
			return nil, err
		}

		result = append(result, v)
	}

	return &Result{
		Headers: headers,
		Data:    result,
	}, nil
}

func (s *Database) SetConfigTableValue(ctx context.Context, key OpenGovernanceConfigKey, value string) error {
	// Create table if not exists
	_, err := s.conn.Exec(ctx, "CREATE TABLE IF NOT EXISTS og_configs(key TEXT PRIMARY KEY, value TEXT)")
	if err != nil {
		return err
	}

	// Insert or update
	_, err = s.conn.Exec(ctx, "INSERT INTO og_configs(key, value) VALUES($1, $2) ON CONFLICT (key) DO UPDATE SET value = $2", string(key), value)
	if err != nil {
		return err
	}

	return nil
}

func (s *Database) UnsetConfigTableValue(ctx context.Context, key OpenGovernanceConfigKey) error {
	// Create table if not exists
	_, err := s.conn.Exec(ctx, "CREATE TABLE IF NOT EXISTS og_configs(key TEXT PRIMARY KEY, value TEXT)")
	if err != nil {
		return err
	}

	_, err = s.conn.Exec(ctx, "DELETE FROM og_configs WHERE key = $1", string(key))
	if err != nil {
		return err
	}

	return nil
}
