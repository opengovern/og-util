package platformspec

import (
	"fmt"
	"log"
	"time"

	"github.com/github/go-spdx/v2/spdxexp"
)

// initializeSPDX attempts to pre-load/check the SPDX license list.
func initializeSPDX() {
	// licenses.IsValidLicenseID implicitly handles loading/caching.
	// Use a known valid license ID for the check.
	// ValidateLicenses returns bool, []string. We only need the bool here.
	valid, _ := spdxexp.ValidateLicenses([]string{"MIT"})
	if !valid {
		// This might log internal errors from the library if fetching fails.
		log.Println("Warning: Initial check for SPDX license 'MIT' failed. SPDX validation might be unavailable or inaccurate if the license list couldn't be loaded.")
	} else {
		log.Println("SPDX license list appears available for validation.")
	}
}

// validateMetadata performs structural, date format, and SPDX license validation on a Metadata object.
// This is specific to Plugin and standalone Task specifications.
func (v *defaultValidator) validateMetadata(meta *Metadata, context string) error {
	if meta == nil {
		return fmt.Errorf("%s: metadata section cannot be nil", context)
	}
	if !isNonEmpty(meta.Author) {
		return fmt.Errorf("%s: metadata.author is required", context)
	}
	if !isNonEmpty(meta.PublishedDate) {
		return fmt.Errorf("%s: metadata.published-date is required", context)
	}
	// Validate PublishedDate Format (YYYY-MM-DD)
	if _, err := time.Parse(PublishedDateFormat, meta.PublishedDate); err != nil {
		return fmt.Errorf("%s: invalid metadata.published-date format '%s' (expected '%s'): %w", context, meta.PublishedDate, PublishedDateFormat, err)
	}
	if !isNonEmpty(meta.Contact) {
		return fmt.Errorf("%s: metadata.contact is required", context)
	}
	if !isNonEmpty(meta.License) {
		return fmt.Errorf("%s: metadata.license is required", context)
	}
	// Validate License against SPDX list using github/go-spdx library
	valid, invalidList := spdxexp.ValidateLicenses([]string{meta.License})
	if !valid {
		// Provide helpful error message including link to SPDX website and the invalid part found
		return fmt.Errorf("%s: metadata.license '%s' is not a valid SPDX license identifier (invalid parts: %v). See https://spdx.org/licenses/", context, meta.License, invalidList)
	}
	// Optional fields (Description, Website) don't need presence checks.
	return nil
}
