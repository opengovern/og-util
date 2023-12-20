package kaytu

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/elastic/go-elasticsearch/v7/esapi"
	"github.com/turbot/steampipe-plugin-sdk/v5/grpc/proto"
	"github.com/turbot/steampipe-plugin-sdk/v5/plugin"
	"github.com/turbot/steampipe-plugin-sdk/v5/plugin/context_key"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/opensearch-project/opensearch-go/v2"
	"github.com/opensearch-project/opensearch-go/v2/opensearchapi"
	"github.com/opensearch-project/opensearch-go/v2/opensearchutil"
)

func CloseSafe(resp *opensearchapi.Response) {
	if resp != nil && resp.Body != nil {
		_, _ = io.ReadAll(resp.Body)
		resp.Body.Close() //nolint,gosec
	}
}

func ESCloseSafe(resp *esapi.Response) {
	if resp != nil && resp.Body != nil {
		_, _ = io.ReadAll(resp.Body)
		resp.Body.Close() //nolint,gosec
	}
}

func CheckError(resp *opensearchapi.Response) error {
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
	if strings.TrimSpace(e.Info.Type) == "" && strings.TrimSpace(e.Info.Reason) == "" {
		return fmt.Errorf(string(data))
	}

	return e
}

func ESCheckError(resp *esapi.Response) error {
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
	if strings.TrimSpace(e.Info.Type) == "" && strings.TrimSpace(e.Info.Reason) == "" {
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

func BuildFilter(ctx context.Context, queryContext *plugin.QueryContext,
	filtersQuals map[string]string,
	accountProvider string, accountID *string, encodedResourceGroupFilters *string, clientType *string) []BoolFilter {
	return BuildFilterWithDefaultFieldName(ctx, queryContext, filtersQuals,
		accountProvider, accountID, encodedResourceGroupFilters, clientType, false)
}

func BuildFilterWithDefaultFieldName(ctx context.Context, queryContext *plugin.QueryContext,
	filtersQuals map[string]string,
	accountProvider string, accountID *string, encodedResourceGroupFilters *string, clientType *string,
	useDefaultFieldName bool) []BoolFilter {
	var filters []BoolFilter
	plugin.Logger(ctx).Trace("BuildFilter", "queryContext.UnsafeQuals", queryContext.UnsafeQuals)

	for _, quals := range queryContext.UnsafeQuals {
		if quals == nil {
			continue
		}

		for _, qual := range quals.GetQuals() {
			fn := qual.GetFieldName()
			fieldName, ok := filtersQuals[fn]
			if !ok {
				if useDefaultFieldName {
					fieldName = fn
				} else {
					continue
				}
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

					filters = append(filters, NewTermsFilter(fieldName, values))
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

	if accountID != nil && len(*accountID) > 0 && *accountID != "all" {
		var accountFieldName string
		switch accountProvider {
		case "aws":
			accountFieldName = "AccountID"
		case "azure":
			accountFieldName = "SubscriptionID"
		}
		filters = append(filters, NewTermFilter("metadata."+accountFieldName, *accountID))
	}

	if encodedResourceGroupFilters != nil && len(*encodedResourceGroupFilters) > 0 {
		resourceGroupFiltersJson, err := base64.StdEncoding.DecodeString(*encodedResourceGroupFilters)
		if err != nil {
			plugin.Logger(ctx).Error("BuildFilter", "resourceGroupFiltersJson", "err", err)
		} else {
			var resourceGroupFilters []ResourceCollectionFilter
			err = json.Unmarshal(resourceGroupFiltersJson, &resourceGroupFilters)
			if err != nil {
				plugin.Logger(ctx).Error("BuildFilter", "resourceGroupFiltersJson", "err", err)
			} else {
				esResourceGroupFilters := make([]BoolFilter, 0, len(resourceGroupFilters)+1)
				if clientType != nil && len(*clientType) > 0 && *clientType == "compliance" {
					taglessTypes := make([]string, 0, len(awsTaglessResourceTypes)+len(azureTaglessResourceTypes))
					for _, awsTaglessResourceType := range awsTaglessResourceTypes {
						taglessTypes = append(taglessTypes, strings.ToLower(awsTaglessResourceType))
					}
					for _, azureTaglessResourceType := range azureTaglessResourceTypes {
						taglessTypes = append(taglessTypes, strings.ToLower(azureTaglessResourceType))
					}
					taglessTermsFilter := NewTermsFilter("metadata.ResourceType", taglessTypes)
					esResourceGroupFilters = append(esResourceGroupFilters, NewBoolMustFilter(taglessTermsFilter))
				}
				for _, resourceGroupFilter := range resourceGroupFilters {
					andFilters := make([]BoolFilter, 0, 5)
					if len(resourceGroupFilter.Connectors) > 0 {
						andFilters = append(andFilters, NewTermsFilter("source_type", resourceGroupFilter.Connectors))
					}
					if len(resourceGroupFilter.AccountIDs) > 0 {
						andFilters = append(andFilters, NewTermsFilter("metadata.AccountID", resourceGroupFilter.AccountIDs))
					}
					if len(resourceGroupFilter.ResourceTypes) > 0 {
						andFilters = append(andFilters, NewTermsFilter("metadata.ResourceType", resourceGroupFilter.ResourceTypes))
					}
					if len(resourceGroupFilter.Regions) > 0 {
						andFilters = append(andFilters,
							NewBoolShouldFilter( // OR
								NewTermsFilter("metadata.Region", resourceGroupFilter.Regions),   // AWS
								NewTermsFilter("metadata.Location", resourceGroupFilter.Regions), // Azure
							),
						)
					}
					if len(resourceGroupFilter.Tags) > 0 {
						for k, v := range resourceGroupFilter.Tags {
							k := strings.ToLower(k)
							v := strings.ToLower(v)
							andFilters = append(andFilters,
								NewNestedFilter("canonical_tags",
									NewBoolMustFilter(
										NewTermFilter("canonical_tags.key", k),
										NewTermFilter("canonical_tags.value", v),
									),
								),
							)
						}
					}
					if len(andFilters) > 0 {
						esResourceGroupFilters = append(esResourceGroupFilters, NewBoolMustFilter(andFilters...))
					}
				}
				if len(esResourceGroupFilters) > 0 {
					filters = append(filters, NewBoolShouldFilter(esResourceGroupFilters...))
				}
			}
		}
	}
	jsonFilters, _ := json.Marshal(filters)
	plugin.Logger(ctx).Trace("BuildFilter", "filters", filters, "jsonFilters", string(jsonFilters))

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
	case *proto.QualValue_InetValue:
		valStr = fmt.Sprintf("%v", v.InetValue.GetCidr())
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

type TermsFilter struct {
	field  string
	values []string
}

func NewTermsFilter(field string, values []string) BoolFilter {
	return TermsFilter{
		field:  field,
		values: values,
	}
}

func (t TermsFilter) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"terms": map[string][]string{
			t.field: t.values,
		},
	})
}

func (t TermsFilter) IsBoolFilter() {}

type TermsSetMatchAllFilter struct {
	field  string
	values []string
}

func NewTermsSetMatchAllFilter(field string, values []string) BoolFilter {
	return TermsSetMatchAllFilter{
		field:  field,
		values: values,
	}
}

func (t TermsSetMatchAllFilter) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"terms_set": map[string]any{
			t.field: map[string]any{
				"terms": t.values,
				"minimum_should_match_script": map[string]string{
					"source": "params.num_terms",
				},
			},
		},
	})
}

