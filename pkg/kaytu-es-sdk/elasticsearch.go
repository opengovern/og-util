package kaytu

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/turbot/steampipe-plugin-sdk/v5/grpc/proto"
	"io"
	"math"
	"strings"

	elasticsearchv7 "github.com/elastic/go-elasticsearch/v7"
	"github.com/elastic/go-elasticsearch/v7/esapi"
	"github.com/elastic/go-elasticsearch/v7/esutil"
	"github.com/turbot/steampipe-plugin-sdk/v5/plugin"
)

func CloseSafe(resp *esapi.Response) {
	if resp != nil && resp.Body != nil {
		_, _ = io.ReadAll(resp.Body)
		resp.Body.Close() //nolint,gosec
	}
}

func CheckError(resp *esapi.Response) error {
	if !resp.IsError() {
		return nil
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read error: %w", err)
	}

	var e ErrorResponse
	if err := json.Unmarshal(data, &e); err != nil {
		return fmt.Errorf(string(data))
	}

	return e
}

func IsIndexNotFoundErr(err error) bool {
	var e ErrorResponse
	return errors.As(err, &e) &&
		strings.EqualFold(e.Info.Type, "index_not_found_exception")
}

type BoolFilter interface {
	IsBoolFilter()
}

func BuildFilter(ctx context.Context, queryContext *plugin.QueryContext, filtersQuals map[string]string, accountProvider, accountID string) []BoolFilter {
	var filters []BoolFilter
	if queryContext.UnsafeQuals == nil {
		return filters
	}

	plugin.Logger(ctx).Trace("BuildFilter", "queryContext.UnsafeQuals", queryContext.UnsafeQuals)

	for _, quals := range queryContext.UnsafeQuals {
		if quals == nil {
			continue
		}

		for _, qual := range quals.GetQuals() {
			fn := qual.GetFieldName()
			fieldName, ok := filtersQuals[fn]
			if !ok {
				continue
			}

			var oprStr string
			opr := qual.GetOperator()
			if strOpr, ok := opr.(*proto.Qual_StringValue); ok {
				oprStr = strOpr.StringValue
			}
			if oprStr == "=" {
				if qual.GetValue().GetListValue() != nil {
					vals := qual.GetValue().GetListValue().GetValues()
					values := make([]string, 0, len(vals))
					for _, value := range vals {
						values = append(values, qualValue(value))
					}

					filters = append(filters, TermsFilter(fieldName, values))
				} else {
					filters = append(filters, NewTermFilter(fieldName, qualValue(qual.GetValue())))
				}
			}
			if oprStr == ">" {
				filters = append(filters, NewRangeFilter(fieldName, qualValue(qual.GetValue()), "", "", ""))
			}
			if oprStr == ">=" {
				filters = append(filters, NewRangeFilter(fieldName, "", qualValue(qual.GetValue()), "", ""))
			}
			if oprStr == "<" {
				filters = append(filters, NewRangeFilter(fieldName, "", "", qualValue(qual.GetValue()), ""))
			}
			if oprStr == "<=" {
				filters = append(filters, NewRangeFilter(fieldName, "", "", "", qualValue(qual.GetValue())))
			}
		}
	}

	if len(accountID) > 0 && accountID != "all" {
		var accountFieldName string
		switch accountProvider {
		case "aws":
			accountFieldName = "AccountID"
		case "azure":
			accountFieldName = "SubscriptionID"
		}
		filters = append(filters, NewTermFilter("metadata."+accountFieldName, accountID))
	}

	plugin.Logger(ctx).Trace("BuildFilter", "filters", filters)

	return filters
}

func qualValue(qual *proto.QualValue) string {
	var valStr string
	val := qual.Value
	switch v := val.(type) {
	case *proto.QualValue_StringValue:
		valStr = v.StringValue
	case *proto.QualValue_Int64Value:
		valStr = fmt.Sprintf("%v", v.Int64Value)
	case *proto.QualValue_DoubleValue:
		valStr = fmt.Sprintf("%v", v.DoubleValue)
	case *proto.QualValue_BoolValue:
		valStr = fmt.Sprintf("%v", v.BoolValue)
	default:
		valStr = qual.String()
	}
	return valStr
}

type TermFilter struct {
	field string
	value string
}

func NewTermFilter(field, value string) BoolFilter {
	return TermFilter{
		field: field,
		value: value,
	}
}

func (t TermFilter) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"term": map[string]string{
			t.field: t.value,
		},
	})
}

