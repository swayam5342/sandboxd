package config

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/swayam5342/sandboxd/internal/models"
)

func writeTempConfig(t *testing.T, yaml string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "lang.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

const validYAML = `
languages:
  - id: py3
    name: Python 3
    source_filename: solution.py
    source_filename_strategy: fixed
    check: --version
    run:
      cmd: /usr/bin/python3
      args: ["{{source}}"]
      limits:
        wall_time_s: 9
        memory_kb: 102400
        max_processes: 100

  - id: c
    name: C
    source_filename: solution.c
    source_filename_strategy: fixed
    artifact_filename: solution
    artifact_filename_strategy: fixed
    check: --version
    build:
      cmd: /usr/bin/gcc
      args: ["{{flags}}", "-o", "{{artifact}}", "{{source}}"]
      limits:
        wall_time_s: 10
        memory_kb: 524288
        max_processes: 100
      flag_allowlist:
        - "-O0"
        - "-std=*"
      flag_denylist:
        - "-include*"
    run:
      cmd: "{{artifact}}"
      args: []
      limits:
        wall_time_s: 5
        memory_kb: 262144
        max_processes: 64
      flag_allowlist:
        - "-run-only-flag"
`

func TestLoadFile_Valid(t *testing.T) {
	path := writeTempConfig(t, validYAML)
	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error: %v", err)
	}
	if len(cfg.Languages) != 2 {
		t.Fatalf("want 2 languages, got %d", len(cfg.Languages))
	}
	if !cfg.KnownLanguages["py3"] || !cfg.KnownLanguages["c"] {
		t.Error("expected py3 and c in KnownLanguages")
	}
	if cfg.LanguagesByID["py3"].Run.Cmd != "/usr/bin/python3" {
		t.Errorf("unexpected py3 run cmd: %q", cfg.LanguagesByID["py3"].Run.Cmd)
	}
}

func TestLoadFile_MissingFile(t *testing.T) {
	_, err := LoadFile(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	path := writeTempConfig(t, "languages: [this is not: valid: yaml")
	_, err := LoadFile(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestLoad_NoLanguages(t *testing.T) {
	path := writeTempConfig(t, "languages: []\n")
	_, err := LoadFile(path)
	if err == nil {
		t.Fatal("expected error for empty languages list, got nil")
	}
}

func TestLoad_MissingID(t *testing.T) {
	path := writeTempConfig(t, `
languages:
  - name: NoID
    run:
      cmd: /bin/true
`)
	_, err := LoadFile(path)
	if err == nil {
		t.Fatal("expected error for missing id, got nil")
	}
}

func TestLoad_MissingName(t *testing.T) {
	path := writeTempConfig(t, `
languages:
  - id: noname
    run:
      cmd: /bin/true
`)
	_, err := LoadFile(path)
	if err == nil {
		t.Fatal("expected error for missing name, got nil")
	}
}

func TestLoad_MissingRunCmd(t *testing.T) {
	path := writeTempConfig(t, `
languages:
  - id: norun
    name: No Run Cmd
    run:
      args: []
`)
	_, err := LoadFile(path)
	if err == nil {
		t.Fatal("expected error for missing run.cmd, got nil")
	}
}

func TestLoad_BuildPresentWithoutCmd(t *testing.T) {
	path := writeTempConfig(t, `
languages:
  - id: badbuild
    name: Bad Build
    run:
      cmd: /bin/true
    build:
      args: []
`)
	_, err := LoadFile(path)
	if err == nil {
		t.Fatal("expected error for build block missing cmd, got nil")
	}
}

func TestLoad_FixedStrategyRequiresSourceFilename(t *testing.T) {
	path := writeTempConfig(t, `
languages:
  - id: nofilename
    name: No Filename
    source_filename_strategy: fixed
    run:
      cmd: /bin/true
`)
	_, err := LoadFile(path)
	if err == nil {
		t.Fatal("expected error: fixed strategy requires source_filename")
	}
}

func TestLoad_InvalidFilenameStrategy(t *testing.T) {
	path := writeTempConfig(t, `
languages:
  - id: badstrategy
    name: Bad Strategy
    source_filename_strategy: whatever
    run:
      cmd: /bin/true
`)
	_, err := LoadFile(path)
	if err == nil {
		t.Fatal("expected error for invalid source_filename_strategy")
	}
}

func TestLoad_FromRequestStrategyDoesNotRequireFilename(t *testing.T) {
	path := writeTempConfig(t, `
languages:
  - id: fromreq
    name: From Request
    source_filename_strategy: from_request
    run:
      cmd: /bin/true
`)
	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error: %v", err)
	}
	if cfg.LanguagesByID["fromreq"].SourceFilename != "" {
		t.Error("expected empty SourceFilename for from_request strategy")
	}
}

func TestLoad_BuildAndRunFlagAllowlistsAreSeparate(t *testing.T) {
	path := writeTempConfig(t, validYAML)
	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error: %v", err)
	}
	buildFlags := cfg.AllowedBuildFlags["c"]
	runFlags := cfg.AllowedRunFlags["c"]

	if !slices.Contains(buildFlags, "-O0") {
		t.Error("expected -O0 in build flags")
	}
	if slices.Contains(buildFlags, "-run-only-flag") {
		t.Error("build allowlist must not include run-only flags")
	}
	if !slices.Contains(runFlags, "-run-only-flag") {
		t.Error("expected -run-only-flag in run flags")
	}
	if slices.Contains(runFlags, "-O0") {
		t.Error("run allowlist must not include build-only flags")
	}
}

func TestLoad_FlagDenylistLoaded(t *testing.T) {
	path := writeTempConfig(t, validYAML)
	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error: %v", err)
	}
	if !slices.Contains(cfg.DeniedBuildFlags["c"], "-include*") {
		t.Errorf("expected -include* in denied build flags, got %v", cfg.DeniedBuildFlags["c"])
	}
	if len(cfg.DeniedRunFlags["c"]) != 0 {
		t.Errorf("expected no denied run flags for c (none configured), got %v", cfg.DeniedRunFlags["c"])
	}
}

