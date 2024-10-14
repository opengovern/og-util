package kaytu

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/elastic/go-elasticsearch/v7/esapi"
	"github.com/opensearch-project/opensearch-go/v2"
	"github.com/opensearch-project/opensearch-go/v2/opensearchapi"
	"github.com/opensearch-project/opensearch-go/v2/opensearchutil"
	"github.com/turbot/steampipe-plugin-sdk/v5/plugin"
	"github.com/turbot/steampipe-plugin-sdk/v5/plugin/context_key"
)

type BaseESPaginator struct {
	client *opensearch.Client

	index    string         // Query index
	query    map[string]any // Query filters
	pageSize int64          // Query page size
	pitID    string         // Query point in time id (Only set if max is greater than size)

	limit   int64 // Maximum documents to query
	queried int64 // Current count of queried documents

	sort []map[string]any

	searchAfter []any
	done        bool
}

func NewPaginatorWithSort(client *opensearch.Client, index string, filters []BoolFilter, limit *int64, sort []map[string]any) (*BaseESPaginator, error) {
	var query map[string]any
	if len(filters) > 0 {
		query = map[string]any{
			"bool": map[string]any{
				"filter": filters,
			},
		}
	} else {
		query = map[string]any{
			"match_all": map[string]any{},
		}
	}

	// We need a tiebreaker for the sort to work properly, so we add _id if it's not present
	foundId := false
	for _, sortMap := range sort {
		for k, _ := range sortMap {
			if k == "_id" {
				foundId = true
				break
			}
		}
	}
	if !foundId {
		sort = append(sort, map[string]any{
			"_id": "desc",
		})
	}

	var max int64
	if limit == nil {
		max = math.MaxInt64
	} else {
		max = *limit
	}

	if max < 0 {
		return nil, fmt.Errorf("invalid limit: %d", max)
	}

	return &BaseESPaginator{
		client:   client,
		index:    index,
		query:    query,
		pageSize: 10000,
		limit:    max,
		sort:     sort,
		queried:  0,
	}, nil
}

func NewPaginator(client *opensearch.Client, index string, filters []BoolFilter, limit *int64) (*BaseESPaginator, error) {
	return NewPaginatorWithSort(client, index, filters, limit, nil)
}

func (p *BaseESPaginator) Done() bool {
	return p.done
}

func (p *BaseESPaginator) UpdatePageSize(i int64) {
	p.pageSize = i
}

func (p *BaseESPaginator) Deallocate(ctx context.Context) error {
	if p.pitID != "" {
		pitRaw, _, err := p.client.PointInTime.Delete(
			p.client.PointInTime.Delete.WithPitID(p.pitID),
		)
		if err != nil {
			LogWarn(ctx, fmt.Sprintf("Deallocate.Err err=%v pitRaw=%v", err, pitRaw))
			return err
		} else if errIf := CheckErrorWithContext(pitRaw, ctx); errIf != nil {
			LogWarn(ctx, fmt.Sprintf("Deallocate.CheckErr err=%v errIf=%v pitRaw=%s", err, errIf, pitRaw.String()))

			if pitRaw.StatusCode != http.StatusMethodNotAllowed {
				return errIf
			}

			// try elasticsearch api instead
			req := esapi.ClosePointInTimeRequest{
				Body: strings.NewReader(fmt.Sprintf(`{"id": "%s"}`, p.pitID)),
			}
			res, err2 := req.Do(ctx, p.client.Transport)
			defer ESCloseSafe(res)
			if err2 != nil {
				if errIf != nil {
					return errIf
				}
				return err
			} else if err2 := ESCheckError(res); err2 != nil {
				if errIf != nil {
					return errIf
				}
				return err
			}
		}
		//
		//if err != nil {
		//	LogWarn(ctx, fmt.Sprintf("failed to delete PIT %v", err))
		//	return err
		//} else if errIf := CheckError(pitRaw); errIf != nil {
		//	LogWarn(ctx, fmt.Sprintf("failed to delete PIT %v", errIf))
		//	return errIf
		//}
		p.pitID = ""
	}
	return nil
}

// The response will be marshalled if the search was successfull
func (p *BaseESPaginator) Search(ctx context.Context, response any) error {
	return p.SearchWithLog(ctx, response, false)
}