func (t TermFilter) IsBoolFilter() {}

type termsFilter struct {
	field  string
	values []string
}

func TermsFilter(field string, values []string) BoolFilter {
	return termsFilter{
		field:  field,
		values: values,
	}
}

func (t termsFilter) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"terms": map[string][]string{
			t.field: t.values,
		},
	})
}

func (t termsFilter) IsBoolFilter() {}

type RangeFilter struct {
	field string
	gt    string
	gte   string
	lt    string
	lte   string
}

func NewRangeFilter(field, gt, gte, lt, lte string) BoolFilter {
	return RangeFilter{
		field: field,
		gt:    gt,
		gte:   gte,
		lt:    lt,
		lte:   lte,
	}
}

func (t RangeFilter) MarshalJSON() ([]byte, error) {
	fieldMap := map[string]interface{}{}
	if len(t.gt) > 0 {
		fieldMap["gt"] = t.gt
	}
	if len(t.gt) > 0 {
		fieldMap["gte"] = t.gt
	}
	if len(t.gt) > 0 {
		fieldMap["lt"] = t.lt
	}
	if len(t.gt) > 0 {
		fieldMap["lte"] = t.lt
	}

	return json.Marshal(map[string]interface{}{
		"range": map[string]interface{}{
			t.field: fieldMap,
		},
	})
}

func (t RangeFilter) IsBoolFilter() {}

type BaseESPaginator struct {
	client *elasticsearchv7.Client

	index    string         // Query index
	query    map[string]any // Query filters
	pageSize int64          // Query page size
	pitID    string         // Query point in time id (Only set if max is greater than size)

	limit   int64 // Maximum documents to query
	queried int64 // Current count of queried documents

	searchAfter []any
	done        bool
}

func NewPaginator(client *elasticsearchv7.Client, index string, filters []BoolFilter, limit *int64) (*BaseESPaginator, error) {
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
		queried:  0,
	}, nil
}

func (p *BaseESPaginator) Done() bool {
	return p.done
}

func (p *BaseESPaginator) UpdatePageSize(i int64) {
	p.pageSize = i
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
	}

	if p.limit > p.pageSize {
		sa.PIT = &PointInTime{
			ID:        p.pitID,
			KeepAlive: "1m",
		}

		sa.Sort = []map[string]any{
			{
				"_shard_doc": "desc",
			},
		}
	}

	if p.searchAfter != nil {
		sa.SearchAfter = p.searchAfter
	}

	opts := []func(*esapi.SearchRequest){
		p.client.Search.WithContext(ctx),
		p.client.Search.WithBody(esutil.NewJSONReader(sa)),
		p.client.Search.WithTrackTotalHits(false),
	}
	if sa.PIT == nil {
		opts = append(opts, p.client.Search.WithIndex(p.index))
	}

	if doLog {
		m, _ := json.Marshal(sa)
		plugin.Logger(ctx).Trace("SearchWithLog", string(m))
	}

	res, err := p.client.Search(opts...)
	defer CloseSafe(res)
	if err != nil {
		b, _ := io.ReadAll(res.Body)
		fmt.Printf("failure while querying es: %v\n%s\n", err, string(b))
		return err
	} else if err := CheckError(res); err != nil {
		if IsIndexNotFoundErr(err) {
			return nil
		}
		b, _ := io.ReadAll(res.Body)
		fmt.Printf("failure while querying es: %v\n%s\n", err, string(b))
		return err
	}

	b, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if err := json.Unmarshal(b, response); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}

	return nil
}

