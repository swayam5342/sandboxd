package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/swayam5342/sandboxd/internal/models"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

// --- RequestID ---

func TestRequestID_SetsHeaderAndContext(t *testing.T) {
	var idFromCtx string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		idFromCtx = requestIDFrom(r.Context())
	})

	rw := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	RequestID(next).ServeHTTP(rw, req)

	header := rw.Header().Get("X-Request-ID")
	if header == "" {
		t.Fatal("expected X-Request-ID header to be set")
	}
	if idFromCtx != header {
		t.Errorf("context request id (%q) should match response header (%q)", idFromCtx, header)
	}
}

func TestRequestIDFrom_MissingContext_ReturnsUnknown(t *testing.T) {
	if got := requestIDFrom(context.Background()); got != "unknown" {
		t.Errorf("want %q, got %q", "unknown", got)
	}
}

// --- RequireAPIKey ---

func TestRequireAPIKey_EmptyKey_AllowsAll(t *testing.T) {
	rw := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/run", nil)
	RequireAPIKey("")(okHandler()).ServeHTTP(rw, req)

	if rw.Code != http.StatusOK {
		t.Fatalf("want 200 when no API key configured, got %d", rw.Code)
	}
}

func TestRequireAPIKey_MissingHeader_Rejected(t *testing.T) {
	rw := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/run", nil)
	RequireAPIKey("secret")(okHandler()).ServeHTTP(rw, req)

	if rw.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rw.Code)
	}
}

func TestRequireAPIKey_WrongKey_Rejected(t *testing.T) {
	rw := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/run", nil)
	req.Header.Set("Authorization", "Bearer wrong-key")
	RequireAPIKey("secret")(okHandler()).ServeHTTP(rw, req)

	if rw.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rw.Code)
	}
}

func TestRequireAPIKey_CorrectKey_Allowed(t *testing.T) {
	rw := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/run", nil)
	req.Header.Set("Authorization", "Bearer secret")
	RequireAPIKey("secret")(okHandler()).ServeHTTP(rw, req)

	if rw.Code != http.StatusOK {
		t.Fatalf("want 200 with correct key, got %d", rw.Code)
	}
}

func TestRequireAPIKey_NoBearerPrefix_Rejected(t *testing.T) {
	rw := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/run", nil)
	req.Header.Set("Authorization", "secret") // missing "Bearer " prefix
	RequireAPIKey("secret")(okHandler()).ServeHTTP(rw, req)

	if rw.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rw.Code)
	}
}

// --- MaxBodySize ---

func TestMaxBodySize_UnderLimit_Passes(t *testing.T) {
	var bodyRead string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 5)
		n, _ := r.Body.Read(buf)
		bodyRead = string(buf[:n])
	})

	rw := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/run", strings.NewReader("hello"))
	MaxBodySize(1024)(next).ServeHTTP(rw, req)

	if bodyRead != "hello" {
		t.Errorf("want body %q, got %q", "hello", bodyRead)
	}
}

func TestMaxBodySize_OverLimit_Errors(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err == nil {
			t.Error("expected read error for oversized body")
		}
	})

	rw := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/run", strings.NewReader("0123456789"))
	MaxBodySize(5)(next).ServeHTTP(rw, req)
}

// --- RecoverPanic ---

func TestRecoverPanic_RecoversAndReturns500(t *testing.T) {
	panicky := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	})

	rw := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	// Must not panic out of the test itself.
	RecoverPanic(noopLogger())(panicky).ServeHTTP(rw, req)

	if rw.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", rw.Code)
	}
	var resp models.ErrorResponse
	if err := json.NewDecoder(rw.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, rw.Body.String())
	}
	if resp.Error.Code != "internal_error" {
		t.Errorf("want internal_error, got %q", resp.Error.Code)
	}
}

func TestRecoverPanic_NoPanic_PassesThrough(t *testing.T) {
	rw := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	RecoverPanic(noopLogger())(okHandler()).ServeHTTP(rw, req)

	if rw.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rw.Code)
	}
}

// --- Logger middleware ---

func TestLogger_CapturesStatusCode(t *testing.T) {
	notFound := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	rw := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/nope", nil)
	// Just verifying it doesn't panic and passes the status through — the
	// log line itself goes to the noop logger's discard writer.
	Logger(noopLogger())(notFound).ServeHTTP(rw, req)

	if rw.Code != http.StatusNotFound {
		t.Fatalf("want 404 passed through, got %d", rw.Code)
	}
}
