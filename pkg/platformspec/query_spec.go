// query_spec.go
package platformspec

import (
	"errors"
	"fmt"
	"log"
	"regexp"
	"sort" // For sorting detected params and tags
	"strings"

	"gopkg.in/yaml.v3"
)

// Compile regex for parameter detection once
var queryParamRegex = regexp.MustCompile(`\{\{\.(.*?)\}\}`)

// Compile regex for ID validation once (alphanumeric + hyphen/underscore, constraints)
var idFormatRegex = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9]|[_-][a-z0-9])*$`)

// detectQueryParams finds unique template parameters like {{.ParamName}} in a query string.
// Assumes isNonEmpty is defined elsewhere (e.g., common.go)
func detectQueryParams(query string) []string {
	matches := queryParamRegex.FindAllStringSubmatch(query, -1)
	if matches == nil {
		return []string{} // Return empty slice if no matches
	}

	// Use a map to store unique parameter names found
	uniqueParams := make(map[string]struct{})
	for _, match := range matches {
		if len(match) > 1 { // Ensure the capturing group exists
			paramName := strings.TrimSpace(match[1])       // Trim whitespace
			if isNonEmpty(paramName) && paramName != "." { // Ignore empty names like {{.}} or just {{ . }}
				uniqueParams[paramName] = struct{}{}
			}
		}
	}

	// Convert map keys to a sorted slice for consistent output
	paramList := make([]string, 0, len(uniqueParams))
	for param := range uniqueParams {
		paramList = append(paramList, param)
	}
	sort.Strings(paramList) // Sort alphabetically

	return paramList
}

// processQuerySpec handles the parsing and validation specific to query specifications.
// It's called by ProcessSpecification in validator.go.
// Assumes isNonEmpty is defined elsewhere (e.g., common.go)
func (v *defaultValidator) processQuerySpec(data []byte, filePath string, defaultedAPIVersion, originalAPIVersion string) (*QuerySpecification, error) {
	var spec QuerySpecification
	if err := yaml.Unmarshal(data, &spec); err != nil {
		// Provide slightly more context in the parsing error
		return nil, fmt.Errorf("failed to parse YAML file '%s' as query spec: %w", filePath, err)
	}

	// Apply defaulted API version if necessary
	if !isNonEmpty(spec.APIVersion) {
		spec.APIVersion = defaultedAPIVersion
		// Log defaulting only if it actually happens and wasn't already defaulted
		if defaultedAPIVersion == APIVersionV1 && originalAPIVersion != APIVersionV1 {
			log.Printf("Info: Specification '%s' (type: %s) missing 'api-version', defaulting to '%s'.", filePath, spec.Type, APIVersionV1)
		}
	}
	// Ensure parsed APIVersion matches base (and is v1 after defaulting)
	if spec.APIVersion != APIVersionV1 {
		actualVersion := originalAPIVersion
		if isNonEmpty(spec.APIVersion) && spec.APIVersion != defaultedAPIVersion {
			actualVersion = spec.APIVersion
		}
		return nil, fmt.Errorf("query specification '%s': api-version must be '%s' (or omitted to default), got '%s'", filePath, APIVersionV1, actualVersion)
	}
	// Ensure type is set correctly (should be 'query' from base parse)
	if !isNonEmpty(spec.Type) {
		spec.Type = SpecTypeQuery // Default if somehow missing after base parse
		log.Printf("Info: Specification '%s' parsed without 'type', defaulting to '%s'.", filePath, SpecTypeQuery)
	} else if spec.Type != SpecTypeQuery {
		return nil, fmt.Errorf("query specification '%s': type must be '%s', got '%s'", filePath, SpecTypeQuery, spec.Type)
	}

	log.Printf("Validating query specification structure for '%s' (ID: %s)...", filePath, spec.ID)
	if err := v.validateQueryStructure(&spec); err != nil {
		// Wrap error to include file path
		return nil, fmt.Errorf("query specification structure validation failed for '%s': %w", filePath, err)
	}

	// Detect and store parameters after successful validation
	spec.DetectedParams = detectQueryParams(spec.Query)
	log.Printf("Detected query parameters for spec ID '%s': %v", spec.ID, spec.DetectedParams)

	log.Printf("Query specification '%s' (ID: %s) structure validation successful.", filePath, spec.ID)
	// No artifact validation currently defined for queries
	return &spec, nil
}

// validateQueryStructure performs structural checks specific to 'query' specifications.
// Assumes isNonEmpty and validateOptionalTagsMap are defined elsewhere (e.g., common.go)
func (v *defaultValidator) validateQueryStructure(spec *QuerySpecification) error {
	if spec == nil {
		return errors.New("query specification cannot be nil")
	}
	// Define context early for use in error messages
	specContext := "query specification (ID missing)"
	if isNonEmpty(spec.ID) {
		specContext = fmt.Sprintf("query specification (ID: %s)", spec.ID)
	} else {
		return errors.New("query specification: id is required") // ID is mandatory
	}
	// API Version and Type already validated by processQuerySpec

	// --- Required Fields --- (ID checked above)
	lowerID := strings.ToLower(spec.ID)
	if !idFormatRegex.MatchString(lowerID) {
		return fmt.Errorf("%s: id contains invalid characters or format. Allowed: lowercase alphanumeric (a-z, 0-9), hyphen (-), underscore (_). Must start/end with alphanumeric. Symbols (- or _) cannot be consecutive or at start/end", specContext)
	}

	if !isNonEmpty(spec.Title) {
		return fmt.Errorf("%s: title is required", specContext)
	}

	for i, itype := range spec.IntegrationType {
		if !isNonEmpty(itype) {
			return fmt.Errorf("%s: integration_type entry %d cannot be empty", specContext, i)
		}
	}
	if !isNonEmpty(spec.Query) {
		return fmt.Errorf("%s: query text is required and cannot be empty", specContext)
	}

	if spec.Metadata != nil {
		if len(spec.Metadata) == 0 {
			log.Printf("Warning: %s: metadata field exists but is empty.", specContext)
		}
		// Use blank identifier '_' for the unused map value 'val'
		for k, _ := range spec.Metadata {
			if !isNonEmpty(k) {
				return fmt.Errorf("%s: metadata keys cannot be empty", specContext)
			}
			// Value 'val' is intentionally ignored here as empty values are currently allowed.
		}
	}

	// is_view defaults to false - no validation needed

	// Parameters
	if spec.Parameters == nil {
		spec.Parameters = []QueryParameter{} // Ensure it's an empty slice, not nil for consistency
	} else if len(spec.Parameters) > 0 { // Only validate entries if the list is not empty
		paramKeys := make(map[string]struct{})
		for i, param := range spec.Parameters {
			entryContext := fmt.Sprintf("%s parameters entry %d", specContext, i)
			if !isNonEmpty(param.Key) {
				return fmt.Errorf("%s: key is required", entryContext)
			}
			// Value being "" is allowed.
			if _, exists := paramKeys[param.Key]; exists {
				return fmt.Errorf("%s: duplicate parameter key '%s' found", specContext, param.Key)
			}
			paramKeys[param.Key] = struct{}{}
		}
	}

	// --- Tags Validation (Using Helper) ---
	// Calls the helper function assumed defined in common.go
	if err := validateOptionalTagsMap(spec.Tags, specContext); err != nil {
		return err // Error is already contextualized by the helper
	}

	// --- Classification Validation ---
	if spec.Classification != nil {
		if len(spec.Classification) == 0 {
			log.Printf("Warning: %s: classification field exists but is empty.", specContext)
		}
		for i, innerList := range spec.Classification {
			if len(innerList) == 0 {
				return fmt.Errorf("%s: classification entry %d: inner list cannot be empty", specContext, i)
			}
			for j, item := range innerList {
				if !isNonEmpty(item) {
					return fmt.Errorf("%s: classification entry %d, item %d cannot be empty", specContext, i, j)
				}
			}
		}
	}

	// Description and PrimaryTable are optional strings, no validation needed if omitted.

	return nil
} // --- END validateQueryStructure ---

// Note: Assumes defaultValidator struct is defined elsewhere (e.g., validator.go)
// Note: Assumes isNonEmpty func is defined elsewhere (e.g., common.go)
// Note: Assumes validateOptionalTagsMap func is defined elsewhere (e.g., common.go)
// Note: Assumes APIVersionV1 and SpecTypeQuery constants are defined elsewhere (e.g., validator.go)
