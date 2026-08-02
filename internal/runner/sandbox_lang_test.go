package runner

// sandbox_lang_test.go — one build+run "happy path" test and one distinct
// custom-exit-code test per language configured in config/lang.yaml.
//
// Each language config here mirrors config/lang.yaml's cmd/args/limits for
// that language. Every test skips (not fails) if the language's toolchain
// isn't installed on the host running `go test`, so this file works the
// same whether run inside the Docker image or on a dev machine that only
// has a subset of toolchains.
//
// Run with: go test ./internal/runner/ -v -run TestLang

import (
	"os"
	"testing"

	"github.com/swayam5342/sandboxd/internal/config"
	"github.com/swayam5342/sandboxd/internal/models"
)

// requireBinary skips the test unless the given binary exists on disk.
func requireBinary(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Skipf("%s not found — skipping", path)
	}
}

// runLangJob builds and/or runs a language job end-to-end and returns the
// response, failing the test (not skipping) on unexpected internal errors.
func runLangJob(t *testing.T, r *Runner, lang *config.Language, source string, tests []models.TestInput) *models.RunResponse {
	t.Helper()
	job := Job{
		RequestID:        t.Name(),
		Language:         lang,
		Source:           source,
		SourceFilename:   lang.SourceFilename,
		ArtifactFilename: lang.ArtifactFilename,
		Tests:            tests,
		RunLimits:        lang.Run.Limits,
	}
	if lang.Build != nil {
		job.BuildLimits = lang.Build.Limits
	}
	resp, err := r.Run(t.Context(), job)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	return resp
}

// =============================================================================
// bash
// =============================================================================

func bashLang() *config.Language {
	return &config.Language{
		ID:                     "bash",
		Name:                   "Bash",
		SourceFilename:         "solution.sh",
		SourceFilenameStrategy: "fixed",
		Run: config.Phase{
			Cmd:  "/bin/bash",
			Args: []string{"{{source}}"},
			Limits: config.Limits{
				WallTimeS:    9,
				MemoryKB:     65536,
				MaxProcesses: 50,
			},
		},
	}
}

func TestLang_Bash_HappyPath(t *testing.T) {
	requireBinary(t, "/bin/bash")
	r := newTestRunner(t)
	resp := runLangJob(t, r, bashLang(), `echo "hi $((2+2))"`,
		[]models.TestInput{{ExpectedStdout: "hi 4\n"}})

	if resp.Status != models.StatusAccepted {
		t.Errorf("want accepted, got %q (tests=%+v)", resp.Status, resp.Tests)
	}
}

func TestLang_Bash_ExitCode(t *testing.T) {
	requireBinary(t, "/bin/bash")
	r := newTestRunner(t)
	resp := runLangJob(t, r, bashLang(), `exit 42`, []models.TestInput{{}})

	if resp.Tests[0].Status != models.TestRuntimeError {
		t.Errorf("want runtime_error, got %q", resp.Tests[0].Status)
	}
	if resp.Tests[0].ExitCode != 42 {
		t.Errorf("want exit code 42, got %d", resp.Tests[0].ExitCode)
	}
}

// =============================================================================
// node
// =============================================================================

func nodeLang() *config.Language {
	return &config.Language{
		ID:                     "node",
		Name:                   "JavaScript (Node)",
		SourceFilename:         "solution.js",
		SourceFilenameStrategy: "fixed",
		Run: config.Phase{
			Cmd:  "/usr/bin/node",
			Args: []string{"{{source}}"},
			Limits: config.Limits{
				WallTimeS:    9,
				MemoryKB:     262144,
				MaxProcesses: 100,
			},
		},
	}
}

func TestLang_Node_HappyPath(t *testing.T) {
	requireBinary(t, "/usr/bin/node")
	r := newTestRunner(t)
	resp := runLangJob(t, r, nodeLang(), `console.log(1 + 41)`,
		[]models.TestInput{{ExpectedStdout: "42\n"}})

	if resp.Status != models.StatusAccepted {
		t.Errorf("want accepted, got %q (tests=%+v)", resp.Status, resp.Tests)
	}
}

func TestLang_Node_ExitCode(t *testing.T) {
	requireBinary(t, "/usr/bin/node")
	r := newTestRunner(t)
	resp := runLangJob(t, r, nodeLang(), `process.exit(3)`, []models.TestInput{{}})

	if resp.Tests[0].Status != models.TestRuntimeError {
		t.Errorf("want runtime_error, got %q", resp.Tests[0].Status)
	}
	if resp.Tests[0].ExitCode != 3 {
		t.Errorf("want exit code 3, got %d", resp.Tests[0].ExitCode)
	}
}

