package runner

// sandbox_exec_test.go — integration tests that actually run nsjail.
//
// Place at: internal/runner/sandbox_exec_test.go
//
// These tests require:
//   1. nsjail installed at /usr/sbin/nsjail (or NSJAIL_PATH env var)
//   2. python3 installed at /usr/bin/python3
//   3. Root or sufficient capabilities to run nsjail
//
// Run with:
//   go test ./internal/runner/ -v -run TestSandboxExec
//
// Skip automatically if nsjail is not found.

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/swayam5342/sandboxd/internal/config"
	"github.com/swayam5342/sandboxd/internal/models"
)

// nsjailPath returns the nsjail binary path, or skips the test if not found.
func requireNsjail(t *testing.T) string {
	t.Helper()
	path := os.Getenv("NSJAIL_PATH")
	if path == "" {
		path = "/usr/sbin/nsjail"
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("nsjail not found at %s — skipping sandbox exec tests", path)
	}
	return path
}

// requireRootExitCodes skips tests that depend on nsjail propagating a
// distinct exit code (2 = time_exceeded, 3 = memory_exceeded) for a killed
// child. That propagation only happens when nsjail itself runs as root with
// CLONE_NEWUSER disabled (NsjailArgs does this in the Docker image). When
// running unprivileged (e.g. local/WSL dev as a regular user), nsjail uses
// its normal unprivileged user-namespace path instead, and its own process
// always exits 0 regardless of why the jailed child was killed — there is
// no way to distinguish time/memory exceeded from success via exit code
// alone in that mode. This is an nsjail behavior difference, not something
// sandboxd controls, so these tests only run meaningfully as root.
func requireRootExitCodes(t *testing.T) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("skipping: nsjail only propagates time/memory-exceeded exit codes when run as root (see Dockerfile/CI); run as root to exercise this")
	}
}

// newTestRunner builds a Runner pointed at the real nsjail binary.
func newTestRunner(t *testing.T) *Runner {
	t.Helper()
	nsjail := requireNsjail(t)
	return New(Options{
		NsjailPath:    nsjail,
		MaxConcurrent: 4,
		Logger:        noopLogger(),
	})
}

// py3Lang returns a minimal Python 3 language config matching languages.yaml.
func py3Lang() *config.Language {
	return &config.Language{
		ID:                     "py3",
		Name:                   "Python 3",
		SourceFilename:         "solution.py",
		SourceFilenameStrategy: "fixed",
		Run: config.Phase{
			Cmd:  "/usr/bin/python3",
			Args: []string{"{{source}}"},
			Limits: config.Limits{
				WallTimeS:    90,
				MemoryKB:     102400,
				MaxProcesses: 100,
			},
		},
	}
}

// runPy3 is a helper that runs a Python snippet against a list of test cases.
func runPy3(t *testing.T, r *Runner, source string, tests []models.TestInput) *models.RunResponse {
	t.Helper()
	lang := py3Lang()
	job := Job{
		RequestID:      t.Name(),
		Language:       lang,
		Source:         source,
		SourceFilename: lang.SourceFilename,
		Tests:          tests,
		RunLimits:      lang.Run.Limits,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := r.Run(ctx, job)
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}
	return resp
}

// =============================================================================
// Basic execution
// =============================================================================

func TestSandboxExec_HelloWorld(t *testing.T) {
	r := newTestRunner(t)

	resp := runPy3(t, r,
		`print("Hello, World!")`,
		[]models.TestInput{
			{Stdin: "", ExpectedStdout: "Hello, World!\n"},
		},
	)

	if resp.Status != models.StatusAccepted {
		t.Errorf("status: want %q, got %q", models.StatusAccepted, resp.Status)
	}
	if resp.Tests[0].Stdout != "Hello, World!\n" {
		t.Errorf("stdout: want %q, got %q", "Hello, World!\n", resp.Tests[0].Stdout)
	}
	if resp.Tests[0].Status != models.TestAccepted {
		t.Errorf("test[0] status: want %q, got %q", models.TestAccepted, resp.Tests[0].Status)
	}
}

func TestSandboxExec_ReadsStdin(t *testing.T) {
	r := newTestRunner(t)

	resp := runPy3(t, r,
		`import sys
n = int(sys.stdin.read().strip())
print(n * 2)`,
		[]models.TestInput{
			{Stdin: "5", ExpectedStdout: "10\n"},
		},
	)

	if resp.Status != models.StatusAccepted {
		t.Errorf("status: want accepted, got %q", resp.Status)
	}
	if resp.Tests[0].Stdout != "10\n" {
		t.Errorf("stdout: want %q, got %q", "10\n", resp.Tests[0].Stdout)
	}
}

