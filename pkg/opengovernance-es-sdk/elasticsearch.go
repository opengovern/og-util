// Copyright (c) 2024 Open Governance, Inc. and Contributors
//
// Portions of this software are licensed as follows:
//
// Any folder or subfolder that includes an explicit LICENSE.md is governed by the license specified in that file.
// All other content in this repository is licensed under the Elastic License v2.0.
// Licensed under the Elastic License v2.0. You may not use this file except in compliance with the Elastic License v2.0. You may obtain a copy of the Elastic License v2.0 at
//
// https://www.elastic.co/licensing/elastic-license
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
//
// -------------------------------------------------------------------------------------------------------
// Overview:
//
// This file is part of the OpenComply platform and its related integrations, serving as a core
// mechanism for querying data in OpenSearch. Through a set of well-defined filter objects and
// helper functions, it converts external "WHERE" clauses or other query expressions (coming from
// integrators such as the AWS or GitHub modules, or from plugin systems like Steampipe) into
// valid JSON for OpenSearch.
//
// 1. Integration with OpenComply
//    - OpenComply orchestrates compliance data collection from multiple sources (AWS, GitHub, or
//      other custom services). Each integration passes high-level filters to this file, which
//      transforms them into OpenSearch queries. That allows OpenComply to unify search, indexing,
//      and compliance checks across diverse data sets.
//
// 2. Query Building and Filter Types
//    - BuildFilter / BuildFilterWithDefaultFieldName
//      Translates external query context (e.g., from Steampipe, or from the integrator’s code) into
//      “BoolFilter” objects like TermFilter, TermsFilter, RangeFilter, etc.
//    - Filter Structs
//       * TermFilter        => Single exact match (e.g. `"term": {"field": "value"}`)
//       * TermsFilter       => Multi-value match (`"terms": {"field": [...]}`)
//       * RangeFilter       => Range queries (`"range": {...}` supporting `gt`, `gte`, `lt`, `lte`)
//       * BoolMustFilter    => AND relationships (`"must": [...]`)
//       * BoolShouldFilter  => OR relationships (`"should": [...]`)
//       * BoolMustNotFilter => NOT relationships (`"must_not": [...]`)
//       * NestedFilter      => Nested field queries (`"nested": {...}`)
//      Each filter object implements MarshalJSON() to produce valid OpenSearch DSL.
//
// 3. Error Handling & Response Utilities
//    - CheckError, CheckErrorWithContext, ESCheckError parse responses for OpenSearch errors
//      (`index_not_found_exception`, etc.).
//    - CloseSafe / ESCloseSafe ensure the response body is consumed and closed to prevent
//      connection leaks.
//
// 4. Additional Operations
//    - Healthcheck checks cluster health, returning an error if "red" or otherwise failing.
//    - CreateIndexTemplate / CreateComponentTemplate set up index or component templates in
//      OpenSearch.
//    - DeleteByQuery wraps `_delete_by_query`, removing documents that match a specific JSON query.
//
// 5. Serving Integrations & External Queries
//    - Various integrators or direct OpenComply modules rely on BuildFilter() to take user-driven
//      or automated compliance queries and transform them into concrete OpenSearch DSL. By
//      handling all DSL generation and error-checking in one place, each integration (AWS, GitHub,
//      custom) can plug into OpenComply’s unified search backend without duplicating logic.
//
// Overall, this file serves as the central bridge between the OpenComply platform’s external queries
// (and integrators) and the data source. By wrapping error-handling, query-building, and
// resource-cleanup in one place, it ensures consistent, reliable interactions with OpenSearch,
// enabling OpenComply and its modules to manage compliance data seamlessly.
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

// textFields indicates which fields are known to be "text" and thus have a ".keyword" subfield.
var textFields = map[string]bool{
	"Description.Name":          true,
	"Description.NameWithOwner": true,
	// Add more as needed...
}

// containsSpecialSymbol checks for punctuation that might need dual (field vs. .keyword).
func containsSpecialSymbol(val string) bool {
	specialChars := "/\\<>,-_()[]="
	return strings.ContainsAny(val, specialChars)
}

// BuildTermCaseInsensitive is used for normal "=" on text fields, producing
// a single text term with "case_insensitive": true.
func buildTermCaseInsensitive(field, value string) map[string]any {
	return map[string]any{
		field: map[string]any{
			"value":            value,
			"case_insensitive": true,
		},
	}
}

// CloseSafe reads and closes the response body to free resources.
func CloseSafe(resp *opensearchapi.Response) {
	if resp != nil && resp.Body != nil {
		_, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
	}
}

// ESCloseSafe is similar but for go-elasticsearch/v7 esapi.Response.
func ESCloseSafe(resp *esapi.Response) {
	if resp != nil && resp.Body != nil {
		_, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
	}
}

// CheckError checks if resp is an error.
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

// LogWarn ...
func LogWarn(ctx context.Context, data string) {
	if ctx.Value(context_key.Logger) == nil {
		fmt.Println(data)
	} else {
		plugin.Logger(ctx).Warn(data)
	}
}

// CheckErrorWithContext ...
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

// ESCheckError ...
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

// IsIndexNotFoundErr ...
func IsIndexNotFoundErr(err error) bool {
	var e ErrorResponse
	return errors.As(err, &e) &&
		strings.EqualFold(e.Info.Type, "index_not_found_exception")
}

