package validator

import (
	"strings"
	"testing"

	"github.com/swayam5342/sandboxd/internal/models"
)

func validReq() *models.RunRequest {
	return &models.RunRequest{
		Language: "py3",
		Source:   "print(1)",
		Tests:    []models.TestInput{{Stdin: "", ExpectedStdout: "1\n"}},
	}
}

func knownLangs() map[string]bool {
	return map[string]bool{"py3": true}
}

// --- ValidateRunRequest ---

func TestValidateRunRequest_Valid(t *testing.T) {
	if err := ValidateRunRequest(validReq(), knownLangs(), nil, nil, nil, nil); err != nil {
		t.Fatalf("expected no error, got %+v", err)
	}
}

func TestValidateRunRequest_MissingLanguage(t *testing.T) {
	req := validReq()
	req.Language = ""
	err := ValidateRunRequest(req, knownLangs(), nil, nil, nil, nil)
	if err == nil || err.Code != models.ErrMissingField {
		t.Fatalf("want ErrMissingField, got %+v", err)
	}
}

func TestValidateRunRequest_UnknownLanguage(t *testing.T) {
	req := validReq()
	req.Language = "cobol"
	err := ValidateRunRequest(req, knownLangs(), nil, nil, nil, nil)
	if err == nil || err.Code != models.ErrUnknownLanguage {
		t.Fatalf("want ErrUnknownLanguage, got %+v", err)
	}
}

func TestValidateRunRequest_MissingSource(t *testing.T) {
	req := validReq()
	req.Source = ""
	err := ValidateRunRequest(req, knownLangs(), nil, nil, nil, nil)
	if err == nil || err.Code != models.ErrMissingField {
		t.Fatalf("want ErrMissingField, got %+v", err)
	}
}

func TestValidateRunRequest_SourceTooLarge(t *testing.T) {
	orig := MaxSourceBytes
	MaxSourceBytes = 10
	defer func() { MaxSourceBytes = orig }()

	req := validReq()
	req.Source = strings.Repeat("x", 11)
	err := ValidateRunRequest(req, knownLangs(), nil, nil, nil, nil)
	if err == nil || err.Code != models.ErrSourceTooLarge {
		t.Fatalf("want ErrSourceTooLarge, got %+v", err)
	}
}

func TestValidateRunRequest_BadSourceFilename(t *testing.T) {
	req := validReq()
	req.SourceFilename = "../etc/passwd"
	err := ValidateRunRequest(req, knownLangs(), nil, nil, nil, nil)
	if err == nil || err.Code != models.ErrInvalidFilename {
		t.Fatalf("want ErrInvalidFilename, got %+v", err)
	}
}

func TestValidateRunRequest_BadArtifactFilename(t *testing.T) {
	req := validReq()
	req.ArtifactFilename = "/etc/passwd"
	err := ValidateRunRequest(req, knownLangs(), nil, nil, nil, nil)
	if err == nil || err.Code != models.ErrInvalidFilename {
		t.Fatalf("want ErrInvalidFilename, got %+v", err)
	}
}

func TestValidateRunRequest_NoTests(t *testing.T) {
	req := validReq()
	req.Tests = nil
	err := ValidateRunRequest(req, knownLangs(), nil, nil, nil, nil)
	if err == nil || err.Code != models.ErrMissingField {
		t.Fatalf("want ErrMissingField, got %+v", err)
	}
}

func TestValidateRunRequest_TooManyTests(t *testing.T) {
	orig := MaxTests
	MaxTests = 2
	defer func() { MaxTests = orig }()

	req := validReq()
	req.Tests = []models.TestInput{{}, {}, {}}
	err := ValidateRunRequest(req, knownLangs(), nil, nil, nil, nil)
	if err == nil || err.Code != models.ErrTooManyTests {
		t.Fatalf("want ErrTooManyTests, got %+v", err)
	}
}

func TestValidateRunRequest_StdinTooLarge(t *testing.T) {
	orig := MaxStdinBytes
	MaxStdinBytes = 5
	defer func() { MaxStdinBytes = orig }()

	req := validReq()
	req.Tests = []models.TestInput{{Stdin: strings.Repeat("x", 6)}}
	err := ValidateRunRequest(req, knownLangs(), nil, nil, nil, nil)
	if err == nil || err.Code != models.ErrSourceTooLarge {
		t.Fatalf("want ErrSourceTooLarge, got %+v", err)
	}
}

func TestValidateRunRequest_BuildFlagDisallowed(t *testing.T) {
	req := validReq()
	req.Build = &models.PhaseInput{Flags: []string{"-not-allowed"}}
	buildFlags := map[string][]string{"py3": {"-O0"}}
	err := ValidateRunRequest(req, knownLangs(), buildFlags, nil, nil, nil)
	if err == nil || err.Code != models.ErrDisallowedFlag {
		t.Fatalf("want ErrDisallowedFlag, got %+v", err)
	}
}

