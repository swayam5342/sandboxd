package runner

// Place this file at: internal/runner/sandbox_test.go
//
// These tests cover sandbox internals without needing nsjail running.
// They test: expandArgs, mapExitStatus, buildNsjailArgs, createSandboxDir,
// SweepOrphanDirs, limitedWriter, truncateIfNeeded, and Runner concurrency.
//
// Run with: go test ./internal/runner/ -v -run TestSandbox
// Or all:   go test ./internal/runner/ -v

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/swayam5342/sandboxd/internal/config"
	"github.com/swayam5342/sandboxd/internal/models"
)

// =============================================================================
// expandArgs — pure function, no side effects
// =============================================================================

func TestExpandArgs_SourceOnly(t *testing.T) {
	// Python: /usr/bin/python3 {{source}}
	got := expandArgs([]string{"{{source}}"}, "/solution.py", "", nil, "/", "")
	assertSlice(t, got, []string{"/solution.py"})
}

func TestExpandArgs_BuildPhase_NoFlags(t *testing.T) {
	// C build without flags: gcc -o {{artifact}} {{source}}
	args := []string{"-o", "{{artifact}}", "{{source}}"}
	got := expandArgs(args, "/solution.c", "/solution", nil, "/", "solution")
	assertSlice(t, got, []string{"-o", "/solution", "/solution.c"})
}

func TestExpandArgs_BuildPhase_WithFlags(t *testing.T) {
	// C++ build with flags: {{flags}} -o {{artifact}} {{source}}
	args := []string{"{{flags}}", "-o", "{{artifact}}", "{{source}}"}
	got := expandArgs(args, "/solution.cpp", "/solution", []string{"-O2", "-std=c++17"}, "/", "solution")
	assertSlice(t, got, []string{"-O2", "-std=c++17", "-o", "/solution", "/solution.cpp"})
}

func TestExpandArgs_Flags_ExpandsToSeparateArgs(t *testing.T) {
	// Critical: {{flags}} must expand to multiple args, not one joined string
	args := []string{"{{flags}}", "{{source}}"}
	got := expandArgs(args, "/solution.c", "", []string{"-O2", "-Wall", "-lm"}, "/", "")
	assertSlice(t, got, []string{"-O2", "-Wall", "-lm", "/solution.c"})
}

func TestExpandArgs_Flags_EmptyExpandsToNothing(t *testing.T) {
	// {{flags}} with nil/empty flags must not leave an empty string element
	args := []string{"{{flags}}", "-o", "{{artifact}}", "{{source}}"}
	got := expandArgs(args, "/s.c", "/s", nil, "/", "s")
	assertSlice(t, got, []string{"-o", "/s", "/s.c"})
}

func TestExpandArgs_Workdir_Java(t *testing.T) {
	// Java run: java -cp {{workdir}} {{artifact}}
	args := []string{"-cp", "{{workdir}}", "{{artifact}}"}
	got := expandArgs(args, "/Solution.java", "/Solution", nil, "/", "Solution")
	assertSlice(t, got, []string{"-cp", "/", "/Solution"})
}

func TestExpandArgs_EmptyArgs(t *testing.T) {
	// C/C++ run phase has empty args — cmd is {{artifact}} handled separately
	got := expandArgs([]string{}, "/solution.py", "", nil, "/", "")
	if len(got) != 0 {
		t.Errorf("empty template args: want [], got %v", got)
	}
}

func TestExpandArgs_LiteralPassthrough(t *testing.T) {
	// Args with no placeholders pass through unchanged
	args := []string{"-version", "--help"}
	got := expandArgs(args, "/s.py", "", nil, "/", "")
	assertSlice(t, got, []string{"-version", "--help"})
}

func TestExpandArgs_ArtifactName_BareNameNoLeadingSlash(t *testing.T) {
	// Java run: java -cp {{workdir}} {{artifact_name}} — needs the bare
	// class name (no leading "/"), unlike {{artifact}} which is a path.
	args := []string{"-cp", "{{workdir}}", "{{artifact_name}}"}
	got := expandArgs(args, "/Solution.java", "/Solution", nil, "/", "Solution")
	assertSlice(t, got, []string{"-cp", "/", "Solution"})
}