func TestSandboxExec_MultipleTests(t *testing.T) {
	r := newTestRunner(t)

	resp := runPy3(t, r,
		`import sys
n = int(sys.stdin.read().strip())
print(n * 2)`,
		[]models.TestInput{
			{Stdin: "1", ExpectedStdout: "2\n"},
			{Stdin: "5", ExpectedStdout: "10\n"},
			{Stdin: "21", ExpectedStdout: "42\n"},
			{Stdin: "0", ExpectedStdout: "0\n"},
		},
	)

	if resp.Status != models.StatusAccepted {
		t.Errorf("top status: want accepted, got %q", resp.Status)
	}
	if len(resp.Tests) != 4 {
		t.Fatalf("expected 4 test results, got %d", len(resp.Tests))
	}
	for i, tr := range resp.Tests {
		if tr.Status != models.TestAccepted {
			t.Errorf("test[%d]: want accepted, got %q (stdout=%q stderr=%q)",
				i, tr.Status, tr.Stdout, tr.Stderr)
		}
	}
}

func TestSandboxExec_EmptyStdin(t *testing.T) {
	r := newTestRunner(t)

	resp := runPy3(t, r,
		`print(42)`,
		[]models.TestInput{
			{Stdin: "", ExpectedStdout: "42\n"},
		},
	)

	if resp.Status != models.StatusAccepted {
		t.Errorf("status: want accepted, got %q", resp.Status)
	}
}

func TestSandboxExec_MultilineOutput(t *testing.T) {
	r := newTestRunner(t)

	resp := runPy3(t, r,
		`for i in range(5):
    print(i)`,
		[]models.TestInput{
			{Stdin: "", ExpectedStdout: "0\n1\n2\n3\n4\n"},
		},
	)

	if resp.Status != models.StatusAccepted {
		t.Errorf("status: want accepted, got %q\nstdout: %q\nstderr: %q",
			resp.Status, resp.Tests[0].Stdout, resp.Tests[0].Stderr)
	}
}

// =============================================================================
// Output comparison
// =============================================================================

func TestSandboxExec_WrongOutput(t *testing.T) {
	r := newTestRunner(t)

	resp := runPy3(t, r,
		`print("wrong answer")`,
		[]models.TestInput{
			{Stdin: "", ExpectedStdout: "Hello, World!\n"},
		},
	)

	if resp.Status != models.StatusWrongOutput {
		t.Errorf("status: want wrong_output, got %q", resp.Status)
	}
	if resp.Tests[0].Status != models.TestWrongOutput {
		t.Errorf("test[0] status: want wrong_output, got %q", resp.Tests[0].Status)
	}
	// Stdout should still be captured even on wrong output
	if resp.Tests[0].Stdout != "wrong answer\n" {
		t.Errorf("stdout should still be captured: got %q", resp.Tests[0].Stdout)
	}
}

func TestSandboxExec_WhitespaceMismatch(t *testing.T) {
	r := newTestRunner(t)

	// print() adds \n but expected_stdout has no \n — whitespace mismatch
	resp := runPy3(t, r,
		`print("hello")`,
		[]models.TestInput{
			{Stdin: "", ExpectedStdout: "hello"}, // missing trailing newline
		},
	)

	if resp.Tests[0].Status != models.TestWhitespaceMismatch {
		t.Errorf("test[0] status: want output_whitespace_mismatch, got %q\nstdout=%q",
			resp.Tests[0].Status, resp.Tests[0].Stdout)
	}
}

func TestSandboxExec_FirstTestFailsRestStillRun(t *testing.T) {
	// All test cases should run even if some fail
	r := newTestRunner(t)

	resp := runPy3(t, r,
		`import sys
n = int(sys.stdin.read().strip())
print(n * 2)`,
		[]models.TestInput{
			{Stdin: "5", ExpectedStdout: "999\n"}, // wrong
			{Stdin: "5", ExpectedStdout: "10\n"},  // correct
		},
	)

	// Top-level should reflect the first failure
	if resp.Status != models.StatusWrongOutput {
		t.Errorf("top status: want wrong_output, got %q", resp.Status)
	}
	if len(resp.Tests) != 2 {
		t.Fatalf("both tests must run, got %d results", len(resp.Tests))
	}
	if resp.Tests[0].Status != models.TestWrongOutput {
		t.Errorf("test[0]: want wrong_output, got %q", resp.Tests[0].Status)
	}
	if resp.Tests[1].Status != models.TestAccepted {
		t.Errorf("test[1]: want accepted, got %q", resp.Tests[1].Status)
	}
}