func (t TermsSetMatchAllFilter) IsBoolFilter() {}

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
	if len(t.gte) > 0 {
		fieldMap["gte"] = t.gte
	}
	if len(t.lt) > 0 {
		fieldMap["lt"] = t.lt
	}
	if len(t.lte) > 0 {
		fieldMap["lte"] = t.lte
	}

	return json.Marshal(map[string]interface{}{
		"range": map[string]interface{}{
			t.field: fieldMap,
		},
	})
}

func (t RangeFilter) IsBoolFilter() {}

type BoolShouldFilter struct {
	filters []BoolFilter
}

func NewBoolShouldFilter(filters ...BoolFilter) BoolFilter {
	return BoolShouldFilter{
		filters: filters,
	}
}

func (t BoolShouldFilter) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"bool": map[string][]BoolFilter{
			"should": t.filters,
		},
	})
}

func (t BoolShouldFilter) IsBoolFilter() {}

type BoolMustFilter struct {
	filters []BoolFilter
}

func NewBoolMustFilter(filters ...BoolFilter) BoolFilter {
	return BoolMustFilter{
		filters: filters,
	}
}

func (t BoolMustFilter) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"bool": map[string][]BoolFilter{
			"must": t.filters,
		},
	})
}

func (t BoolMustFilter) IsBoolFilter() {}