func (p *BaseESPaginator) SearchWithLog(ctx context.Context, response any, doLog bool) error {
	if p.done {
		return errors.New("no more page to query")
	}

	if err := p.CreatePit(ctx); err != nil {
		if IsIndexNotFoundErr(err) {
			return nil
		}
		return err
	}

	sa := SearchRequest{
		Size:  &p.pageSize,
		Query: p.query,
		Sort:  p.sort,
	}

	if p.limit > p.pageSize && p.pitID != "" {
		sa.PIT = &PointInTime{
			ID:        p.pitID,
			KeepAlive: "1m",
		}
	}

	if p.searchAfter != nil {
		sa.SearchAfter = p.searchAfter
	}

	opts := []func(*opensearchapi.SearchRequest){
		p.client.Search.WithContext(ctx),
		p.client.Search.WithBody(opensearchutil.NewJSONReader(sa)),
		p.client.Search.WithTrackTotalHits(false),
	}
	if sa.PIT == nil {
		opts = append(opts, p.client.Search.WithIndex(p.index))
	}

	if doLog {
		m, _ := json.Marshal(sa)
		LogWarn(ctx, fmt.Sprintf("SearchWithLog: %s", string(m)))
	}

	res, err := p.client.Search(opts...)
	defer CloseSafe(res)
	if err != nil {
		var b []byte
		if res != nil {
			b, _ = io.ReadAll(res.Body)
		}
		if doLog {
			if ctx.Value(context_key.Logger) == nil {
				fmt.Println(fmt.Sprintf("failure while querying es: %v\n%s\n", err, string(b)))
			} else {
				plugin.Logger(ctx).Trace(fmt.Sprintf("failure while querying es: %v\n%s\n", err, string(b)))
			}
		}

		return err
	} else if err := CheckError(res); err != nil {
		if IsIndexNotFoundErr(err) {
			return nil
		}
		var b []byte
		if res != nil {
			b, _ = io.ReadAll(res.Body)
		}
		if doLog {
			if ctx.Value(context_key.Logger) == nil {
				fmt.Println(fmt.Sprintf("failure while querying es: %v\n%s\n", err, string(b)))
			} else {
				plugin.Logger(ctx).Trace(fmt.Sprintf("failure while querying es: %v\n%s\n", err, string(b)))
			}
		}
		return err
	}

	b, err := io.ReadAll(res.Body)
	if err != nil {
		if doLog {
			if ctx.Value(context_key.Logger) == nil {
				fmt.Println(fmt.Sprintf("read response: %v", err))
			} else {
				plugin.Logger(ctx).Warn(fmt.Sprintf("read response: %v", err))
			}
		}
		return fmt.Errorf("read response: %w", err)
	}

	if err := json.Unmarshal(b, response); err != nil {
		if doLog {
			if ctx.Value(context_key.Logger) == nil {
				fmt.Println(fmt.Sprintf("unmarshal response: %v", err))
			} else {
				plugin.Logger(ctx).Warn(fmt.Sprintf("unmarshal response: %v", err))
			}
		}
		return fmt.Errorf("unmarshal response: %w", err)
	}

	return nil
}

func (p *BaseESPaginator) CreatePit(ctx context.Context) (err error) {
	return p.CreatePitWithRetry(ctx, 0)
}

// createPit, sets up the PointInTime for the search with more than 10000 limit
func (p *BaseESPaginator) CreatePitWithRetry(ctx context.Context, retry int) (err error) {
	if p.limit <= p.pageSize {
		return nil
	} else if p.pitID != "" {
		return nil
	}

	defer func() {
		if err == nil {
			return
		}

		// check if the index exists
		res, resErr := p.client.Indices.Exists([]string{p.index})
		defer CloseSafe(res)
		if resErr != nil {
			return
		}
		if res.StatusCode == http.StatusNotFound {
			err = nil
			return
		}
	}()

	pitRaw, pitRes, err := p.client.PointInTime.Create(
		p.client.PointInTime.Create.WithIndex(p.index),
		p.client.PointInTime.Create.WithKeepAlive(1*time.Minute),
		p.client.PointInTime.Create.WithContext(ctx),
	)

	defer CloseSafe(pitRaw)
	if err != nil && !strings.Contains(err.Error(), "illegal_argument_exception") {
		LogWarn(ctx, fmt.Sprintf("PointInTime.Err err=%v pitRaw=%v", err, pitRaw))
		return err
	} else if errIf := CheckErrorWithContext(pitRaw, ctx); errIf != nil || (err != nil && strings.Contains(err.Error(), "illegal_argument_exception")) {
		LogWarn(ctx, fmt.Sprintf("PointInTime.CheckErr err=%v errIf=%v pitRaw=%s", err, errIf, pitRaw.String()))
		if pitRaw.StatusCode == http.StatusTooManyRequests && retry < 10 {
			time.Sleep(time.Duration(retry+1) * time.Second)
			return p.CreatePitWithRetry(ctx, retry+1)
		}

		// try elasticsearch api instead
		req := esapi.OpenPointInTimeRequest{
			Index:     []string{p.index},
			KeepAlive: "1m",
		}
		res, err2 := req.Do(ctx, p.client.Transport)
		defer ESCloseSafe(res)
		if err2 != nil {
			if errIf != nil {
				return errIf
			}
			return err
		} else if err2 := ESCheckError(res); err2 != nil {
			if IsIndexNotFoundErr(err2) {
				return nil
			}
			if errIf != nil {
				return errIf
			}
			return err
		} else {
			data, err2 := io.ReadAll(res.Body)
			if err2 != nil {
				return fmt.Errorf("read response: %w", err2)
			}
			var pit PointInTimeResponse
			if err2 = json.Unmarshal(data, &pit); err2 != nil {
				return fmt.Errorf("unmarshal response: %w", err2)
			}
			p.pitID = pit.ID
			return nil
		}
	}

	p.pitID = pitRes.PitID
	return nil
}

func (p *BaseESPaginator) UpdateState(numHits int64, searchAfter []any, pitID string) {
	p.queried += numHits
	if p.queried > p.limit {
		// Have found enough documents
		p.done = true
	} else if numHits == 0 || numHits < p.pageSize {
		// The isn't more documents thus the last batch had less than page size
		p.done = true
	}

	if numHits > 0 {
		p.searchAfter = searchAfter
		p.pitID = pitID
	}
}
