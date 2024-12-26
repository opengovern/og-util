// Copyright (c) 2024 Open Governance, Inc. and Contributors
//
// Portions of this software are licensed as follows:
//
// Any folder or subfolder that includes an explicit LICENSE.md is governed by the license specified in that file.
// All other content in this repository is licensed under the Elastic License v2.0.
// Licensed under the Elastic License v2.0. You may not use this file except in compliance with the Elastic License v2.0. You may obtain a copy of the Elastic License v2.0 at
//
// https://www.elastic.co/licensing/elastic-license
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES
// OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS
// BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF
// OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
//
// -------------------------------------------------------------------------------------------------------
//
// This file is part of the OpenComply platform. It transforms external query expressions
// (from integrators like Steampipe) into OpenSearch queries.
//
// -------------------------------------------------------------------------------------------------------

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

// containsSpecialSymbol checks for punctuation that might cause
// partial tokenization in a text field, thus needing the dual approach (field & field.keyword).
func containsSpecialSymbol(val string) bool {
	specialChars := "/\\<>,-_()[]=:;"
	return strings.ContainsAny(val, specialChars)
}

// buildCaseInsensitiveTerm constructs:
// "term": {
//   "<field>": {
//     "value": "<value>",
//     "case_insensitive": true
//   }
// }
func buildCaseInsensitiveTerm(field, value string) map[string]any {
	return map[string]any{
		field: map[string]any{
			"value":            value,
			"case_insensitive": true,
		},
	}
}

// attemptParseDate tries multiple date/time formats, including time zone variants.
// Returns (true, theParsedTime) if success, or (false, time.Time{}) if not.
func attemptParseDate(val string) (bool, time.Time) {
	formats := []string{
		time.RFC3339,                  // 2006-01-02T15:04:05Z07:00
		time.RFC3339Nano,             // includes fractions of seconds
		"2006-01-02",                 // date only
		"2006-01-02 15:04:05",        // date + time
		"2006-01-02T15:04:05.999999Z",// more variants
		"2006-01-02T15:04:05Z07:00",   // date/time + offset
	}
	for _, f := range formats {
		if t, err := time.Parse(f, val); err == nil {
			return true, t
		}
	}
	return false, time.Time{}
}

// CloseSafe reads & closes the response body to avoid leaks.
func CloseSafe(resp *opensearchapi.Response) {
	if resp != nil && resp.Body != nil {
		_, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
	}
}

// ESCloseSafe does the same for go-elasticsearch/v7 esapi.Response.
func ESCloseSafe(resp *esapi.Response) {
	if resp != nil && resp.Body != nil {
		_, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
	}
}

// CheckError reads an opensearchapi.Response and tries to decode an error.
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

// LogWarn logs a warning either via plugin.Logger() or fmt.Println if none.
func LogWarn(ctx context.Context, data string) {
	if ctx.Value(context_key.Logger) == nil {
		fmt.Println(data)
	} else {
		plugin.Logger(ctx).Warn(data)
	}
}

// CheckErrorWithContext logs error details and returns an error if found.
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

// ESCheckError does the same for the esapi.Response type.
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

// IsIndexNotFoundErr checks if error is index_not_found_exception
func IsIndexNotFoundErr(err error) bool {
	var e ErrorResponse
	return errors.As(err, &e) && strings.EqualFold(e.Info.Type, "index_not_found_exception")
}

// IsIndexAlreadyExistsErr checks if error says index is already created
func IsIndexAlreadyExistsErr(err error) bool {
	var e ErrorResponse
	return errors.As(err, &e) && strings.Contains(e.Info.Type, "index_already_exists_exception")
}

// BoolFilter is an interface for all filters (TermFilter, RangeFilter, etc.)
type BoolFilter interface {
	IsBoolFilter()
}

// BuildFilter is the main entry used by integrators, producing a slice of BoolFilter.
func BuildFilter(ctx context.Context, queryContext *plugin.QueryContext,
	filtersQuals map[string]string, integrationID *string,
	encodedResourceGroupFilters *string, clientType *string) []BoolFilter {

	return BuildFilterWithDefaultFieldName(ctx, queryContext, filtersQuals,
		integrationID, encodedResourceGroupFilters, clientType, false)
}