// =============================================================================
// mapExitStatus
// =============================================================================

func TestMapExitStatus_NilError_Run(t *testing.T) {
	got := mapExitStatus(nil, false, 0, config.Limits{}, 0)
	if got != models.TestAccepted {
		t.Errorf("want %q, got %q", models.TestAccepted, got)
	}
}

func TestMapExitStatus_NilError_Build(t *testing.T) {
	got := mapExitStatus(nil, true, 0, config.Limits{}, 0)
	if got != models.BuildOK {
		t.Errorf("want %q, got %q", models.BuildOK, got)
	}
}

func TestMapExitStatus_NonExitError_Run(t *testing.T) {
	// Non-ExitError (e.g. binary not found) → internal_error
	err := fmt.Errorf("exec: not found")
	got := mapExitStatus(err, false, 0, config.Limits{}, 0)
	if got != models.TestInternalError {
		t.Errorf("want %q, got %q", models.TestInternalError, got)
	}
}

func TestMapExitStatus_NonExitError_Build(t *testing.T) {
	err := fmt.Errorf("exec: not found")
	got := mapExitStatus(err, true, 0, config.Limits{}, 0)
	if got != models.BuildInternalError {
		t.Errorf("want %q, got %q", models.BuildInternalError, got)
	}
}

// exitErrWithCode runs a real subprocess that exits with the given code, so
// tests get a genuine *exec.ExitError (ExitError wraps unexported fields
// that can't be constructed by hand).
func exitErrWithCode(t *testing.T, code int) error {
	t.Helper()
	err := exec.Command("sh", "-c", fmt.Sprintf("exit %d", code)).Run()
	if err == nil {
		t.Fatalf("expected exit code %d to produce an error", code)
	}
	return err
}

// --- exit code 2/3 disambiguation: nsjail reuses these codes both for its
// own time/memory-limit kills AND as a plain passthrough of a child that
// exited normally with that same code. mapExitStatus must not misreport an
// ordinary exit(2)/exit(3) as time/memory_exceeded. ---

func TestMapExitStatus_ExitCode2_DurationBelowLimit_IsRuntimeError(t *testing.T) {
	// A program that calls exit(2) quickly, well under the wall-time limit,
	// was not killed for exceeding it — this must not be time_exceeded.
	err := exitErrWithCode(t, 2)
	got := mapExitStatus(err, false, 50, config.Limits{WallTimeS: 10}, 0)
	if got != models.TestRuntimeError {
		t.Errorf("want %q, got %q", models.TestRuntimeError, got)
	}
}

func TestMapExitStatus_ExitCode2_DurationAtLimit_IsTimeExceeded(t *testing.T) {
	err := exitErrWithCode(t, 2)
	got := mapExitStatus(err, false, 10_000, config.Limits{WallTimeS: 10}, 0)
	if got != models.TestTimeExceeded {
		t.Errorf("want %q, got %q", models.TestTimeExceeded, got)
	}
}

func TestMapExitStatus_ExitCode2_Build_AlwaysBuildFailed(t *testing.T) {
	// Build phase has no distinct time_exceeded status — any exit(2) during
	// build reports as a plain build failure regardless of duration.
	err := exitErrWithCode(t, 2)
	got := mapExitStatus(err, true, 50, config.Limits{WallTimeS: 10}, 0)
	if got != models.BuildFailed {
		t.Errorf("want %q, got %q", models.BuildFailed, got)
	}
}

func TestMapExitStatus_ExitCode3_MemoryBelowLimit_IsRuntimeError(t *testing.T) {
	// A program that calls exit(3) while using little memory was not
	// killed for exceeding the memory limit — this must not be
	// memory_exceeded.
	err := exitErrWithCode(t, 3)
	got := mapExitStatus(err, false, 0, config.Limits{MemoryKB: 102400}, 1024)
	if got != models.TestRuntimeError {
		t.Errorf("want %q, got %q", models.TestRuntimeError, got)
	}
}

func TestMapExitStatus_ExitCode3_MemoryAtLimit_IsMemoryExceeded(t *testing.T) {
	err := exitErrWithCode(t, 3)
	got := mapExitStatus(err, false, 0, config.Limits{MemoryKB: 102400}, 102400)
	if got != models.TestMemoryExceeded {
		t.Errorf("want %q, got %q", models.TestMemoryExceeded, got)
	}
}

