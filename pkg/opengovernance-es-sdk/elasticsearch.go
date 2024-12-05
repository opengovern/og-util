package opengovernance

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/elastic/go-elasticsearch/v7/esapi"
	"github.com/turbot/steampipe-plugin-sdk/v5/grpc/proto"
	"github.com/turbot/steampipe-plugin-sdk/v5/plugin"
	"github.com/turbot/steampipe-plugin-sdk/v5/plugin/context_key"

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
		return fmt.Errorf("%s: %s", resp.String(), string(data))
	}
	if strings.TrimSpace(e.Info.Type) == "" && strings.TrimSpace(e.Info.Reason) == "" {
		return fmt.Errorf("%s: %s", resp.String(), string(data))
	}

	return e
}

func LogWarn(ctx context.Context, data string) {
	if ctx.Value(context_key.Logger) == nil {
		fmt.Println(data)
	} else {
		plugin.Logger(ctx).Warn(data)
	}
}

func CheckErrorWithContext(resp *opensearchapi.Response, ctx context.Context) error {
	if !resp.IsError() {
		return nil
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read error: %w", err)
	}

	LogWarn(ctx, fmt.Sprintf("CheckErr data: %s", string(data)))

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

func IsIndexAlreadyExistsErr(err error) bool {
	var e ErrorResponse
	return errors.As(err, &e) &&
		strings.Contains(e.Info.Type, "index_already_exists_exception")
}

type BoolFilter interface {
	IsBoolFilter()
}

func BuildFilter(ctx context.Context, queryContext *plugin.QueryContext,
	filtersQuals map[string]string, integrationID *string, encodedResourceGroupFilters *string, clientType *string) []BoolFilter {
	return BuildFilterWithDefaultFieldName(ctx, queryContext, filtersQuals, integrationID, encodedResourceGroupFilters, clientType, false)
}

func BuildFilterWithDefaultFieldName(ctx context.Context, queryContext *plugin.QueryContext,
	filtersQuals map[string]string, integrationID *string, encodedResourceGroupFilters *string, clientType *string,
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

	if integrationID != nil && len(*integrationID) > 0 && *integrationID != "all" {
		filters = append(filters, NewTermFilter("integration_id", *integrationID))
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
	case *proto.QualValue_TimestampValue:
		valStr = fmt.Sprintf("%v", v.TimestampValue.AsTime().Format(time.RFC3339))
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

type BoolMustNotFilter struct {
	filters []BoolFilter
}

func NewBoolMustNotFilter(filters ...BoolFilter) BoolFilter {
	return BoolMustNotFilter{
		filters: filters,
	}
}

func (t BoolMustNotFilter) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"bool": map[string][]BoolFilter{
			"must_not": t.filters,
		},
	})
}

func (t BoolMustNotFilter) IsBoolFilter() {}

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
