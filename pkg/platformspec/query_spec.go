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

// Compile regex for ID validation once (alphanumeric + hyphen, no leading/trailing hyphen)
var idFormatRegex = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)

// detectQueryParams finds unique template parameters like {{.ParamName}} in a query string.
func detectQueryParams(query string) []string {
	matches := queryParamRegex.FindAllStringSubmatch(query, -1)
	if matches == nil {
		return []string{} // Return empty slice if no matches
	}

	// Use a map to store unique parameter names found
	uniqueParams := make(map[string]struct{})
	for _, match := range matches {
		if len(match) > 1 { // Ensure the capturing group exists
			paramName := strings.TrimSpace(match[1]) // Trim whitespace
			if paramName != "" {                     // Ignore empty names like {{.}}
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
func (v *defaultValidator) processQuerySpec(data []byte, filePath string, defaultedAPIVersion, originalAPIVersion string) (*QuerySpecification, error) {
	var spec QuerySpecification
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("failed to parse specification file '%s' as query: %w", filePath, err)
	}

	// Apply defaulted API version if necessary
	if !isNonEmpty(spec.APIVersion) {
		spec.APIVersion = defaultedAPIVersion
	}
	// Ensure parsed APIVersion matches base (and is v1 after defaulting)
	if spec.APIVersion != APIVersionV1 {
		return nil, fmt.Errorf("query specification '%s': api-version must be '%s' (or omitted to default), got '%s'", filePath, APIVersionV1, originalAPIVersion)
	}
	// Ensure type is set correctly (should be 'query' from base parse)
	if !isNonEmpty(spec.Type) {
		spec.Type = SpecTypeQuery // Default if somehow missing after base parse
	} else if spec.Type != SpecTypeQuery {
		return nil, fmt.Errorf("query specification '%s': type must be '%s', got '%s'", filePath, SpecTypeQuery, spec.Type)
	}

	log.Println("Validating query specification structure...")
	if err := v.validateQueryStructure(&spec); err != nil {
		return nil, fmt.Errorf("query specification structure validation failed: %w", err)
	}

	// Detect and store parameters after successful validation
	spec.DetectedParams = detectQueryParams(spec.Query)
	log.Printf("Detected query parameters: %v", spec.DetectedParams)

	log.Println("Query specification structure validation successful.")
	// No artifact validation currently defined for queries
	return &spec, nil
}

// validateQueryStructure performs structural checks specific to 'query' specifications.
func (v *defaultValidator) validateQueryStructure(spec *QuerySpecification) error {
	if spec == nil {
		return errors.New("query specification cannot be nil")
	}
	// API Version and Type already validated by processQuerySpec

	// --- Required Fields ---
	if !isNonEmpty(spec.ID) {
		return errors.New("query specification: id is required")
	}
	// Validate ID format (basic domain component rules)
	if !idFormatRegex.MatchString(spec.ID) {
		return fmt.Errorf("query specification (ID: %s): id contains invalid characters or format (must be alphanumeric with hyphens, not starting/ending with hyphen)", spec.ID)
	}

	if !isNonEmpty(spec.Title) {
		return fmt.Errorf("query specification (ID: %s): title is required", spec.ID)
	}
	if len(spec.IntegrationType) == 0 {
		return fmt.Errorf("query specification (ID: %s): integration_type is required and cannot be empty", spec.ID)
	}
	for i, itype := range spec.IntegrationType {
		if !isNonEmpty(itype) {
			return fmt.Errorf("query specification (ID: %s): integration_type entry %d cannot be empty", spec.ID, i)
		}
	}
	if !isNonEmpty(spec.Query) {
		return fmt.Errorf("query specification (ID: %s): query text is required and cannot be empty", spec.ID)
	}

	// --- Optional Fields Validation (if present) ---
	if spec.Metadata != nil {
		if len(spec.Metadata) == 0 {
			// Allow empty map if key exists, but maybe warn?
			log.Printf("Warning: query specification (ID: %s): metadata field exists but is empty.", spec.ID)
		}
		for k, val := range spec.Metadata {
			if !isNonEmpty(k) {
				return fmt.Errorf("query specification (ID: %s): metadata keys cannot be empty", spec.ID)
			}
			if !isNonEmpty(val) {
				return fmt.Errorf("query specification (ID: %s): metadata value for key '%s' cannot be empty", spec.ID, k)
			}
		}
	}

	// is_view defaults to false, no explicit validation needed unless true has implications.

	// Parameters default to empty slice if omitted or explicitly null in YAML
	if spec.Parameters == nil {
		spec.Parameters = []QueryParameter{} // Ensure it's an empty slice, not nil
	} else if len(spec.Parameters) > 0 { // Only validate entries if the list is not empty
		paramKeys := make(map[string]struct{})
		for i, param := range spec.Parameters {
			entryContext := fmt.Sprintf("query specification (ID: %s) parameters entry %d", spec.ID, i)
			if !isNonEmpty(param.Key) {
				return fmt.Errorf("%s: key is required", entryContext)
			}
			// Value being "" is allowed by requirement.
			// if param.Value == "" { // Check if explicitly nil/null if needed, but "" is valid
			// 	return fmt.Errorf("%s (key: %s): value is required (can be empty string \"\")", entryContext, param.Key)
			// }
			if _, exists := paramKeys[param.Key]; exists {
				return fmt.Errorf("query specification (ID: %s): duplicate parameter key '%s' found", spec.ID, param.Key)
			}
			paramKeys[param.Key] = struct{}{}
		}
	}

	// Tags default to nil if omitted or explicitly null
	if spec.Tags != nil {
		if len(spec.Tags) == 0 {
			log.Printf("Warning: query specification (ID: %s): tags field exists but is empty.", spec.ID)
		}
		for key, values := range spec.Tags {
			if !isNonEmpty(key) {
				return fmt.Errorf("query specification (ID: %s): tags keys cannot be empty", spec.ID)
			}
			if len(values) == 0 {
				return fmt.Errorf("query specification (ID: %s): tags value list for key '%s' cannot be empty", spec.ID, key)
			}
			for j, val := range values {
				if !isNonEmpty(val) {
					return fmt.Errorf("query specification (ID: %s): tags value entry %d for key '%s' cannot be empty", spec.ID, j, key)
				}
			}
		}
	}

	// Classification defaults to nil if omitted or explicitly null
	if spec.Classification != nil {
		if len(spec.Classification) == 0 {
			log.Printf("Warning: query specification (ID: %s): classification field exists but is empty.", spec.ID)
		}
		for i, innerList := range spec.Classification {
			if len(innerList) == 0 {
				return fmt.Errorf("query specification (ID: %s): classification entry %d: inner list cannot be empty", spec.ID, i)
			}
			for j, item := range innerList {
				if !isNonEmpty(item) {
					return fmt.Errorf("query specification (ID: %s): classification entry %d, item %d: cannot be empty", spec.ID, i, j)
				}
			}
		}
	}

	// Description and PrimaryTable are optional strings, no validation needed if omitted.

	return nil
}

// GetFlattenedTags extracts tags from a QuerySpecification and returns them as a flat list ["key:value"].
func (v *defaultValidator) GetFlattenedTags(spec *QuerySpecification) []string {
	if spec == nil || spec.Tags == nil || len(spec.Tags) == 0 {
		return []string{} // Return empty slice if spec or tags are nil/empty
	}

	flattened := make([]string, 0)
	// Sort keys for consistent output order (optional but good practice)
	keys := make([]string, 0, len(spec.Tags))
	for k := range spec.Tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		values := spec.Tags[key]
		// Sort values within each key for consistency (optional)
		sort.Strings(values)
		for _, value := range values {
			// Format: "key:value"
			flattened = append(flattened, fmt.Sprintf("%s:%s", key, value))
		}
	}
	return flattened
}