func TestHitWallTime(t *testing.T) {
	cases := []struct {
		durationMs int64
		wallTimeS  int
		want       bool
	}{
		{durationMs: 100, wallTimeS: 10, want: false},
		{durationMs: 8_900, wallTimeS: 10, want: false}, // just under the 90% threshold
		{durationMs: 9_100, wallTimeS: 10, want: true},  // just over the 90% threshold
		{durationMs: 10_000, wallTimeS: 10, want: true},
	}
	for _, c := range cases {
		if got := hitWallTime(c.durationMs, c.wallTimeS); got != c.want {
			t.Errorf("hitWallTime(%d, %d) = %v, want %v", c.durationMs, c.wallTimeS, got, c.want)
		}
	}
}

func TestHitMemoryLimit(t *testing.T) {
	cases := []struct {
		peakKB, limitKB int64
		want            bool
	}{
		{peakKB: 100, limitKB: 1000, want: false},
		{peakKB: 890, limitKB: 1000, want: false},
		{peakKB: 910, limitKB: 1000, want: true},
		{peakKB: 1000, limitKB: 1000, want: true},
	}
	for _, c := range cases {
		if got := hitMemoryLimit(c.peakKB, int(c.limitKB)); got != c.want {
			t.Errorf("hitMemoryLimit(%d, %d) = %v, want %v", c.peakKB, c.limitKB, got, c.want)
		}
	}
}

func TestHitMemoryLimit_ZeroLimit_NeverTrue(t *testing.T) {
	// A zero/unset memory limit must not be treated as "any usage exceeds it".
	if hitMemoryLimit(999_999, 0) {
		t.Error("want false for a zero memory limit")
	}
}

// =============================================================================
// buildNsjailArgs
// =============================================================================

func TestBuildNsjailArgs_Structure(t *testing.T) {
	limits := config.Limits{WallTimeS: 5, MemoryKB: 65536, MaxProcesses: 50}
	args := NsjailArgs("/usr/sbin/nsjail", "/tmp/jail-123", limits, "/usr/bin/python3", []string{"/solution.py"})

	// First element must be the nsjail binary
	if args[0] != "/usr/sbin/nsjail" {
		t.Errorf("args[0]: want /usr/sbin/nsjail, got %q", args[0])
	}

	// Must be one-shot mode
	assertPair(t, args, "--mode", "o")

	// Chroot must be the sandbox dir
	assertPair(t, args, "--chroot", "/tmp/jail-123")

	// Working directory must be jail root
	assertPair(t, args, "--cwd", "/")

	// Time limit must match config
	assertPair(t, args, "--time_limit", "5")

	// User command and args must be after --
	sepIdx := indexOf(args, "--")
	if sepIdx == -1 {
		t.Fatal("-- separator not found")
	}
	tail := args[sepIdx+1:]
	if len(tail) < 2 || tail[0] != "/usr/bin/python3" || tail[1] != "/solution.py" {
		t.Errorf("after --: want [/usr/bin/python3 /solution.py], got %v", tail)
	}
}

func TestBuildNsjailArgs_RequiredBindMounts(t *testing.T) {
	limits := config.Limits{WallTimeS: 5}
	args := NsjailArgs("/usr/sbin/nsjail", "/tmp/jail", limits, "/usr/bin/python3", nil)

	for _, mount := range []string{"/usr", "/bin"} {
		if !hasPair(args, "--bindmount_ro", mount) {
			t.Errorf("missing bind mount: --bindmount_ro %s", mount)
		}
	}
	// /proc is mounted fresh (scoped to the jail's own PID namespace), not
	// bind-mounted from the host — bind-mounting host /proc would leak the
	// host's process list into the jail.
	if !hasPair(args, "--mount", "none:/proc:proc:") {
		t.Error("expected a fresh, namespace-scoped /proc mount")
	}
	// /etc is deliberately NOT bind-mounted wholesale from the host (that
	// would leak host passwd/hostname/etc) — only narrow toolchain config
	// paths are conditionally mounted when present.
	if hasPair(args, "--bindmount_ro", "/etc") {
		t.Error("host /etc must not be bind-mounted wholesale")
	}
}