// IsIndexAlreadyExistsErr ...
func IsIndexAlreadyExistsErr(err error) bool {
	var e ErrorResponse
	return errors.As(err, &e) &&
		strings.Contains(e.Info.Type, "index_already_exists_exception")
}

// BoolFilter ...
type BoolFilter interface {
	IsBoolFilter()
}

// BuildFilter ...
func BuildFilter(ctx context.Context, queryContext *plugin.QueryContext,
	filtersQuals map[string]string, integrationID *string,
	encodedResourceGroupFilters *string, clientType *string) []BoolFilter {

	return BuildFilterWithDefaultFieldName(ctx, queryContext, filtersQuals,
		integrationID, encodedResourceGroupFilters, clientType, false)
}

// BuildFilterWithDefaultFieldName ...
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

			// if operator is "==", produce a special "TermFilter" with a flag
			// meaning "use EXACT KEYWORD match" (case-sensitive).
			if oprStr == "==" {
				filters = append(filters, NewTermFilterWithExact(fieldName, qualValue(qual.GetValue())))
				continue
			}

			// For the normal "=", ">" etc...
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

	// resource group filters logic ...
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
					taglessTypes := make([]string, 0,
						len(awsTaglessResourceTypes)+len(azureTaglessResourceTypes))
					for _, awsTaglessResourceType := range awsTaglessResourceTypes {
						taglessTypes = append(taglessTypes, strings.ToLower(awsTaglessResourceType))
					}
					for _, azureTaglessResourceType := range azureTaglessResourceTypes {
						taglessTypes = append(taglessTypes, strings.ToLower(azureTaglessResourceType))
					}
					taglessTermsFilter := NewTermsFilter("metadata.ResourceType", taglessTypes)
					esResourceGroupFilters = append(esResourceGroupFilters,
						NewBoolMustFilter(taglessTermsFilter))
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
						esResourceGroupFilters = append(esResourceGroupFilters,
							NewBoolMustFilter(andFilters...))
					}
				}
				if len(esResourceGroupFilters) > 0 {
					filters = append(filters,
						NewBoolShouldFilter(esResourceGroupFilters...))
				}
			}
		}
	}

	jsonFilters, _ := json.Marshal(filters)
	plugin.Logger(ctx).Trace("BuildFilter", "filters", filters,
		"jsonFilters", string(jsonFilters))

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

// TermFilter => for normal "=" logic
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

// TermFilterExact => used if operator is "==" for text fields => EXACT KEYWORD match
type TermFilterExact struct {
	field string
	value string
}

// NewTermFilterWithExact => used in BuildFilter if operator is "=="
func NewTermFilterWithExact(field, value string) BoolFilter {
	return TermFilterExact{
		field: field,
		value: value,
	}
}

// MarshalJSON for normal "=" logic
func (t TermFilter) MarshalJSON() ([]byte, error) {
	isText := textFields[t.field]

	if isText {
		// same logic as your existing code: case_insensitive + possibly dual approach
		if containsSpecialSymbol(t.value) {
			return json.Marshal(map[string]any{
				"bool": map[string]any{
					"should": []map[string]any{
						{
							"term": buildTermCaseInsensitive(t.field, t.value),
						},
						{
							"term": buildTermCaseInsensitive(t.field+".keyword", t.value),
						},
					},
					"minimum_should_match": 1,
				},
			})
		}
		// single text approach => case_insensitive
		return json.Marshal(map[string]any{
			"term": buildTermCaseInsensitive(t.field, t.value),
		})
	}

	// if not text => normal single term
	return json.Marshal(map[string]any{
		"term": map[string]string{
			t.field: t.value,
		},
	})
}

func (t TermFilter) IsBoolFilter() {}

// MarshalJSON for "==", we do EXACT match on field.keyword only if it's text
func (te TermFilterExact) MarshalJSON() ([]byte, error) {
	isText := textFields[te.field]

	if isText {
		// For "==", we do EXACT match => field.keyword => case-sensitive
		// e.g. { "term": { "Description.Name.keyword": "<value>" } }
		keywordField := te.field + ".keyword"
		return json.Marshal(map[string]any{
			"term": map[string]any{
				keywordField: te.value,
			},
		})
	}

	// If not text => fallback to normal single term
	return json.Marshal(map[string]any{
		"term": map[string]string{
			te.field: te.value,
		},
	})
}

func (te TermFilterExact) IsBoolFilter() {}

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
		return fmt.Errorf("failed to get cluster health: %v", err)
	} else if err := CheckError(res); err != nil {
		return err
	}
	if res.StatusCode != http.StatusOK {
		return errors.New("failed to get cluster health")
	}

	b, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("read body: %v", err)
	}
	var js map[string]interface{}
	if err := json.Unmarshal(b, &js); err != nil {
		return fmt.Errorf("unmarshal: %v", err)
	}

	if js["status"] != "green" && js["status"] != "yellow" {
		return errors.New("unhealthy cluster status")
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

func DeleteByQuery(ctx context.Context, es *opensearch.Client, indices []string,
	query any, opts ...func(*opensearchapi.DeleteByQueryRequest)) (DeleteByQueryResponse, error) {

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