func TestLoad_NoBuildBlock_AllowedBuildFlagsEmpty(t *testing.T) {
	path := writeTempConfig(t, validYAML)
	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error: %v", err)
	}
	if len(cfg.AllowedBuildFlags["py3"]) != 0 {
		t.Errorf("expected no build flags for py3 (no build block), got %v", cfg.AllowedBuildFlags["py3"])
	}
}

// --- EffectiveSourceFilename / EffectiveArtifactFilename ---

func TestEffectiveSourceFilename_Fixed(t *testing.T) {
	lang := &Language{SourceFilenameStrategy: "fixed", SourceFilename: "solution.py"}
	if got := lang.EffectiveSourceFilename("ignored.py"); got != "solution.py" {
		t.Errorf("want solution.py, got %q", got)
	}
}

func TestEffectiveSourceFilename_FromRequest(t *testing.T) {
	lang := &Language{SourceFilenameStrategy: "from_request"}
	if got := lang.EffectiveSourceFilename("Main.java"); got != "Main.java" {
		t.Errorf("want Main.java, got %q", got)
	}
}

func TestEffectiveArtifactFilename_Fixed(t *testing.T) {
	lang := &Language{ArtifactFilenameStrategy: "fixed", ArtifactFilename: "solution"}
	if got := lang.EffectiveArtifactFilename("ignored"); got != "solution" {
		t.Errorf("want solution, got %q", got)
	}
}

func TestEffectiveArtifactFilename_FromRequest(t *testing.T) {
	lang := &Language{ArtifactFilenameStrategy: "from_request"}
	if got := lang.EffectiveArtifactFilename("Main"); got != "Main" {
		t.Errorf("want Main, got %q", got)
	}
}

// --- EffectiveBuildLimits / EffectiveRunLimits / mergeLimits / clampOverride ---

func TestEffectiveBuildLimits_NoBuildBlock(t *testing.T) {
	lang := &Language{}
	got := lang.EffectiveBuildLimits(nil)
	if got != (Limits{}) {
		t.Errorf("want zero Limits when Build is nil, got %+v", got)
	}
}

func TestEffectiveRunLimits_NilOverride_ReturnsDefaults(t *testing.T) {
	lang := &Language{Run: Phase{Limits: Limits{WallTimeS: 9, MemoryKB: 1024, MaxProcesses: 10}}}
	got := lang.EffectiveRunLimits(nil)
	want := Limits{WallTimeS: 9, MemoryKB: 1024, MaxProcesses: 10}
	if got != want {
		t.Errorf("want %+v, got %+v", want, got)
	}
}

func TestEffectiveRunLimits_OverrideWithinRange(t *testing.T) {
	lang := &Language{Run: Phase{Limits: Limits{WallTimeS: 10, MemoryKB: 1000, MaxProcesses: 50}}}
	five := 5
	got := lang.EffectiveRunLimits(&models.LimitOverride{WallTimeS: &five})
	if got.WallTimeS != 5 {
		t.Errorf("want WallTimeS=5 (tightened), got %d", got.WallTimeS)
	}
	if got.MemoryKB != 1000 || got.MaxProcesses != 50 {
		t.Errorf("unrelated fields should stay at defaults, got %+v", got)
	}
}

func TestEffectiveRunLimits_OverrideAboveDefault_ClampedDown(t *testing.T) {
	// A client must not be able to LOOSEN a limit past the language default.
	lang := &Language{Run: Phase{Limits: Limits{WallTimeS: 10}}}
	huge := 999999
	got := lang.EffectiveRunLimits(&models.LimitOverride{WallTimeS: &huge})
	if got.WallTimeS != 10 {
		t.Errorf("want override clamped to default (10), got %d", got.WallTimeS)
	}
}