// =============================================================================
// c
// =============================================================================

func cLang() *config.Language {
	return &config.Language{
		ID:                       "c",
		Name:                     "C",
		SourceFilename:           "solution.c",
		SourceFilenameStrategy:   "fixed",
		ArtifactFilename:         "solution",
		ArtifactFilenameStrategy: "fixed",
		Build: &config.Phase{
			Cmd:  "/usr/bin/gcc",
			Args: []string{"{{flags}}", "-o", "{{artifact}}", "{{source}}"},
			Limits: config.Limits{
				WallTimeS:    10,
				MemoryKB:     524288,
				MaxProcesses: 100,
			},
		},
		Run: config.Phase{
			Cmd:  "{{artifact}}",
			Args: []string{},
			Limits: config.Limits{
				WallTimeS:    5,
				MemoryKB:     262144,
				MaxProcesses: 64,
			},
		},
	}
}

func buildThenRun(t *testing.T, r *Runner, lang *config.Language, source string, tests []models.TestInput) *models.RunResponse {
	t.Helper()
	return runLangJob(t, r, lang, source, tests)
}

func TestLang_C_HappyPath(t *testing.T) {
	requireBinary(t, "/usr/bin/gcc")
	r := newTestRunner(t)
	resp := buildThenRun(t, r, cLang(),
		`#include <stdio.h>
int main() { printf("hello %d\n", 1+2); return 0; }`,
		[]models.TestInput{{ExpectedStdout: "hello 3\n"}})

	if resp.Build == nil || resp.Build.Status != models.BuildOK {
		t.Fatalf("want build ok, got %+v", resp.Build)
	}
	if resp.Status != models.StatusAccepted {
		t.Errorf("want accepted, got %q (tests=%+v)", resp.Status, resp.Tests)
	}
}

func TestLang_C_ExitCode(t *testing.T) {
	requireBinary(t, "/usr/bin/gcc")
	r := newTestRunner(t)
	resp := buildThenRun(t, r, cLang(),
		`int main() { return 5; }`,
		[]models.TestInput{{}})

	if resp.Build == nil || resp.Build.Status != models.BuildOK {
		t.Fatalf("want build ok, got %+v", resp.Build)
	}
	if resp.Tests[0].Status != models.TestRuntimeError {
		t.Errorf("want runtime_error, got %q", resp.Tests[0].Status)
	}
	if resp.Tests[0].ExitCode != 5 {
		t.Errorf("want exit code 5, got %d", resp.Tests[0].ExitCode)
	}
}

func TestLang_C_BuildFailure(t *testing.T) {
	requireBinary(t, "/usr/bin/gcc")
	r := newTestRunner(t)
	resp := buildThenRun(t, r, cLang(),
		`this is not valid C`,
		[]models.TestInput{{}})

	if resp.Status != models.StatusBuildFailed {
		t.Errorf("want build_failed, got %q", resp.Status)
	}
	if resp.Tests[0].Status != models.TestNotExecuted {
		t.Errorf("want not_executed for tests after a failed build, got %q", resp.Tests[0].Status)
	}
}

// =============================================================================
// cpp
// =============================================================================

func cppLang() *config.Language {
	return &config.Language{
		ID:                       "cpp",
		Name:                     "C++",
		SourceFilename:           "solution.cpp",
		SourceFilenameStrategy:   "fixed",
		ArtifactFilename:         "solution",
		ArtifactFilenameStrategy: "fixed",
		Build: &config.Phase{
			Cmd:  "/usr/bin/g++",
			Args: []string{"{{flags}}", "-o", "{{artifact}}", "{{source}}"},
			Limits: config.Limits{
				WallTimeS:    10,
				MemoryKB:     1048576,
				MaxProcesses: 100,
			},
		},
		Run: config.Phase{
			Cmd:  "{{artifact}}",
			Args: []string{},
			Limits: config.Limits{
				WallTimeS:    5,
				MemoryKB:     524288,
				MaxProcesses: 64,
			},
		},
	}
}

func TestLang_Cpp_HappyPath(t *testing.T) {
	requireBinary(t, "/usr/bin/g++")
	r := newTestRunner(t)
	resp := buildThenRun(t, r, cppLang(),
		`#include <iostream>
int main() { std::cout << "hi " << (1+1) << std::endl; return 0; }`,
		[]models.TestInput{{ExpectedStdout: "hi 2\n"}})

	if resp.Build == nil || resp.Build.Status != models.BuildOK {
		t.Fatalf("want build ok, got %+v", resp.Build)
	}
	if resp.Status != models.StatusAccepted {
		t.Errorf("want accepted, got %q (tests=%+v)", resp.Status, resp.Tests)
	}
}