func TestBuildNsjailArgs_PathEnv(t *testing.T) {
	limits := config.Limits{WallTimeS: 5}
	args := NsjailArgs("/usr/sbin/nsjail", "/tmp/jail", limits, "/usr/bin/python3", nil)
	if !hasPair(args, "--env", "PATH=/usr/bin:/bin:/usr/sbin:/sbin") {
		t.Error("PATH env must be injected into the jail")
	}
}

func TestBuildNsjailArgs_RWFlag(t *testing.T) {
	// --rw needed so gcc can write the compiled artifact
	limits := config.Limits{WallTimeS: 5}
	args := NsjailArgs("/usr/sbin/nsjail", "/tmp/jail", limits, "/usr/bin/gcc", nil)
	if !contains(args, "--rw") {
		t.Error("--rw flag required for build phase")
	}
}

func TestBuildNsjailArgs_NetworkDisabled(t *testing.T) {
	limits := config.Limits{WallTimeS: 5}
	args := NsjailArgs("/usr/sbin/nsjail", "/tmp/jail", limits, "/usr/bin/python3", nil)
	if !contains(args, "--iface_no_lo") {
		t.Error("--iface_no_lo must disable network inside jail")
	}
}

func TestBuildNsjailArgs_SeparatorBeforeCmd(t *testing.T) {
	limits := config.Limits{WallTimeS: 9}
	args := NsjailArgs("/usr/sbin/nsjail", "/tmp/jail", limits, "/usr/bin/python3", nil)

	sepIdx := indexOf(args, "--")
	cmdIdx := indexOf(args, "/usr/bin/python3")

	if sepIdx == -1 {
		t.Fatal("-- separator not found")
	}
	if cmdIdx <= sepIdx {
		t.Errorf("user cmd must come AFTER --: sep=%d cmd=%d", sepIdx, cmdIdx)
	}
}

// =============================================================================
// createSandboxDir
// =============================================================================

func TestCreateSandboxDir_CreatesDirectory(t *testing.T) {
	dir, err := createSandboxDir("test")
	if err != nil {
		t.Fatalf("createSandboxDir() error: %v", err)
	}
	defer os.RemoveAll(dir)

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("dir does not exist after creation: %v", err)
	}
	if !info.IsDir() {
		t.Error("created path is not a directory")
	}
}

func TestCreateSandboxDir_UnderJailBaseDir(t *testing.T) {
	dir, err := createSandboxDir("test")
	if err != nil {
		t.Fatalf("createSandboxDir() error: %v", err)
	}
	defer os.RemoveAll(dir)

	if !strings.HasPrefix(dir, JailBaseDir) {
		t.Errorf("sandbox dir %q must be under JailBaseDir %q", dir, JailBaseDir)
	}
}

func TestCreateSandboxDir_IsUnique(t *testing.T) {
	dir1, err := createSandboxDir("test")
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	defer os.RemoveAll(dir1)

	dir2, err := createSandboxDir("tes")
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	defer os.RemoveAll(dir2)

	if dir1 == dir2 {
		t.Errorf("two calls returned same dir: %q", dir1)
	}
}

func TestCreateSandboxDir_HasTmpSubdir(t *testing.T) {
	// javac and rustc need /tmp inside the jail during compilation
	dir, err := createSandboxDir("test")
	if err != nil {
		t.Fatalf("createSandboxDir() error: %v", err)
	}
	defer os.RemoveAll(dir)

	tmpPath := filepath.Join(dir, "tmp")
	info, err := os.Stat(tmpPath)
	if err != nil {
		t.Fatalf("sandbox /tmp not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("sandbox /tmp is not a directory")
	}
}

func TestCreateSandboxDir_Permissions(t *testing.T) {
	dir, err := createSandboxDir("test")
	if err != nil {
		t.Fatalf("createSandboxDir() error: %v", err)
	}
	defer os.RemoveAll(dir)

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	// 0700 = only the server process can enter the directory
	if perm := info.Mode().Perm(); perm != 0700 {
		t.Errorf("permissions: want 0700, got %04o", perm)
	}
}

// =============================================================================
// SweepOrphanDirs
// =============================================================================

func TestSweepOrphanDirs_RemovesOldDirs(t *testing.T) {
	if err := os.MkdirAll(JailBaseDir, 0700); err != nil {
		t.Fatalf("setup: %v", err)
	}

	orphan := filepath.Join(JailBaseDir, "sweep-test-old-99999")
	if err := os.Mkdir(orphan, 0700); err != nil {
		t.Fatalf("create orphan: %v", err)
	}

	// Back-date the directory so it looks old
	oldTime := time.Now().Add(-(OrphanMaxAge + 5*time.Minute))
	if err := os.Chtimes(orphan, oldTime, oldTime); err != nil {
		os.RemoveAll(orphan)
		t.Fatalf("chtimes: %v", err)
	}

	SweepOrphanDirs(&testLogger{t: t})

	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		os.RemoveAll(orphan)
		t.Error("old orphan dir should have been swept")
	}
}

