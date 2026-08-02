package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/swayam5342/sandboxd/internal/config"
	"github.com/swayam5342/sandboxd/internal/models"
	"github.com/swayam5342/sandboxd/internal/runner"
)

func noopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// echoLang is a language config that only depends on /bin/cat — always
// present on Linux, so tests that just need "some valid language" don't
// depend on any real toolchain being installed. The run command echoes the
// submitted source file back verbatim, which makes success-path assertions
// straightforward (output == input).
func echoLang() *config.Language {
	return &config.Language{
		ID:                     "echo",
		Name:                   "Echo",
		SourceFilename:         "solution.txt",
		SourceFilenameStrategy: "fixed",
		Check:                  "--version",
		Run: config.Phase{
			Cmd:  "/bin/cat",
			Args: []string{"{{source}}"},
			Limits: config.Limits{
				WallTimeS:    5,
				MemoryKB:     65536,
				MaxProcesses: 10,
			},
		},
	}
}

func javaLikeLang() *config.Language {
	lang := echoLang()
	lang.ID = "javalike"
	lang.SourceFilenameStrategy = "from_request"
	lang.ArtifactFilenameStrategy = "from_request"
	return lang
}

func testConfig(langs ...*config.Language) *config.Config {
	cfg := &config.Config{
		LanguagesByID:     make(map[string]*config.Language),
		KnownLanguages:    make(map[string]bool),
		AllowedBuildFlags: make(map[string][]string),
		AllowedRunFlags:   make(map[string][]string),
	}
	for _, l := range langs {
		cfg.Languages = append(cfg.Languages, *l)
		cfg.LanguagesByID[l.ID] = l
		cfg.KnownLanguages[l.ID] = true
	}
	return cfg
}

// testHandler builds a Handler that never actually needs to exec nsjail for
// the paths under test (validation errors return before Runner.Run is
// called). nsjailPath is set to /bin/echo so Info/Readyz probes are
// deterministic without depending on nsjail being installed.
func testHandler(cfg *config.Config) *Handler {
	return &Handler{
		cfg:        cfg,
		runner:     runner.New(runner.Options{NsjailPath: "/bin/echo", MaxConcurrent: 4, Logger: noopLogger()}),
		logger:     noopLogger(),
		nsjailPath: "/bin/echo",
		version:    "test",
		commit:     "abc123",
	}
}

func doRun(t *testing.T, h *Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/run", strings.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), contextKeyRequestID, "test-request-id"))
	rw := httptest.NewRecorder()
	h.Run(rw, req)
	return rw
}

func decodeError(t *testing.T, rw *httptest.ResponseRecorder) models.ErrorResponse {
	t.Helper()
	var resp models.ErrorResponse
	if err := json.NewDecoder(rw.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error response: %v (body=%s)", err, rw.Body.String())
	}
	return resp
}

// --- Handler.Healthz ---

func TestHealthz(t *testing.T) {
	h := testHandler(testConfig())
	rw := httptest.NewRecorder()
	h.Healthz(rw, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rw.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rw.Code)
	}
	var resp models.HealthResponse
	if err := json.NewDecoder(rw.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("want status=ok, got %q", resp.Status)
	}
}

// --- Handler.Run: request-decoding and validation paths ---

func TestRun_InvalidJSON(t *testing.T) {
	h := testHandler(testConfig(echoLang()))
	rw := doRun(t, h, `{not valid json`)

	if rw.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rw.Code)
	}
	if got := decodeError(t, rw).Error.Code; got != models.ErrInvalidJSON {
		t.Errorf("want %q, got %q", models.ErrInvalidJSON, got)
	}
}

func TestRun_UnknownLanguage(t *testing.T) {
	h := testHandler(testConfig(echoLang()))
	body := `{"language":"cobol","source":"x","tests":[{"stdin":"","expected_stdout":""}]}`
	rw := doRun(t, h, body)

	if rw.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rw.Code)
	}
	if got := decodeError(t, rw).Error.Code; got != models.ErrUnknownLanguage {
		t.Errorf("want %q, got %q", models.ErrUnknownLanguage, got)
	}
}

func TestRun_MissingSource(t *testing.T) {
	h := testHandler(testConfig(echoLang()))
	body := `{"language":"echo","tests":[{"stdin":"","expected_stdout":""}]}`
	rw := doRun(t, h, body)

	if rw.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rw.Code)
	}
	if got := decodeError(t, rw).Error.Code; got != models.ErrMissingField {
		t.Errorf("want %q, got %q", models.ErrMissingField, got)
	}
}

func TestRun_MissingSourceFilename_FromRequestStrategy(t *testing.T) {
	h := testHandler(testConfig(javaLikeLang()))
	body := `{"language":"javalike","source":"x","tests":[{"stdin":"","expected_stdout":""}]}`
	rw := doRun(t, h, body)

	if rw.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rw.Code)
	}
	apiErr := decodeError(t, rw).Error
	if apiErr.Code != models.ErrMissingField || !strings.Contains(apiErr.Message, "source_filename") {
		t.Errorf("want missing_field mentioning source_filename, got %+v", apiErr)
	}
}