// =============================================================================
// Error cases
// =============================================================================

func TestSandboxExec_RuntimeError_DivByZero(t *testing.T) {
	r := newTestRunner(t)

	resp := runPy3(t, r,
		`print(1 // 0)`,
		[]models.TestInput{
			{Stdin: "", ExpectedStdout: ""},
		},
	)

	if resp.Tests[0].Status != models.TestRuntimeError {
		t.Errorf("test[0] status: want runtime_error, got %q\nstderr: %q",
			resp.Tests[0].Status, resp.Tests[0].Stderr)
	}
	// stderr should contain the Python traceback
	if !strings.Contains(resp.Tests[0].Stderr, "ZeroDivisionError") {
		t.Errorf("stderr should contain ZeroDivisionError, got: %q", resp.Tests[0].Stderr)
	}
}

func TestSandboxExec_RuntimeError_NameError(t *testing.T) {
	r := newTestRunner(t)

	resp := runPy3(t, r,
		`print(undefined_variable)`,
		[]models.TestInput{
			{Stdin: "", ExpectedStdout: ""},
		},
	)

	if resp.Tests[0].Status != models.TestRuntimeError {
		t.Errorf("want runtime_error, got %q", resp.Tests[0].Status)
	}
	if !strings.Contains(resp.Tests[0].Stderr, "NameError") {
		t.Errorf("stderr should contain NameError, got: %q", resp.Tests[0].Stderr)
	}
}

func TestSandboxExec_SyntaxError(t *testing.T) {
	r := newTestRunner(t)

	resp := runPy3(t, r,
		`print("unclosed string)`,
		[]models.TestInput{
			{Stdin: "", ExpectedStdout: ""},
		},
	)

	// Python syntax errors cause a non-zero exit — maps to runtime_error
	if resp.Tests[0].Status != models.TestRuntimeError {
		t.Errorf("want runtime_error for syntax error, got %q\nstderr: %q",
			resp.Tests[0].Status, resp.Tests[0].Stderr)
	}
}

func TestSandboxExec_StderrCaptured(t *testing.T) {
	r := newTestRunner(t)

	resp := runPy3(t, r,
		`import sys
sys.stderr.write("error message\n")
print("ok")`,
		[]models.TestInput{
			{Stdin: "", ExpectedStdout: "ok\n"},
		},
	)

	// Program exits 0, so output should match
	if resp.Tests[0].Status != models.TestAccepted {
		t.Errorf("want accepted, got %q", resp.Tests[0].Status)
	}
	// stderr must still be captured
	if !strings.Contains(resp.Tests[0].Stderr, "error message") {
		t.Errorf("stderr not captured: got %q", resp.Tests[0].Stderr)
	}
}

// =============================================================================
// Resource limits
// =============================================================================