func TestValidateRunRequest_RunFlagDisallowed(t *testing.T) {
	req := validReq()
	req.Run = &models.PhaseInput{Flags: []string{"-not-allowed"}}
	runFlags := map[string][]string{"py3": {"-safe"}}
	err := ValidateRunRequest(req, knownLangs(), nil, runFlags, nil, nil)
	if err == nil || err.Code != models.ErrDisallowedFlag {
		t.Fatalf("want ErrDisallowedFlag, got %+v", err)
	}
}

func TestValidateRunRequest_BuildAndRunFlagsUseSeparateAllowlists(t *testing.T) {
	req := validReq()
	// A flag that's allowed for build must NOT be silently allowed for run,
	// and vice versa.
	req.Build = &models.PhaseInput{Flags: []string{"-build-only"}}
	req.Run = &models.PhaseInput{Flags: []string{"-run-only"}}
	buildFlags := map[string][]string{"py3": {"-build-only"}}
	runFlags := map[string][]string{"py3": {"-run-only"}}

	if err := ValidateRunRequest(req, knownLangs(), buildFlags, runFlags, nil, nil); err != nil {
		t.Fatalf("expected both flags to validate against their own allowlist, got %+v", err)
	}

	req.Run = &models.PhaseInput{Flags: []string{"-build-only"}}
	if err := ValidateRunRequest(req, knownLangs(), buildFlags, runFlags, nil, nil); err == nil {
		t.Fatal("expected -build-only to be rejected for the run phase")
	}
}

func TestValidateRunRequest_DenylistOverridesAllowlist(t *testing.T) {
	req := validReq()
	req.Run = &models.PhaseInput{Flags: []string{"-C linker=/bin/sh"}}
	runAllow := map[string][]string{"py3": {"-C*"}}
	runDeny := map[string][]string{"py3": {"-C linker=*"}}

	err := ValidateRunRequest(req, knownLangs(), nil, runAllow, nil, runDeny)
	if err == nil || err.Code != models.ErrDeniedFlag {
		t.Fatalf("want ErrDeniedFlag, got %+v", err)
	}
}

// --- ValidateFilename ---

func TestValidateFilename_Empty(t *testing.T) {
	if err := ValidateFilename(""); err != nil {
		t.Errorf("empty filename should be allowed (means 'use default'), got %+v", err)
	}
}

func TestValidateFilename_Valid(t *testing.T) {
	for _, name := range []string{"solution.py", "Main.java", "a_b-c.2.txt"} {
		if err := ValidateFilename(name); err != nil {
			t.Errorf("expected %q to be valid, got %+v", name, err)
		}
	}
}

func TestValidateFilename_TooLong(t *testing.T) {
	if err := ValidateFilename(strings.Repeat("a", MaxFilenameLen+1)); err == nil {
		t.Error("expected error for over-length filename")
	}
}

func TestValidateFilename_LeadingDot(t *testing.T) {
	if err := ValidateFilename(".hidden"); err == nil {
		t.Error("expected error for leading dot")
	}
}

func TestValidateFilename_PathTraversal(t *testing.T) {
	for _, name := range []string{"../etc/passwd", "a/b", "a\\b", "/etc/passwd", "./x"} {
		if err := ValidateFilename(name); err == nil {
			t.Errorf("expected error for path-like filename %q", name)
		}
	}
}

func TestValidateFilename_DisallowedCharacters(t *testing.T) {
	for _, name := range []string{"a b.py", "a;b.py", "a$b.py", "a*b.py"} {
		if err := ValidateFilename(name); err == nil {
			t.Errorf("expected error for filename with disallowed char: %q", name)
		}
	}
}

// --- ValidateFlags / matchesAny ---

func TestValidateFlags_ExactMatch(t *testing.T) {
	if err := ValidateFlags([]string{"-O2"}, []string{"-O2", "-Wall"}, nil); err != nil {
		t.Errorf("expected -O2 to be allowed, got %+v", err)
	}
}

func TestValidateFlags_NotAllowed(t *testing.T) {
	err := ValidateFlags([]string{"-fsanitize=address"}, []string{"-O2"}, nil)
	if err == nil || err.Code != models.ErrDisallowedFlag {
		t.Fatalf("want ErrDisallowedFlag, got %+v", err)
	}
}

func TestValidateFlags_Wildcard(t *testing.T) {
	if err := ValidateFlags([]string{"-std=c11"}, []string{"-std=*"}, nil); err != nil {
		t.Errorf("expected -std=c11 to match wildcard -std=*, got %+v", err)
	}
}

func TestValidateFlags_WildcardDoesNotMatchUnrelatedPrefix(t *testing.T) {
	err := ValidateFlags([]string{"-standalone"}, []string{"-std=*"}, nil)
	// "-standalone" does start with "-std" but not "-std=" — HasPrefix on
	// "-std=" as the prefix means "-standalone" should NOT match.
	if err == nil {
		t.Error("expected -standalone to be rejected against -std=* allowlist")
	}
}