func TestRun_MissingArtifactFilename_FromRequestStrategy(t *testing.T) {
	h := testHandler(testConfig(javaLikeLang()))
	body := `{"language":"javalike","source":"x","source_filename":"Main.java","tests":[{"stdin":"","expected_stdout":""}]}`
	rw := doRun(t, h, body)

	if rw.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rw.Code)
	}
	apiErr := decodeError(t, rw).Error
	if apiErr.Code != models.ErrMissingField || !strings.Contains(apiErr.Message, "artifact_filename") {
		t.Errorf("want missing_field mentioning artifact_filename, got %+v", apiErr)
	}
}

func TestRun_BodyTooLarge(t *testing.T) {
	h := testHandler(testConfig(echoLang()))

	huge := `{"language":"echo","source":"` + strings.Repeat("x", 2_000_000) + `","tests":[]}`
	rw := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/run", bytes.NewReader([]byte(huge)))
	req = req.WithContext(context.WithValue(req.Context(), contextKeyRequestID, "test-request-id"))
	// Mirrors what the MaxBodySize middleware does at the real request entry
	// point; called directly here since this test exercises the handler in
	// isolation, without the full middleware chain.
	req.Body = http.MaxBytesReader(rw, req.Body, 512*1024)

	h.Run(rw, req)

	if rw.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("want 413, got %d (body=%s)", rw.Code, rw.Body.String())
	}
	if got := decodeError(t, rw).Error.Code; got != models.ErrSourceTooLarge {
		t.Errorf("want %q, got %q", models.ErrSourceTooLarge, got)
	}
}

// --- Handler.Run: success path (requires a real, executable language) ---

func requireEcho(t *testing.T) {
	t.Helper()
	if _, err := os.Stat("/bin/cat"); err != nil {
		t.Skip("/bin/cat not found — skipping (this test assumes a Linux host)")
	}
}

// requireRealNsjail skips unless a real nsjail binary is available — the
// Info/Readyz tests fake it out (they only exercise the probe call), but
// Handler.Run() actually executes the job through Runner, which needs the
// genuine binary to produce a meaningful result.
func requireRealNsjail(t *testing.T) string {
	t.Helper()
	path := os.Getenv("NSJAIL_PATH")
	if path == "" {
		path = "/usr/sbin/nsjail"
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("nsjail not found at %s — skipping /run success-path test", path)
	}
	return path
}

func testHandlerWithRealNsjail(t *testing.T, cfg *config.Config) *Handler {
	t.Helper()
	nsjail := requireRealNsjail(t)
	return &Handler{
		cfg:        cfg,
		runner:     runner.New(runner.Options{NsjailPath: nsjail, MaxConcurrent: 4, Logger: noopLogger()}),
		logger:     noopLogger(),
		nsjailPath: nsjail,
		version:    "test",
		commit:     "abc123",
	}
}

func TestRun_Success(t *testing.T) {
	h := testHandlerWithRealNsjail(t, testConfig(echoLang()))

	body := `{"language":"echo","source":"hello","tests":[{"stdin":"","expected_stdout":"hello"}]}`
	rw := doRun(t, h, body)

	if rw.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", rw.Code, rw.Body.String())
	}
	var resp models.RunResponse
	if err := json.NewDecoder(rw.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != models.StatusAccepted {
		t.Errorf("want accepted, got %q (tests=%+v)", resp.Status, resp.Tests)
	}
}

// --- Handler.Info / Handler.Readyz ---

func TestInfo(t *testing.T) {
	requireEcho(t)
	h := testHandler(testConfig(echoLang()))
	rw := httptest.NewRecorder()
	h.Info(rw, httptest.NewRequest(http.MethodGet, "/info", nil))

	if rw.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rw.Code)
	}
	var resp models.InfoResponse
	if err := json.NewDecoder(rw.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Languages) != 1 || resp.Languages[0].ID != "echo" {
		t.Errorf("unexpected languages: %+v", resp.Languages)
	}
	if resp.BuildInfo.Version != "test" {
		t.Errorf("want version=test, got %q", resp.BuildInfo.Version)
	}
}

func TestReadyz_AllProbesOK(t *testing.T) {
	requireEcho(t)
	h := testHandler(testConfig(echoLang()))
	rw := httptest.NewRecorder()
	h.Readyz(rw, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rw.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", rw.Code, rw.Body.String())
	}
	var resp models.ReadyzResponse
	if err := json.NewDecoder(rw.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("want status=ok, got %q", resp.Status)
	}
}

func TestReadyz_LanguageProbeFails_Degraded(t *testing.T) {
	badLang := echoLang()
	badLang.Run.Cmd = "/no/such/binary-xyz"
	h := testHandler(testConfig(badLang))

	rw := httptest.NewRecorder()
	h.Readyz(rw, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rw.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", rw.Code)
	}
	var resp models.ReadyzResponse
	if err := json.NewDecoder(rw.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "degraded" {
		t.Errorf("want degraded, got %q", resp.Status)
	}
	if resp.Languages["echo"].OK {
		t.Error("expected echo language probe to be reported as failing")
	}
}