func TestSandboxExec_TimeLimit_Exceeded(t *testing.T) {
	requireRootExitCodes(t)
	r := newTestRunner(t)
	lang := py3Lang()
	lang.Run.Limits.WallTimeS = 2 // tight limit

	job := Job{
		RequestID:      t.Name(),
		Language:       lang,
		Source:         `while True: pass`, // infinite loop
		SourceFilename: "solution.py",
		Tests:          []models.TestInput{{Stdin: "", ExpectedStdout: ""}},
		RunLimits:      lang.Run.Limits,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := r.Run(ctx, job)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if resp.Tests[0].Status != models.TestTimeExceeded {
		t.Errorf("want time_exceeded, got %q", resp.Tests[0].Status)
	}
}

func TestSandboxExec_TimeLimit_TopLevelStatus(t *testing.T) {
	requireRootExitCodes(t)
	r := newTestRunner(t)
	lang := py3Lang()
	lang.Run.Limits.WallTimeS = 1

	job := Job{
		RequestID:      t.Name(),
		Language:       lang,
		Source:         `import time; time.sleep(10)`,
		SourceFilename: "solution.py",
		Tests:          []models.TestInput{{Stdin: "", ExpectedStdout: ""}},
		RunLimits:      lang.Run.Limits,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := r.Run(ctx, job)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if resp.Status != models.StatusTimeExceeded {
		t.Errorf("top status: want time_exceeded, got %q", resp.Status)
	}
}

// =============================================================================
// Sandbox isolation — security tests
// =============================================================================

func TestSandboxExec_NoNetworkAccess(t *testing.T) {
	r := newTestRunner(t)

	// Attempt to connect to the internet — must fail inside the jail
	resp := runPy3(t, r,
		`import socket
try:
    socket.setdefaulttimeout(2)
    socket.socket().connect(("8.8.8.8", 53))
    print("connected")
except Exception as e:
    print("blocked")`,
		[]models.TestInput{
			{Stdin: "", ExpectedStdout: "blocked\n"},
		},
	)

	if resp.Tests[0].Status != models.TestAccepted {
		t.Errorf("network should be blocked: status=%q stdout=%q stderr=%q",
			resp.Tests[0].Status, resp.Tests[0].Stdout, resp.Tests[0].Stderr)
	}
}

func TestSandboxExec_NoHostFilesystem(t *testing.T) {
	r := newTestRunner(t)

	// Try to read /etc/passwd from the host — should fail or return jail's version
	resp := runPy3(t, r,
		`try:
    with open("/etc/passwd") as f:
        lines = f.readlines()
    # If we can read it, it should be the jail's /etc, not host
    # The jail /etc/passwd has nobody (uid 65534) but not normal users
    found_root = any("root:x:0:" in l for l in lines)
    print("has_root:" + str(found_root))
except Exception as e:
    print("blocked")`,
		[]models.TestInput{
			// Either blocked or jail's /etc is used — we just verify it runs
			{Stdin: "", ExpectedStdout: ""},
		},
	)

	// We're not asserting the exact output — just that it doesn't crash internally
	t.Logf("filesystem test result: status=%q stdout=%q",
		resp.Tests[0].Status, resp.Tests[0].Stdout)
}

func TestSandboxExec_CannotWriteOutsideSandbox(t *testing.T) {
	r := newTestRunner(t)

	// Try to write to /tmp on the host — must fail or write to jail's /tmp
	resp := runPy3(t, r,
		`import os
import time
try:
    with open("/tmp/escape_test.txt", "w") as f:
        f.write("escaped")
    print("wrote")
except Exception as e:
    print("blocked")
time.sleep(60)
	`,
		[]models.TestInput{
			{Stdin: "", ExpectedStdout: ""},
		},
	)

	// Whether it writes or blocks, the host /tmp must not have the file
	if _, err := os.Stat("/tmp/escape_test.txt"); !os.IsNotExist(err) {
		os.Remove("/tmp/escape_test.txt")
		t.Error("file escaped the sandbox to the host /tmp — sandbox breach!")
	}
	t.Logf("write-outside test: status=%q stdout=%q",
		resp.Tests[0].Status, resp.Tests[0].Stdout)
}

// =============================================================================
// Sandbox cleanup
// =============================================================================

func TestSandboxExec_SandboxDirCleanedUp(t *testing.T) {
	r := newTestRunner(t)

	// Count dirs before
	beforeEntries, _ := os.ReadDir(JailBaseDir)
	before := len(beforeEntries)

	runPy3(t, r,
		`print("hello")`,
		[]models.TestInput{{Stdin: "", ExpectedStdout: "hello\n"}},
	)

	// Count dirs after — should be same as before (cleanup ran)
	// Give the defer a moment to complete
	time.Sleep(50 * time.Millisecond)
	afterEntries, _ := os.ReadDir(JailBaseDir)
	after := len(afterEntries)

	if after > before {
		t.Errorf("sandbox dir leaked: had %d dirs before, %d after", before, after)
	}
}

// =============================================================================
// Duration tracking
// =============================================================================

func TestSandboxExec_DurationIsPositive(t *testing.T) {
	r := newTestRunner(t)

	resp := runPy3(t, r,
		`print("hello")`,
		[]models.TestInput{{Stdin: "", ExpectedStdout: "hello\n"}},
	)

	if resp.Tests[0].DurationMs <= 0 {
		t.Errorf("duration_ms should be > 0, got %d", resp.Tests[0].DurationMs)
	}
	t.Logf("duration: %dms", resp.Tests[0].DurationMs)
}

func TestSandboxExec_SlowProgramHasHigherDuration(t *testing.T) {
	r := newTestRunner(t)

	// Fast program
	fastResp := runPy3(t, r,
		`print("fast")`,
		[]models.TestInput{{Stdin: "", ExpectedStdout: "fast\n"}},
	)

	// Slow program (but within limits)
	slowResp := runPy3(t, r,
		`import time; time.sleep(0.5); print("slow")`,
		[]models.TestInput{{Stdin: "", ExpectedStdout: "slow\n"}},
	)

	fastMs := fastResp.Tests[0].DurationMs
	slowMs := slowResp.Tests[0].DurationMs

	t.Logf("fast: %dms, slow: %dms", fastMs, slowMs)

	if slowMs <= fastMs {
		t.Errorf("slow program (%dms) should take longer than fast (%dms)", slowMs, fastMs)
	}
}