func TestValidateFlags_EmptyAllowlistRejectsEverything(t *testing.T) {
	err := ValidateFlags([]string{"-anything"}, nil, nil)
	if err == nil {
		t.Error("expected rejection with empty/nil allowlist")
	}
}

func TestValidateFlags_MultipleFlags_AllMustBeAllowed(t *testing.T) {
	err := ValidateFlags([]string{"-O2", "-not-allowed"}, []string{"-O2"}, nil)
	if err == nil {
		t.Error("expected rejection when any flag in the list is disallowed")
	}
}

// --- denylist overrides allowlist ---

func TestValidateFlags_DenylistOverridesExactAllowlistMatch(t *testing.T) {
	err := ValidateFlags([]string{"-O2"}, []string{"-O2"}, []string{"-O2"})
	if err == nil || err.Code != models.ErrDeniedFlag {
		t.Fatalf("want ErrDeniedFlag even though -O2 is also allowlisted, got %+v", err)
	}
}

func TestValidateFlags_DenylistOverridesWildcardAllowlistMatch(t *testing.T) {
	// This is the real-world case: Rust's "-C*" allowlist wildcard is broad,
	// but "-C linker=*" within it is dangerous and must stay blocked.
	err := ValidateFlags(
		[]string{"-C linker=/bin/sh"},
		[]string{"-C*"},
		[]string{"-C linker=*"},
	)
	if err == nil || err.Code != models.ErrDeniedFlag {
		t.Fatalf("want ErrDeniedFlag for a denylisted flag within an allowed wildcard, got %+v", err)
	}
}

func TestValidateFlags_DenylistWildcard(t *testing.T) {
	err := ValidateFlags([]string{"-C link-arg=-shared"}, []string{"-C*"}, []string{"-C link-arg=*"})
	if err == nil || err.Code != models.ErrDeniedFlag {
		t.Fatalf("want ErrDeniedFlag, got %+v", err)
	}
}

func TestValidateFlags_NonDeniedFlagWithinAllowedWildcardStillPasses(t *testing.T) {
	// Only the specific denied sub-pattern should be blocked — other flags
	// matching the same broad allowlist wildcard must still work.
	if err := ValidateFlags([]string{"-C opt-level=3"}, []string{"-C*"}, []string{"-C linker=*"}); err != nil {
		t.Errorf("expected -C opt-level=3 to remain allowed, got %+v", err)
	}
}

func TestValidateFlags_EmptyDenylist_NoEffect(t *testing.T) {
	if err := ValidateFlags([]string{"-O2"}, []string{"-O2"}, nil); err != nil {
		t.Errorf("expected nil/empty denylist to reject nothing, got %+v", err)
	}
}

// --- TopLevelStatus ---

func TestTopLevelStatus_BuildFailed(t *testing.T) {
	got := TopLevelStatus(models.BuildFailed, nil)
	if got != models.StatusBuildFailed {
		t.Errorf("want %q, got %q", models.StatusBuildFailed, got)
	}
}

func TestTopLevelStatus_AllAccepted(t *testing.T) {
	tests := []models.TestResult{{Status: models.TestAccepted}, {Status: models.TestAccepted}}
	got := TopLevelStatus(models.BuildOK, tests)
	if got != models.StatusAccepted {
		t.Errorf("want %q, got %q", models.StatusAccepted, got)
	}
}

func TestTopLevelStatus_FirstFailingTestPropagates(t *testing.T) {
	tests := []models.TestResult{
		{Status: models.TestAccepted},
		{Status: models.TestWrongOutput},
		{Status: models.TestTimeExceeded}, // should never be reached
	}
	got := TopLevelStatus(models.BuildOK, tests)
	if got != models.TestWrongOutput {
		t.Errorf("want first failure (%q), got %q", models.TestWrongOutput, got)
	}
}

func TestTopLevelStatus_NoTests(t *testing.T) {
	got := TopLevelStatus(models.BuildOK, nil)
	if got != models.StatusAccepted {
		t.Errorf("want %q for empty test list, got %q", models.StatusAccepted, got)
	}
}

// --- CompareOutput ---

func TestCompareOutput_ExactMatch(t *testing.T) {
	if got := CompareOutput("hello\n", "hello\n"); got != models.TestAccepted {
		t.Errorf("want %q, got %q", models.TestAccepted, got)
	}
}

func TestCompareOutput_WhitespaceMismatch(t *testing.T) {
	if got := CompareOutput("hello\n", "  hello  "); got != models.TestWhitespaceMismatch {
		t.Errorf("want %q, got %q", models.TestWhitespaceMismatch, got)
	}
}

func TestCompareOutput_WrongOutput(t *testing.T) {
	if got := CompareOutput("goodbye", "hello"); got != models.TestWrongOutput {
		t.Errorf("want %q, got %q", models.TestWrongOutput, got)
	}
}
