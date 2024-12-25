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

// CloseSafe reads and closes the response body to free resources,
// avoiding "connection leak" issues in the OpenSearch client.
func CloseSafe(resp *opensearchapi.Response) {
	if resp != nil && resp.Body != nil {
		_, _ = io.ReadAll(resp.Body)
		resp.Body.Close() //nolint:gosec
	}
}

// ESCloseSafe is similar to CloseSafe but for the "go-elasticsearch/v7" esapi.Response type.
func ESCloseSafe(resp *esapi.Response) {
	if resp != nil && resp.Body != nil {
		_, _ = io.ReadAll(resp.Body)
		resp.Body.Close() //nolint:gosec
	}
}

// CheckError inspects an opensearchapi.Response to see if it's an error response.
// If so, it unmarshals the body into an ErrorResponse for further inspection.
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
		// If we fail to unmarshal, we return the raw body for debugging.
		return fmt.Errorf("%s: %s", resp.String(), string(data))
	}

	// If the error type/reason is empty, just return the body text.
	if strings.TrimSpace(e.Info.Type) == "" && strings.TrimSpace(e.Info.Reason) == "" {
		return fmt.Errorf("%s: %s", resp.String(), string(data))
	}

	return e
}

// LogWarn is a helper function for consistent logging of warnings. If no logger is found
// in the context, it falls back to fmt.Println.
func LogWarn(ctx context.Context, data string) {
	if ctx.Value(context_key.Logger) == nil {
		fmt.Println(data)
	} else {
		plugin.Logger(ctx).Warn(data)
	}
}

// CheckErrorWithContext does the same error check as CheckError but also logs
// the error body in the event an error is present.
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

// ESCheckError is similar to CheckError, but it works with the go-elasticsearch/v7
// esapi.Response instead of opensearchapi.Response.
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

// IsIndexNotFoundErr returns true if the given error is an index_not_found_exception
// from OpenSearch.
func IsIndexNotFoundErr(err error) bool {
	var e ErrorResponse
	return errors.As(err, &e) &&
		strings.EqualFold(e.Info.Type, "index_not_found_exception")
}

// IsIndexAlreadyExistsErr checks if the error indicates the index already exists.
func IsIndexAlreadyExistsErr(err error) bool {
	var e ErrorResponse
	return errors.As(err, &e) &&
		strings.Contains(e.Info.Type, "index_already_exists_exception")
}

// BoolFilter is a common interface for all filter objects that can be marshaled
// into part of a JSON "bool" query. This includes TermFilter, RangeFilter, etc.
type BoolFilter interface {
	IsBoolFilter()
}

// BuildFilter is the main entry point used by integrators (like Steampipe, or OpenComply modules).
// It reads the queryContext (which includes WHERE clauses) and transforms them
// into an array of BoolFilter objects. The filtersQuals map indicates how to map
// each column name to an actual field path in ES.
func BuildFilter(ctx context.Context, queryContext *plugin.QueryContext,
	filtersQuals map[string]string, integrationID *string, encodedResourceGroupFilters *string, clientType *string) []BoolFilter {
	return BuildFilterWithDefaultFieldName(ctx, queryContext, filtersQuals, integrationID, encodedResourceGroupFilters, clientType, false)
}