type NestedFilter struct {
	path  string
	query BoolFilter
}

func NewNestedFilter(path string, query BoolFilter) BoolFilter {
	return NestedFilter{
		path:  path,
		query: query,
	}
}

func (t NestedFilter) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"nested": map[string]any{
			"path":  t.path,
			"query": t.query,
		},
	})
}

func (t NestedFilter) IsBoolFilter() {}

type BaseESPaginator struct {
	client *opensearch.Client

	index    string         // Query index
	query    map[string]any // Query filters
	pageSize int64          // Query page size
	pitID    string         // Query point in time id (Only set if max is greater than size)

	limit   int64 // Maximum documents to query
	queried int64 // Current count of queried documents

	searchAfter []any
	done        bool
}

func NewPaginator(client *opensearch.Client, index string, filters []BoolFilter, limit *int64) (*BaseESPaginator, error) {
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

	if doLog {
		if ctx.Value(context_key.Logger) == nil {
			fmt.Println("Creating search request")
		} else {
			plugin.Logger(ctx).Trace("Creating search request")
		}
	}

	sa := SearchRequest{
		Size:  &p.pageSize,
		Query: p.query,
	}

	if p.limit > p.pageSize && p.pitID != "" {
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

	if doLog {
		if ctx.Value(context_key.Logger) == nil {
			fmt.Println("Creating Opts")
		} else {
			plugin.Logger(ctx).Trace("Creating Opts")
		}
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
		if ctx.Value(context_key.Logger) == nil {
			fmt.Println("SearchWithLog", string(m))
		} else {
			plugin.Logger(ctx).Trace("SearchWithLog", string(m))
		}
	}

	res, err := p.client.Search(opts...)
	defer CloseSafe(res)
	if err != nil {
		var b []byte
		if res != nil {
			b, _ = io.ReadAll(res.Body)
		}
		fmt.Printf("failure while querying es: %v\n%s\n", err, string(b))
		return err
	} else if err := CheckError(res); err != nil {
		if IsIndexNotFoundErr(err) {
			return nil
		}
		var b []byte
		if res != nil {
			b, _ = io.ReadAll(res.Body)
		}
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
func (p *BaseESPaginator) CreatePit(ctx context.Context) (err error) {
	if p.limit <= p.pageSize {
		return nil
	} else if p.pitID != "" {
		return nil
	}

	defer func() {
		if err == nil {
			return
		}
		if ctx.Value(context_key.Logger) == nil {
			fmt.Println("Error", err)
		} else {
			plugin.Logger(ctx).Info("Error %v", err)
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
	if ctx.Value(context_key.Logger) == nil {
		fmt.Println("Creating PIT")
	} else {
		plugin.Logger(ctx).Info("Creating PIT")
	}

	pitRaw, pitRes, err := p.client.PointInTime.Create(
		p.client.PointInTime.Create.WithIndex(p.index),
		p.client.PointInTime.Create.WithKeepAlive(1*time.Minute),
		p.client.PointInTime.Create.WithContext(ctx),
	)
	if ctx.Value(context_key.Logger) == nil {
		fmt.Println("Created PIT", pitRaw, "res=", pitRes, "err=", err)
	} else {
		plugin.Logger(ctx).Info(fmt.Sprintf("Created PIT %v, res=%v, err=%v", pitRaw, pitRes, err))
	}

	defer CloseSafe(pitRaw)
	if err != nil && !strings.Contains(err.Error(), "illegal_argument_exception") {
		return err
	} else if errIf := CheckError(pitRaw); errIf != nil || strings.Contains(errIf.Error(), "illegal_argument_exception") {
		if ctx.Value(context_key.Logger) == nil {
			fmt.Println("Error2", err)
		} else {
			plugin.Logger(ctx).Info("Error2 %v", err)
		}

		CloseSafe(pitRaw)

		if ctx.Value(context_key.Logger) == nil {
			fmt.Println("Going with ES")
		} else {
			plugin.Logger(ctx).Info("Going with ES")
		}
		// try elasticsearch api instead
		req := esapi.OpenPointInTimeRequest{
			Index:     []string{p.index},
			KeepAlive: "1m",
		}
		res, err2 := req.Do(ctx, p.client.Transport)
		defer ESCloseSafe(res)
		if err2 != nil {
			err = errIf
			return err
		} else if err2 := ESCheckError(res); err2 != nil {
			if IsIndexNotFoundErr(err2) {
				return nil
			}
			err = err2
			return err
		} else {
			data, err2 := io.ReadAll(res.Body)
			if err2 != nil {
				err = fmt.Errorf("read response: %w", err2)
				return err
			}
			var pit PointInTimeResponse
			if err2 = json.Unmarshal(data, &pit); err2 != nil {
				err = fmt.Errorf("unmarshal response: %w", err2)
				return err
			}
			p.pitID = pit.ID
			return nil
		}
	}

	if ctx.Value(context_key.Logger) == nil {
		fmt.Println("PIT DONE")
	} else {
		plugin.Logger(ctx).Info("PIT DONE")
	}
	p.pitID = pitRes.PitID

	if ctx.Value(context_key.Logger) == nil {
		fmt.Println("PIT DONE = ", p.pitID)
	} else {
		plugin.Logger(ctx).Info("PIT DONE = %v", p.pitID)
	}
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
	opts := []func(count *opensearchapi.CountRequest){
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
	opts := []func(*opensearchapi.SearchRequest){
		c.es.Search.WithContext(ctx),
		c.es.Search.WithBody(strings.NewReader(query)),
		c.es.Search.WithTrackTotalHits(trackTotalHits),
		c.es.Search.WithIndex(index),
		c.es.Search.WithFilterPath(filterPath...),
	}

	res, err := c.es.Search(opts...)
	defer CloseSafe(res)
	if err != nil {
		var b []byte
		if res != nil {
			b, _ = io.ReadAll(res.Body)
		}
		fmt.Printf("failure while querying es: %v\n%s\n", err, string(b))
		return err
	} else if err := CheckError(res); err != nil {
		if IsIndexNotFoundErr(err) {
			return nil
		}
		var b []byte
		if res != nil {
			b, _ = io.ReadAll(res.Body)
		}
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
	opts := []func(request *opensearchapi.GetRequest){
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

func (c Client) Healthcheck(ctx context.Context) error {
	opts := []func(request *opensearchapi.ClusterHealthRequest){
		c.es.Cluster.Health.WithContext(ctx),
	}

	res, err := c.es.Cluster.Health(opts...)
	defer CloseSafe(res)
	if err != nil {
		return fmt.Errorf("failed to get cluster health due to %v", err)
	} else if err := CheckError(res); err != nil {
		return fmt.Errorf("CheckError: %v", err)
	}

	if res.StatusCode != http.StatusOK {
		return errors.New("failed to get cluster health")
	}

	b, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("failed to read body due to %v", err)
	}

	var js map[string]interface{}
	if err := json.Unmarshal(b, &js); err != nil {
		return fmt.Errorf("failed to unmarshal due to %v", err)
	}

	if js["status"] != "green" && js["status"] != "yellow" {
		return errors.New("unhealthy")
	}

	return nil
}

func (c Client) CreateIndexTemplate(ctx context.Context, name string, body string) error {
	opts := []func(request *opensearchapi.IndicesPutIndexTemplateRequest){
		c.es.Indices.PutIndexTemplate.WithContext(ctx),
	}

	res, err := c.es.Indices.PutIndexTemplate(name, strings.NewReader(body), opts...)
	defer CloseSafe(res)
	if err != nil {
		return err
	} else if err := CheckError(res); err != nil {
		return err
	}

	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated && res.StatusCode != http.StatusNoContent {
		return errors.New("failed to create index template")
	}

	return nil
}

func (c Client) CreateComponentTemplate(ctx context.Context, name string, body string) error {
	opts := []func(request *opensearchapi.ClusterPutComponentTemplateRequest){
		c.es.Cluster.PutComponentTemplate.WithContext(ctx),
	}

	res, err := c.es.Cluster.PutComponentTemplate(name, strings.NewReader(body), opts...)
	defer CloseSafe(res)
	if err != nil {
		return err
	} else if err := CheckError(res); err != nil {
		return err
	}

	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated && res.StatusCode != http.StatusNoContent {
		return errors.New("failed to create component template")
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

func DeleteByQuery(ctx context.Context, es *opensearch.Client, indices []string, query any, opts ...func(*opensearchapi.DeleteByQueryRequest)) (DeleteByQueryResponse, error) {
	defaultOpts := []func(*opensearchapi.DeleteByQueryRequest){
		es.DeleteByQuery.WithContext(ctx),
		es.DeleteByQuery.WithWaitForCompletion(true),
	}

	resp, err := es.DeleteByQuery(
		indices,
		opensearchutil.NewJSONReader(query),
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