func TestEffectiveRunLimits_OverrideZeroOrNegative_ClampedToOne(t *testing.T) {
	// A client must not be able to send 0/negative to effectively disable a limit.
	lang := &Language{Run: Phase{Limits: Limits{WallTimeS: 10, MemoryKB: 1000, MaxProcesses: 50}}}
	zero := 0
	neg := -5
	got := lang.EffectiveRunLimits(&models.LimitOverride{WallTimeS: &zero, MemoryKB: &neg})
	if got.WallTimeS != 1 {
		t.Errorf("want WallTimeS clamped to 1, got %d", got.WallTimeS)
	}
	if got.MemoryKB != 1 {
		t.Errorf("want MemoryKB clamped to 1, got %d", got.MemoryKB)
	}
}

func TestEffectiveRunLimits_PartialOverride_OnlyOneFieldChanges(t *testing.T) {
	lang := &Language{Run: Phase{Limits: Limits{WallTimeS: 10, MemoryKB: 1000, MaxProcesses: 50}}}
	three := 3
	got := lang.EffectiveRunLimits(&models.LimitOverride{MaxProcesses: &three})
	if got.WallTimeS != 10 || got.MemoryKB != 1000 {
		t.Errorf("unrelated fields should be untouched, got %+v", got)
	}
	if got.MaxProcesses != 3 {
		t.Errorf("want MaxProcesses=3, got %d", got.MaxProcesses)
	}
}

// --- ToLanguageInfo ---

func TestToLanguageInfo(t *testing.T) {
	lang := &Language{
		ID:   "py3",
		Name: "Python 3",
		Run:  Phase{Limits: Limits{WallTimeS: 9, MemoryKB: 1024, MaxProcesses: 10}},
	}
	info := lang.ToLanguageInfo("Python 3.11.2")
	if info.ID != "py3" || info.Name != "Python 3" || info.Version != "Python 3.11.2" {
		t.Errorf("unexpected LanguageInfo: %+v", info)
	}
	if info.DefaultRunLimits.WallTimeS != 9 {
		t.Errorf("want DefaultRunLimits.WallTimeS=9, got %d", info.DefaultRunLimits.WallTimeS)
	}
}

// --- NewHttpConfig ---

func TestNewHttpConfig_Defaults(t *testing.T) {
	for _, k := range []string{"PORT", "READ_TIMEOUT", "WRITE_TIMEOUT", "IDLE_TIMEOUT"} {
		t.Setenv(k, "")
	}
	cfg := NewHttpConfig(nil)
	if cfg.Addr != ":8089" {
		t.Errorf("want default addr :8089, got %q", cfg.Addr)
	}
	if cfg.ReadTimeout.Seconds() != 30 {
		t.Errorf("want default read timeout 30s, got %v", cfg.ReadTimeout)
	}
}

func TestNewHttpConfig_EnvOverride(t *testing.T) {
	t.Setenv("PORT", ":9999")
	t.Setenv("READ_TIMEOUT", "5")
	cfg := NewHttpConfig(nil)
	if cfg.Addr != ":9999" {
		t.Errorf("want :9999, got %q", cfg.Addr)
	}
	if cfg.ReadTimeout.Seconds() != 5 {
		t.Errorf("want 5s, got %v", cfg.ReadTimeout)
	}
}

// --- ProbeLanguage / ProbeNsjail ---

func TestProbeLanguage_Success(t *testing.T) {
	// /bin/echo exits 0 and prints its args — good enough to exercise the
	// success path without depending on any real toolchain being installed.
	lang := &Language{Run: Phase{Cmd: "/bin/echo"}, Check: "hello-world"}
	result := ProbeLanguage(lang)
	if !result.OK {
		t.Fatalf("expected OK probe, got err=%q", result.Err)
	}
	if result.Version != "hello-world" {
		t.Errorf("want version %q, got %q", "hello-world", result.Version)
	}
}

func TestProbeLanguage_UsesBuildCmdWhenPresent(t *testing.T) {
	lang := &Language{
		Run:   Phase{Cmd: "/bin/false"}, // would fail if this were used
		Build: &Phase{Cmd: "/bin/echo"},
		Check: "from-build",
	}
	result := ProbeLanguage(lang)
	if !result.OK || result.Version != "from-build" {
		t.Errorf("expected probe to use Build.Cmd, got OK=%v version=%q err=%q", result.OK, result.Version, result.Err)
	}
}

func TestProbeLanguage_Failure(t *testing.T) {
	lang := &Language{Run: Phase{Cmd: "/no/such/binary-xyz"}, Check: "--version"}
	result := ProbeLanguage(lang)
	if result.OK {
		t.Error("expected probe failure for nonexistent binary")
	}
	if result.Err == "" {
		t.Error("expected a non-empty error message")
	}
}

func TestProbeNsjail_Failure(t *testing.T) {
	ok, _, err := ProbeNsjail("/no/such/nsjail-xyz")
	if ok {
		t.Error("expected ProbeNsjail to fail for nonexistent binary")
	}
	if err == nil {
		t.Error("expected non-nil error")
	}
}