func TestLang_Cpp_ExitCode(t *testing.T) {
	requireBinary(t, "/usr/bin/g++")
	r := newTestRunner(t)
	resp := buildThenRun(t, r, cppLang(),
		`int main() { return 6; }`,
		[]models.TestInput{{}})

	if resp.Tests[0].Status != models.TestRuntimeError {
		t.Errorf("want runtime_error, got %q", resp.Tests[0].Status)
	}
	if resp.Tests[0].ExitCode != 6 {
		t.Errorf("want exit code 6, got %d", resp.Tests[0].ExitCode)
	}
}

// =============================================================================
// java
// =============================================================================

func javaLang() *config.Language {
	return &config.Language{
		ID:                       "java",
		Name:                     "Java",
		SourceFilenameStrategy:   "from_request",
		ArtifactFilenameStrategy: "from_request",
		Build: &config.Phase{
			Cmd:  "/usr/bin/javac",
			Args: []string{"-J-Xmx384m", "-J-XX:-UseCompressedClassPointers", "{{flags}}", "{{source}}"},
			Limits: config.Limits{
				WallTimeS:    15,
				MemoryKB:     524288,
				MaxProcesses: 100,
			},
		},
		Run: config.Phase{
			Cmd:  "/usr/bin/java",
			Args: []string{"-Xmx256m", "-XX:-UseCompressedClassPointers", "-cp", "{{workdir}}", "{{artifact_name}}"},
			Limits: config.Limits{
				WallTimeS:    10,
				MemoryKB:     524288,
				MaxProcesses: 100,
			},
		},
	}
}

func javaJob(t *testing.T, source string, tests []models.TestInput) Job {
	t.Helper()
	lang := javaLang()
	return Job{
		RequestID:        t.Name(),
		Language:         lang,
		Source:           source,
		SourceFilename:   "Main.java",
		ArtifactFilename: "Main",
		Tests:            tests,
		BuildLimits:      lang.Build.Limits,
		RunLimits:        lang.Run.Limits,
	}
}

func TestLang_Java_HappyPath(t *testing.T) {
	requireBinary(t, "/usr/bin/javac")
	requireBinary(t, "/usr/bin/java")
	r := newTestRunner(t)

	source := `public class Main {
  public static void main(String[] args) {
    System.out.println("java ok");
  }
}`
	resp, err := r.Run(t.Context(), javaJob(t, source, []models.TestInput{{ExpectedStdout: "java ok\n"}}))
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if resp.Build == nil || resp.Build.Status != models.BuildOK {
		t.Fatalf("want build ok, got %+v", resp.Build)
	}
	if resp.Status != models.StatusAccepted {
		t.Errorf("want accepted, got %q (tests=%+v)", resp.Status, resp.Tests)
	}
}

func TestLang_Java_ExitCode(t *testing.T) {
	requireBinary(t, "/usr/bin/javac")
	requireBinary(t, "/usr/bin/java")
	r := newTestRunner(t)

	source := `public class Main {
  public static void main(String[] args) {
    System.exit(9);
  }
}`
	resp, err := r.Run(t.Context(), javaJob(t, source, []models.TestInput{{}}))
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if resp.Build == nil || resp.Build.Status != models.BuildOK {
		t.Fatalf("want build ok, got %+v", resp.Build)
	}
	if resp.Tests[0].Status != models.TestRuntimeError {
		t.Errorf("want runtime_error, got %q", resp.Tests[0].Status)
	}
	if resp.Tests[0].ExitCode != 9 {
		t.Errorf("want exit code 9, got %d", resp.Tests[0].ExitCode)
	}
}

// =============================================================================
// verilog
// =============================================================================

func verilogLang() *config.Language {
	return &config.Language{
		ID:                       "verilog",
		Name:                     "Verilog",
		SourceFilename:           "solution.v",
		SourceFilenameStrategy:   "fixed",
		ArtifactFilename:         "solution.vvp",
		ArtifactFilenameStrategy: "fixed",
		Build: &config.Phase{
			Cmd:  "/usr/bin/iverilog",
			Args: []string{"{{flags}}", "-o", "{{artifact}}", "{{source}}"},
			Limits: config.Limits{
				WallTimeS:    10,
				MemoryKB:     262144,
				MaxProcesses: 100,
			},
		},
		Run: config.Phase{
			Cmd:  "/usr/bin/vvp",
			Args: []string{"{{artifact}}"},
			Limits: config.Limits{
				WallTimeS:    5,
				MemoryKB:     131072,
				MaxProcesses: 64,
			},
		},
	}
}