// BuildFilterWithDefaultFieldName optionally falls back to the user’s field name.
func BuildFilterWithDefaultFieldName(ctx context.Context, queryContext *plugin.QueryContext,
	filtersQuals map[string]string, integrationID *string,
	encodedResourceGroupFilters *string, clientType *string,
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
					stringVals := make([]string, 0, len(vals))
					for _, v := range vals {
						stringVals = append(stringVals, qualValue(v))
					}
					filters = append(filters, NewTermsFilter(fieldName, stringVals))
				} else {
					val := qualValue(qual.GetValue())
					filters = append(filters, NewTermFilter(fieldName, val))
				}
			}
			if oprStr == ">" {
				filters = append(filters, NewRangeFilter(fieldName,
					qualValue(qual.GetValue()), "", "", ""))
			}
			if oprStr == ">=" {
				filters = append(filters, NewRangeFilter(fieldName, "",
					qualValue(qual.GetValue()), "", ""))
			}
			if oprStr == "<" {
				filters = append(filters, NewRangeFilter(fieldName, "", "",
					qualValue(qual.GetValue()), ""))
			}
			if oprStr == "<=" {
				filters = append(filters, NewRangeFilter(fieldName, "", "", "",
					qualValue(qual.GetValue())))
			}
		}
	}

	if integrationID != nil && len(*integrationID) > 0 && *integrationID != "all" {
		filters = append(filters, NewTermFilter("integration_id", *integrationID))
	}

	// If there's an encodedResourceGroupFilters => decode & handle
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

				if clientType != nil && *clientType == "compliance" {
					taglessTypes := make([]string, 0, len(awsTaglessResourceTypes)+len(azureTaglessResourceTypes))
					for _, awsTaglessResourceType := range awsTaglessResourceTypes {
						taglessTypes = append(taglessTypes, strings.ToLower(awsTaglessResourceType))
					}
					for _, azureTaglessResourceType := range azureTaglessResourceTypes {
						taglessTypes = append(taglessTypes, strings.ToLower(azureTaglessResourceType))
					}
					esResourceGroupFilters = append(esResourceGroupFilters,
						NewBoolMustFilter(NewTermsFilter("metadata.ResourceType", taglessTypes)))
				}
				for _, rgf := range resourceGroupFilters {
					andFilters := make([]BoolFilter, 0, 5)

					if len(rgf.Connectors) > 0 {
						andFilters = append(andFilters, NewTermsFilter("source_type", rgf.Connectors))
					}
					if len(rgf.AccountIDs) > 0 {
						andFilters = append(andFilters, NewTermsFilter("metadata.AccountID", rgf.AccountIDs))
					}
					if len(rgf.ResourceTypes) > 0 {
						andFilters = append(andFilters, NewTermsFilter("metadata.ResourceType", rgf.ResourceTypes))
					}
					if len(rgf.Regions) > 0 {
						andFilters = append(andFilters,
							NewBoolShouldFilter(
								NewTermsFilter("metadata.Region", rgf.Regions),
								NewTermsFilter("metadata.Location", rgf.Regions),
							),
						)
					}
					if len(rgf.Tags) > 0 {
						for k, v := range rgf.Tags {
							kLower := strings.ToLower(k)
							vLower := strings.ToLower(v)
							andFilters = append(andFilters,
								NewNestedFilter("canonical_tags",
									NewBoolMustFilter(
										NewTermFilter("canonical_tags.key", kLower),
										NewTermFilter("canonical_tags.value", vLower),
									),
								),
							)
						}
					}
					if len(andFilters) > 0 {
						esResourceGroupFilters = append(esResourceGroupFilters,
							NewBoolMustFilter(andFilters...))
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

// qualValue reuses your function, unchanged:
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
		// convert e.g. 2006-01-02T15:04:05Z07:00
		valStr = fmt.Sprintf("%v", v.TimestampValue.AsTime().Format(time.RFC3339))
	default:
		valStr = qual.String()
	}
	return valStr
}

