package validator

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/swayam5342/sandboxd/internal/models"
	"github.com/swayam5342/sandboxd/internal/util"
)

var (
	MaxSourceBytes = util.EnvIntOr("MAX_SOURCE_SIZE", 256*1024)
	MaxTests       = util.EnvIntOr("MAX_TEST_SIZE", 50)
	MaxStdinBytes  = util.EnvIntOr("MAX_STDIN", 64*1024)
	MaxFilenameLen = util.EnvIntOr("MAX_FILENAME_CHAR", 65)
)

func ValidateRunRequest(req *models.RunRequest, knownLanguages map[string]bool, allowedBuildFlags, allowedRunFlags map[string][]string) *models.APIError {

	if err := validateLanguage(req.Language, knownLanguages); err != nil {
		return err
	}
	if err := validateSource(req.Source); err != nil {
		return err
	}
	if req.SourceFilename != "" {
		if err := ValidateFilename(req.SourceFilename); err != nil {
			return err
		}
	}
	if req.ArtifactFilename != "" {
		if err := ValidateFilename(req.ArtifactFilename); err != nil {
			return err
		}
	}
	if err := validateTests(req.Tests); err != nil {
		return err
	}
	if req.Build != nil && len(req.Build.Flags) > 0 {
		if err := ValidateFlags(req.Build.Flags, allowedBuildFlags[req.Language]); err != nil {
			return err
		}
	}
	if req.Run != nil && len(req.Run.Flags) > 0 {
		if err := ValidateFlags(req.Run.Flags, allowedRunFlags[req.Language]); err != nil {
			return err
		}
	}
	return nil // all good
}

func validateLanguage(lang string, knownLanguages map[string]bool) *models.APIError {
	if lang == "" {
		return &models.APIError{
			Code:    models.ErrMissingField,
			Message: "language is required",
		}
	}
	if !knownLanguages[lang] {
		return &models.APIError{
			Code:    models.ErrUnknownLanguage,
			Message: fmt.Sprintf("unknown language: %q — check GET /info for supported languages", lang),
		}
	}
	return nil
}

func validateSource(source string) *models.APIError {
	if source == "" {
		return &models.APIError{
			Code:    models.ErrMissingField,
			Message: "source is required",
		}
	}
	if len(source) > MaxSourceBytes {
		return &models.APIError{
			Code:    models.ErrSourceTooLarge,
			Message: fmt.Sprintf("source exceeds maximum size of %d bytes", MaxSourceBytes),
		}
	}
	return nil
}

func validateTests(tests []models.TestInput) *models.APIError {
	if len(tests) == 0 {
		return &models.APIError{
			Code:    models.ErrMissingField,
			Message: "at least one test case is required",
		}
	}
	if len(tests) > MaxTests {
		return &models.APIError{
			Code:    models.ErrTooManyTests,
			Message: fmt.Sprintf("too many tests: got %d, max is %d", len(tests), MaxTests),
		}
	}
	for i, t := range tests {
		if len(t.Stdin) > MaxStdinBytes {
			return &models.APIError{
				Code:    models.ErrSourceTooLarge,
				Message: fmt.Sprintf("tests[%d].stdin exceeds maximum size of %d bytes", i, MaxStdinBytes),
			}
		}
	}
	return nil
}

func ValidateFilename(filename string) *models.APIError {
	errBadFilename := &models.APIError{
		Code:    models.ErrInvalidFilename,
		Message: "source_filename must be a single path component with no separators, no leading dot, and under 64 characters",
	}
	if filename == "" {
		return nil
	}
	if len(filename) > MaxFilenameLen {
		return errBadFilename
	}
	if strings.HasPrefix(filename, ".") {
		return errBadFilename
	}
	if filepath.Base(filename) != filename {
		return errBadFilename
	}
	if strings.ContainsAny(filename, "/\\") {
		return errBadFilename
	}
	for _, ch := range filename {
		if !unicode.IsLetter(ch) && !unicode.IsDigit(ch) && ch != '.' && ch != '-' && ch != '_' {
			return errBadFilename
		}
	}

	return nil
}

func ValidateFlags(flags []string, allowlist []string) *models.APIError {
	for _, flag := range flags {
		if !isFlagAllowed(flag, allowlist) {
			return &models.APIError{
				Code:    models.ErrDisallowedFlag,
				Message: fmt.Sprintf("flag %q is not on the allowlist for this language", flag),
			}
		}
	}
	return nil
}

func isFlagAllowed(flag string, allowlist []string) bool {
	for _, allowed := range allowlist {
		if strings.HasSuffix(allowed, "*") {
			// Wildcard: check that the flag starts with the prefix before "*"
			prefix := strings.TrimSuffix(allowed, "*")
			if strings.HasPrefix(flag, prefix) {
				return true
			}
		} else {
			if flag == allowed {
				return true
			}
		}
	}
	return false
}
func TopLevelStatus(buildStatus string, tests []models.TestResult) string {
	if buildStatus != models.BuildOK {
		return models.StatusBuildFailed
	}
	for _, t := range tests {
		if t.Status != models.TestAccepted {
			// Map test status to top-level status.
			// They share the same string values, so this is a direct pass-through.
			return t.Status
		}
	}
	return models.StatusAccepted
}
func CompareOutput(actual, expected string) string {
	if actual == expected {
		return models.TestAccepted
	}
	if strings.TrimSpace(actual) == strings.TrimSpace(expected) {
		return models.TestWhitespaceMismatch
	}
	return models.TestWrongOutput
}