func TestLang_Verilog_HappyPath(t *testing.T) {
	requireBinary(t, "/usr/bin/iverilog")
	requireBinary(t, "/usr/bin/vvp")
	r := newTestRunner(t)
	resp := buildThenRun(t, r, verilogLang(),
		`module main;
initial begin
  $display("verilog ok");
end
endmodule`,
		[]models.TestInput{{ExpectedStdout: "verilog ok\n"}})

	if resp.Build == nil || resp.Build.Status != models.BuildOK {
		t.Fatalf("want build ok, got %+v", resp.Build)
	}
	if resp.Status != models.StatusAccepted {
		t.Errorf("want accepted, got %q (tests=%+v)", resp.Status, resp.Tests)
	}
}

// Note: unlike the other languages, Icarus Verilog's vvp does not propagate
// $finish(N)'s N as the process exit code — it always exits 0 on a normal
// simulation finish (verified manually). There is no standard mechanism in
// Verilog/vvp for a testbench to report a custom process exit code, so
// there's no meaningful "distinct exit code" test to write for this
// language; runtime failures here surface as wrong/missing $display output
// instead.

// =============================================================================
// rust
// =============================================================================

func rustLang() *config.Language {
	return &config.Language{
		ID:                       "rust",
		Name:                     "Rust",
		SourceFilename:           "solution.rs",
		SourceFilenameStrategy:   "fixed",
		ArtifactFilename:         "solution",
		ArtifactFilenameStrategy: "fixed",
		Build: &config.Phase{
			Cmd:  "/usr/bin/rustc",
			Args: []string{"{{flags}}", "-o", "{{artifact}}", "{{source}}"},
			Limits: config.Limits{
				WallTimeS:    30,
				MemoryKB:     2097152,
				MaxProcesses: 100,
			},
		},
		Run: config.Phase{
			Cmd:  "{{artifact}}",
			Args: []string{},
			Limits: config.Limits{
				WallTimeS:    5,
				MemoryKB:     262144,
				MaxProcesses: 64,
			},
		},
	}
}

func TestLang_Rust_HappyPath(t *testing.T) {
	requireBinary(t, "/usr/bin/rustc")
	r := newTestRunner(t)
	resp := buildThenRun(t, r, rustLang(),
		`fn main() { println!("hi {}", 2+2); }`,
		[]models.TestInput{{ExpectedStdout: "hi 4\n"}})

	if resp.Build == nil || resp.Build.Status != models.BuildOK {
		t.Fatalf("want build ok, got %+v", resp.Build)
	}
	if resp.Status != models.StatusAccepted {
		t.Errorf("want accepted, got %q (tests=%+v)", resp.Status, resp.Tests)
	}
}

func TestLang_Rust_ExitCode(t *testing.T) {
	requireBinary(t, "/usr/bin/rustc")
	r := newTestRunner(t)
	resp := buildThenRun(t, r, rustLang(),
		`fn main() { std::process::exit(11); }`,
		[]models.TestInput{{}})

	if resp.Tests[0].Status != models.TestRuntimeError {
		t.Errorf("want runtime_error, got %q", resp.Tests[0].Status)
	}
	if resp.Tests[0].ExitCode != 11 {
		t.Errorf("want exit code 11, got %d", resp.Tests[0].ExitCode)
	}
}

func TestLang_Rust_PanicExitCode(t *testing.T) {
	requireBinary(t, "/usr/bin/rustc")
	r := newTestRunner(t)
	resp := buildThenRun(t, r, rustLang(),
		`fn main() { panic!("boom"); }`,
		[]models.TestInput{{}})

	if resp.Tests[0].Status != models.TestRuntimeError {
		t.Errorf("want runtime_error, got %q", resp.Tests[0].Status)
	}
	// Rust's panic runtime conventionally exits with code 101.
	if resp.Tests[0].ExitCode != 101 {
		t.Errorf("want exit code 101 (rust panic convention), got %d", resp.Tests[0].ExitCode)
	}
}

// =============================================================================
// py3 — one additional distinct-exit-code test to match the other languages
// (existing div-by-zero / name-error tests in sandbox_exec_test.go already
// cover py3's happy path and a couple of runtime-error shapes).
// =============================================================================

func TestLang_Py3_ExitCode(t *testing.T) {
	requireBinary(t, "/usr/bin/python3")
	r := newTestRunner(t)
	resp := runLangJob(t, r, py3Lang(), `import sys; sys.exit(7)`, []models.TestInput{{}})

	if resp.Tests[0].Status != models.TestRuntimeError {
		t.Errorf("want runtime_error, got %q", resp.Tests[0].Status)
	}
	if resp.Tests[0].ExitCode != 7 {
		t.Errorf("want exit code 7, got %d", resp.Tests[0].ExitCode)
	}
}