// createPit, sets up the PointInTime for the search with more than 10000 limit
func (p *BaseESPaginator) CreatePit(ctx context.Context) error {
	if p.limit < p.pageSize {
		return nil
	} else if p.pitID != "" {
		return nil
	}

	resPit, err := p.client.OpenPointInTime([]string{p.index}, "1m",
		p.client.OpenPointInTime.WithContext(ctx),
	)
	defer CloseSafe(resPit)
	if err != nil {
		return err
	} else if err := CheckError(resPit); err != nil {
		return err
	}

	data, err := io.ReadAll(resPit.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	var pit PointInTimeResponse
	if err := json.Unmarshal(data, &pit); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}

	p.pitID = pit.ID
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

func (c Client) Search(ctx context.Context, index string, query string, response any) error {
	return c.SearchWithTrackTotalHits(ctx, index, query, nil, response, false)
}

func (c Client) SearchWithFilterPath(ctx context.Context, index string, query string, filterPath []string, response any) error {
	return c.SearchWithTrackTotalHits(ctx, index, query, filterPath, response, false)
}

type CountResponse struct {
	Count int64 `json:"count"`
}

func (c Client) Count(ctx context.Context, index string) (int64, error) {
	opts := []func(count *esapi.CountRequest){
		c.es.Count.WithContext(ctx),
		c.es.Count.WithIndex(index),
	}

	res, err := c.es.Count(opts...)
	defer CloseSafe(res)
	if err != nil {
		return 0, err
	} else if err := CheckError(res); err != nil {
		if IsIndexNotFoundErr(err) {
			return 0, nil
		}
		return 0, err
	}

	b, err := io.ReadAll(res.Body)
	if err != nil {
		return 0, fmt.Errorf("read response: %w", err)
	}

	var response CountResponse
	if err := json.Unmarshal(b, &response); err != nil {
		return 0, fmt.Errorf("unmarshal response: %w", err)
	}
	return response.Count, nil
}

func (c Client) SearchWithTrackTotalHits(ctx context.Context, index string, query string, filterPath []string, response any, trackTotalHits any) error {
	opts := []func(*esapi.SearchRequest){
		c.es.Search.WithContext(ctx),
		c.es.Search.WithBody(strings.NewReader(query)),
		c.es.Search.WithTrackTotalHits(trackTotalHits),
		c.es.Search.WithIndex(index),
		c.es.Search.WithFilterPath(filterPath...),
	}

	res, err := c.es.Search(opts...)
	defer CloseSafe(res)
	if err != nil {
		b, _ := io.ReadAll(res.Body)
		fmt.Printf("failure while querying es: %v\n%s\n", err, string(b))
		return err
	} else if err := CheckError(res); err != nil {
		if IsIndexNotFoundErr(err) {
			return nil
		}
		b, _ := io.ReadAll(res.Body)
		fmt.Printf("failure while querying es: %v\n%s\n", err, string(b))
		return err
	}

	b, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if err := json.Unmarshal(b, response); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}
	return nil
}

func (c Client) GetByID(ctx context.Context, index string, id string, response any) error {
	opts := []func(request *esapi.GetRequest){
		c.es.Get.WithContext(ctx),
	}

	res, err := c.es.Get(index, id, opts...)
	defer CloseSafe(res)
	if err != nil {
		return err
	} else if err := CheckError(res); err != nil {
		if IsIndexNotFoundErr(err) {
			return nil
		}
		return err
	}

	b, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if err := json.Unmarshal(b, response); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}
	return nil
}

type DeleteByQueryResponse struct {
	Took             int  `json:"took"`
	TimedOut         bool `json:"timed_out"`
	Total            int  `json:"total"`
	Deleted          int  `json:"deleted"`
	Batched          int  `json:"batches"`
	VersionConflicts int  `json:"version_conflicts"`
	Noops            int  `json:"noops"`
	Retries          struct {
		Bulk   int `json:"bulk"`
		Search int `json:"search"`
	} `json:"retries"`
	ThrottledMillis      int     `json:"throttled_millis"`
	RequestsPerSecond    float64 `json:"requests_per_second"`
	ThrottledUntilMillis int     `json:"throttled_until_millis"`
	Failures             []any   `json:"failures"`
}

func DeleteByQuery(ctx context.Context, es *elasticsearchv7.Client, indices []string, query any, opts ...func(*esapi.DeleteByQueryRequest)) (DeleteByQueryResponse, error) {
	defaultOpts := []func(*esapi.DeleteByQueryRequest){
		es.DeleteByQuery.WithContext(ctx),
		es.DeleteByQuery.WithWaitForCompletion(true),
	}

	resp, err := es.DeleteByQuery(
		indices,
		esutil.NewJSONReader(query),
		append(defaultOpts, opts...)...,
	)
	defer CloseSafe(resp)
	if err != nil {
		return DeleteByQueryResponse{}, err
	} else if err := CheckError(resp); err != nil {
		if IsIndexNotFoundErr(err) {
			return DeleteByQueryResponse{}, nil
		}
		return DeleteByQueryResponse{}, err
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return DeleteByQueryResponse{}, fmt.Errorf("read response: %w", err)
	}

	var response DeleteByQueryResponse
	if err := json.Unmarshal(b, &response); err != nil {
		return DeleteByQueryResponse{}, fmt.Errorf("unmarshal response: %w", err)
	}
	return response, nil
}