func TestSweepOrphanDirs_KeepsFreshDirs(t *testing.T) {
	if err := os.MkdirAll(JailBaseDir, 0700); err != nil {
		t.Fatalf("setup: %v", err)
	}

	fresh := filepath.Join(JailBaseDir, "sweep-test-fresh-88888")
	if err := os.Mkdir(fresh, 0700); err != nil {
		t.Fatalf("create fresh dir: %v", err)
	}
	defer os.RemoveAll(fresh)

	SweepOrphanDirs(&testLogger{t: t})

	if _, err := os.Stat(fresh); os.IsNotExist(err) {
		t.Error("fresh dir should NOT be swept")
	}
}

func TestSweepOrphanDirs_NoPanicOnMissingBase(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("SweepOrphanDirs panicked: %v", r)
		}
	}()
	// Should just return without panicking when base dir doesn't exist
	SweepOrphanDirs(&testLogger{t: t})
}

// =============================================================================
// limitedWriter
// =============================================================================

func TestLimitedWriter_UnderLimit(t *testing.T) {
	var buf strings.Builder
	lw := &limitedWriter{w: &buf, limit: 100}

	n, err := lw.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 5 {
		t.Errorf("n: want 5, got %d", n)
	}
	if buf.String() != "hello" {
		t.Errorf("buf: want %q, got %q", "hello", buf.String())
	}
}

func TestLimitedWriter_ExceedsLimit_TruncatesContent(t *testing.T) {
	var buf strings.Builder
	lw := &limitedWriter{w: &buf, limit: 3}

	// Write 5 bytes, only 3 should be stored
	n, err := lw.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Must return original len — child must not see a broken pipe
	if n != 5 {
		t.Errorf("n must be original len 5, got %d", n)
	}
	if buf.String() != "hel" {
		t.Errorf("buf: want %q, got %q", "hel", buf.String())
	}
}

func TestLimitedWriter_AfterLimit_DropsAll(t *testing.T) {
	var buf strings.Builder
	lw := &limitedWriter{w: &buf, limit: 3}

	lw.Write([]byte("hel")) // fill limit exactly
	n, err := lw.Write([]byte("lo more data"))
	if err != nil {
		t.Fatalf("unexpected error after limit: %v", err)
	}
	if n != 12 { // original len of "lo more data"
		t.Errorf("n must be original len 12, got %d", n)
	}
	if buf.String() != "hel" {
		t.Errorf("buf should be unchanged at %q, got %q", "hel", buf.String())
	}
}

func TestLimitedWriter_MultipleWrites_Accumulate(t *testing.T) {
	var buf strings.Builder
	lw := &limitedWriter{w: &buf, limit: 10}

	lw.Write([]byte("hello"))
	lw.Write([]byte(" world!!!"))

	if buf.String() != "hello worl" {
		t.Errorf("buf: want %q, got %q", "hello worl", buf.String())
	}
}

// =============================================================================
// truncateIfNeeded
// =============================================================================

func TestTruncateIfNeeded_ShortString(t *testing.T) {
	got := truncateIfNeeded("hello")
	if got != "hello" {
		t.Errorf("short string should be unchanged, got %q", got)
	}
}

func TestTruncateIfNeeded_AtLimit_GetsMarker(t *testing.T) {
	s := strings.Repeat("a", MaxOutputBytes)
	got := truncateIfNeeded(s)
	if !strings.HasSuffix(got, TruncationMarker) {
		t.Error("string at MaxOutputBytes should have truncation marker")
	}
}