// BuildFilterWithDefaultFieldName is a variant of BuildFilter that optionally uses the
// field name directly (useDefaultFieldName = true) if it's not found in filtersQuals.
func BuildFilterWithDefaultFieldName(ctx context.Context, queryContext *plugin.QueryContext,
	filtersQuals map[string]string, integrationID *string, encodedResourceGroupFilters *string, clientType *string,
	useDefaultFieldName bool) []BoolFilter {

	var filters []BoolFilter
	plugin.Logger(ctx).Trace("BuildFilter", "queryContext.UnsafeQuals", queryContext.UnsafeQuals)

	// Loop through each set of Quals from the Steampipe/Integrations query
	for _, quals := range queryContext.UnsafeQuals {
		if quals == nil {
			continue
		}

		for _, qual := range quals.GetQuals() {
			fn := qual.GetFieldName()
			fieldName, ok := filtersQuals[fn]
			if !ok {
				// If the user wants default field usage, fallback to fn
				if useDefaultFieldName {
					fieldName = fn
				} else {
					continue
				}
			}

			// Convert the operator to string form
			var oprStr string
			opr := qual.GetOperator()
			if strOpr, ok := opr.(*proto.Qual_StringValue); ok {
				oprStr = strOpr.StringValue
			}

			// Based on the operator, build the corresponding filter type
			if oprStr == "=" {
				// If the value is a list, use TermsFilter, else TermFilter
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

	// If an integrationID is specified (and it's not "all"), add a TermFilter for that
	if integrationID != nil && len(*integrationID) > 0 && *integrationID != "all" {
		filters = append(filters, NewTermFilter("integration_id", *integrationID))
	}

	// If there's a resourceGroupFilters string, decode and build filters for it
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
							NewBoolShouldFilter(
								NewTermsFilter("metadata.Region", resourceGroupFilter.Regions),
								NewTermsFilter("metadata.Location", resourceGroupFilter.Regions),
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

// qualValue extracts the actual string value from a proto.QualValue (Steampipe representation)
// and converts it to a simple string for ES queries.
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

// containsSpecialSymbol checks if the value has punctuation that might cause tokenization
// or partial matching in a text field. If found, we produce a dual (field + field.keyword) query.
func containsSpecialSymbol(val string) bool {
	// Some special chars: / \ < > , - _ ( ) [ ] =
	specialChars := "/\\<>,-_()[]="
	return strings.ContainsAny(val, specialChars)
}

// buildTermObject is a helper that returns the JSON needed under "term": { field: { ... } }
// ensuring we always include "case_insensitive": true.
func buildTermObject(field, value string) map[string]any {
	return map[string]any{
		field: map[string]any{
			"value":            value,
			"case_insensitive": true, // <-- new
		},
	}
}

// TermFilter represents a single or dual-term approach (if special symbol is found).
type TermFilter struct {
	field string
	value string
}

// NewTermFilter constructs a BoolFilter for an exact match on a single field/value.
func NewTermFilter(field, value string) BoolFilter {
	return TermFilter{
		field: field,
		value: value,
	}
}

// MarshalJSON auto-checks for special symbols. If found, produce a bool.should of
// field vs. field.keyword, each having "case_insensitive": true. Otherwise, single term.
func (t TermFilter) MarshalJSON() ([]byte, error) {
	if containsSpecialSymbol(t.value) {
		// Dual query
		return json.Marshal(map[string]any{
			"bool": map[string]any{
				"should": []map[string]any{
					{
						"term": buildTermObject(t.field, t.value),
					},
					{
						"term": buildTermObject(t.field+".keyword", t.value),
					},
				},
				"minimum_should_match": 1,
			},
		})
	}

	// Single term
	return json.Marshal(map[string]any{
		"term": buildTermObject(t.field, t.value),
	})
}

// IsBoolFilter ensures TermFilter implements the BoolFilter interface.
func (t TermFilter) IsBoolFilter() {}

// TermsFilter is for multi-value queries: "terms": { field: [ val1, val2 ] }
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

// MarshalJSON for TermsFilter. (No "case_insensitive" in ES yet for "terms" queries.)
func (t TermsFilter) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"terms": map[string][]string{
			t.field: t.values,
		},
	})
}
func (t TermsFilter) IsBoolFilter() {}

// TermsSetMatchAllFilter is used for match-all within an array field.
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

// RangeFilter for >, >=, <, <=
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

// BoolShouldFilter => "bool": { "should": [ ... ] }
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

// BoolMustFilter => "bool": { "must": [ ... ] }
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

// BoolMustNotFilter => "bool": { "must_not": [ ... ] }
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

// NestedFilter => "nested": { "path": "...", "query": { ... } }
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

// Healthcheck checks cluster health (green or yellow is acceptable).
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

// CreateIndexTemplate sets up an index template in OpenSearch with the provided name/body.
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

// CreateComponentTemplate sets up a component template in OpenSearch.
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

// DeleteByQueryResponse captures the JSON from the _delete_by_query API.
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

// DeleteByQuery runs _delete_by_query on the specified indices using the given JSON query.
func DeleteByQuery(
	ctx context.Context,
	es *opensearch.Client,
	indices []string,
	query any,
	opts ...func(*opensearchapi.DeleteByQueryRequest),
) (DeleteByQueryResponse, error) {
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