// TermFilter: we do type inference with new logic around leading zeros, date/time parse, etc.
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
	val := t.value

	// 1) Check for bool: "true"/"false"
	lower := strings.ToLower(val)
	if lower == "true" || lower == "false" {
		// single term => no case_insensitive
		return singleTerm(t.field, val), nil
	}

	// 2) Attempt date/time parse
	okDate, _ := attemptParseDate(val)
	if okDate {
		// single term => no case_insensitive
		return singleTerm(t.field, val), nil
	}

	// 3) Check if numeric, ignoring leading zeros => if "0", "1", etc. is fine. But "0001" => treat as text
	// We'll define "numeric" as all digits & either length == 1 or no leading zero.
	if isAllDigits(val) {
		if len(val) == 1 {
			// e.g. "7", that is numeric
			return singleTerm(t.field, val), nil
		}
		// length > 1 => check leading digit
		if val[0] != '0' {
			// e.g. "1234" => numeric
			return singleTerm(t.field, val), nil
		}
		// else => "0001" => treat as text
	}

	// 4) Treat as text => do "case_insensitive"
	// If special punctuation => add .keyword as well
	if containsSpecialSymbol(val) {
		// dual approach
		return json.Marshal(map[string]any{
			"bool": map[string]any{
				"should": []map[string]any{
					{
						"term": buildCaseInsensitiveTerm(t.field, val),
					},
					{
						"term": buildCaseInsensitiveTerm(t.field+".keyword", val),
					},
				},
				"minimum_should_match": 1,
			},
		})
	}

	// single text approach
	b, _ := json.Marshal(map[string]any{
		"term": buildCaseInsensitiveTerm(t.field, val),
	})
	return b, nil
}

func (t TermFilter) IsBoolFilter() {}

// isAllDigits checks if the entire string is 0-9
func isAllDigits(val string) bool {
	if val == "" {
		return false
	}
	for _, c := range val {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// singleTerm is a helper returning:  { "term": { field: "<val>" } } as JSON
func singleTerm(field, val string) []byte {
	data, _ := json.Marshal(map[string]any{
		"term": map[string]string{
			field: val,
		},
	})
	return data
}

// TermsFilter remains the same
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

// RangeFilter ...
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
	if t.gt != "" {
		fieldMap["gt"] = t.gt
	}
	if t.gte != "" {
		fieldMap["gte"] = t.gte
	}
	if t.lt != "" {
		fieldMap["lt"] = t.lt
	}
	if t.lte != "" {
		fieldMap["lte"] = t.lte
	}
	return json.Marshal(map[string]any{
		"range": map[string]interface{}{
			t.field: fieldMap,
		},
	})
}
func (t RangeFilter) IsBoolFilter() {}

// BoolShouldFilter ...
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

// Healthcheck ...
func (c Client) Healthcheck(ctx context.Context) error {
	opts := []func(request *opensearchapi.ClusterHealthRequest){
		c.es.Cluster.Health.WithContext(ctx),
	}
	res, err := c.es.Cluster.Health(opts...)
	defer CloseSafe(res)
	if err != nil {
		return fmt.Errorf("failed to get cluster health: %v", err)
	} else if err := CheckError(res); err != nil {
		return fmt.Errorf("CheckError: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		return errors.New("failed to get cluster health")
	}
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("failed to read body: %v", err)
	}
	var js map[string]interface{}
	if err := json.Unmarshal(b, &js); err != nil {
		return fmt.Errorf("failed to unmarshal: %v", err)
	}
	if js["status"] != "green" && js["status"] != "yellow" {
		return errors.New("unhealthy")
	}
	return nil
}

// CreateIndexTemplate ...
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

// CreateComponentTemplate ...
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

// DeleteByQueryResponse ...
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

// DeleteByQuery ...
func DeleteByQuery(ctx context.Context,
	es *opensearch.Client,
	indices []string,
	query any,
	opts ...func(*opensearchapi.DeleteByQueryRequest)) (DeleteByQueryResponse, error) {

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
	} else if cerr := CheckError(resp); cerr != nil {
		if IsIndexNotFoundErr(cerr) {
			return DeleteByQueryResponse{}, nil
		}
		return DeleteByQueryResponse{}, cerr
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return DeleteByQueryResponse{}, fmt.Errorf("read response: %w", err)
	}
	var response DeleteByQueryResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return DeleteByQueryResponse{}, fmt.Errorf("unmarshal response: %w", err)
	}
	return response, nil
}