func TestTruncateIfNeeded_BelowLimit_NoMarker(t *testing.T) {
	s := strings.Repeat("a", MaxOutputBytes-1)
	got := truncateIfNeeded(s)
	if strings.Contains(got, TruncationMarker) {
		t.Error("string below MaxOutputBytes should not have truncation marker")
	}
}

// =============================================================================
// nsjailCgroupWorks
// =============================================================================

func TestNsjailCgroupWorks_DoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("nsjailCgroupWorks panicked: %v", r)
		}
	}()
	result := nsjailCgroupWorks()
	t.Logf("nsjailCgroupWorks() = %v (host-dependent)", result)
}

// =============================================================================
// Runner struct
// =============================================================================

func TestRunner_New_DefaultConcurrency(t *testing.T) {
	r := New(Options{NsjailPath: "/usr/sbin/nsjail", MaxConcurrent: 0, Logger: noopLogger()})
	if r.MaxConcurrent() <= 0 {
		t.Errorf("default MaxConcurrent should be > 0, got %d", r.MaxConcurrent())
	}
}

func TestRunner_New_ExplicitConcurrency(t *testing.T) {
	r := New(Options{NsjailPath: "/usr/sbin/nsjail", MaxConcurrent: 4, Logger: noopLogger()})
	if r.MaxConcurrent() != 4 {
		t.Errorf("MaxConcurrent: want 4, got %d", r.MaxConcurrent())
	}
}

func TestRunner_InFlight_StartsAtZero(t *testing.T) {
	r := New(Options{NsjailPath: "/usr/sbin/nsjail", Logger: noopLogger()})
	if r.InFlight() != 0 {
		t.Errorf("InFlight at start: want 0, got %d", r.InFlight())
	}
}

func TestRunner_Run_RespectsContextCancel(t *testing.T) {
	r := New(Options{NsjailPath: "/usr/sbin/nsjail", MaxConcurrent: 1, Logger: noopLogger()})

	// Fill the one semaphore slot manually so Run has to wait
	r.sem <- struct{}{}
	defer func() { <-r.sem }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before calling Run

	_, err := r.Run(ctx, Job{Language: &config.Language{ID: "py3"}})
	if err != context.Canceled {
		t.Errorf("want context.Canceled, got %v", err)
	}
}

func TestRunner_Run_RespectsContextTimeout(t *testing.T) {
	r := New(Options{NsjailPath: "/usr/sbin/nsjail", MaxConcurrent: 1, Logger: noopLogger()})

	// Fill the semaphore slot
	r.sem <- struct{}{}
	defer func() { <-r.sem }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()

	time.Sleep(10 * time.Millisecond)

	_, err := r.Run(ctx, Job{Language: &config.Language{ID: "py3"}})
	if err == nil {
		t.Error("expected timeout error, got nil")
	}
}

// =============================================================================
// Helpers
// =============================================================================

func assertSlice(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("len: got %d want %d\n  got:  %v\n  want: %v", len(got), len(want), got, want)
		return
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d]: got %q, want %q", i, got[i], want[i])
		}
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func indexOf(slice []string, item string) int {
	for i, s := range slice {
		if s == item {
			return i
		}
	}
	return -1
}

// hasPair returns true if two consecutive elements appear in the slice.
// Used to verify --flag value pairs e.g. --bindmount_ro /usr
func hasPair(slice []string, a, b string) bool {
	for i := 0; i < len(slice)-1; i++ {
		if slice[i] == a && slice[i+1] == b {
			return true
		}
	}
	return false
}

// assertPair fails if the pair a,b does not appear consecutively.
func assertPair(t *testing.T, slice []string, a, b string) {
	t.Helper()
	if !hasPair(slice, a, b) {
		t.Errorf("pair %q %q not found in %v", a, b, slice)
	}
}

// testLogger logs to t.Log so messages appear with -v
type testLogger struct{ t *testing.T }

func (l *testLogger) Info(msg string, args ...any) {
	l.t.Logf("INFO: "+msg, args...)
}

// noopLogger discards all output
func noopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
