package config

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/swayam5342/sandboxd/internal/models"
	"github.com/swayam5342/sandboxd/internal/util"
	"go.yaml.in/yaml/v3"
)

type Config struct {
	Languages        []Language
	LanguagesByID    map[string]*Language
	KnownLanguages   map[string]bool
	AllowedBuildFlags map[string][]string
	AllowedRunFlags   map[string][]string
}

type Language struct {
	ID                       string `yaml:"id"`
	Name                     string `yaml:"name"`
	SourceFilename           string `yaml:"source_filename"`
	SourceFilenameStrategy   string `yaml:"source_filename_strategy"`
	ArtifactFilename         string `yaml:"artifact_filename"`
	ArtifactFilenameStrategy string `yaml:"artifact_filename_strategy"`
	Build                    *Phase `yaml:"build"`
	Run                      Phase  `yaml:"run"`
	Check                    string `yaml:"check"`
}

type Phase struct {
	Cmd           string   `yaml:"cmd"`
	Args          []string `yaml:"args"`
	Limits        Limits   `yaml:"limits"`
	FlagAllowlist []string `yaml:"flag_allowlist"`
}

type Limits struct {
	WallTimeS    int `yaml:"wall_time_s"`
	MemoryKB     int `yaml:"memory_kb"`
	MaxProcesses int `yaml:"max_processes"`
}

func LoadFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: cannot read %s: %w", path, err)
	}
	return load(data)
}

func load(data []byte) (*Config, error) {
	var raw struct {
		Languages []Language `yaml:"languages"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("config: invalid YAML: %w", err)
	}
	if len(raw.Languages) == 0 {
		return nil, fmt.Errorf("config: no languages defined in YAML")
	}
	cfg := &Config{
		Languages:         raw.Languages,
		LanguagesByID:     make(map[string]*Language, len(raw.Languages)),
		KnownLanguages:    make(map[string]bool, len(raw.Languages)),
		AllowedBuildFlags: make(map[string][]string, len(raw.Languages)),
		AllowedRunFlags:   make(map[string][]string, len(raw.Languages)),
	}
	for i := range cfg.Languages {
		lang := &cfg.Languages[i]
		if err := validateLanguageConfig(lang); err != nil {
			return nil, fmt.Errorf("config: language[%d] (%s): %w", i, lang.ID, err)
		}
		cfg.LanguagesByID[lang.ID] = lang
		cfg.KnownLanguages[lang.ID] = true
		if lang.Build != nil {
			cfg.AllowedBuildFlags[lang.ID] = lang.Build.FlagAllowlist
		}
		cfg.AllowedRunFlags[lang.ID] = lang.Run.FlagAllowlist
	}
	return cfg, nil
}

func NewHttpConfig(h http.Handler) *models.HttpConfig {
	addr := util.EnvOr("PORT", ":8089")

	readTimeout := time.Duration(
		util.EnvIntOr("READ_TIMEOUT", 30),
	) * time.Second

	writeTimeout := time.Duration(
		util.EnvIntOr("WRITE_TIMEOUT", 120),
	) * time.Second

	idleTimeout := time.Duration(
		util.EnvIntOr("IDLE_TIMEOUT", 60),
	) * time.Second

	return &models.HttpConfig{
		Addr:         addr,
		Handler:      h,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
	}
}

func validateLanguageConfig(lang *Language) error {
	if lang.ID == "" {
		return fmt.Errorf("id is required")
	}
	if lang.Name == "" {
		return fmt.Errorf("name is required")
	}
	if err := validateFilenameStrategy(lang.SourceFilenameStrategy, "source_filename_strategy"); err != nil {
		return err
	}
	if err := validateFilenameStrategy(lang.ArtifactFilenameStrategy, "artifact_filename_strategy"); err != nil {
		return err
	}

	if lang.SourceFilenameStrategy == "fixed" && lang.SourceFilename == "" {
		return fmt.Errorf("source_filename is required when source_filename_strategy is 'fixed'")
	}
	if lang.Run.Cmd == "" {
		return fmt.Errorf("run.cmd is required")
	}
	if lang.Build != nil && lang.Build.Cmd == "" {
		return fmt.Errorf("build.cmd is required when build block is present")
	}
	return nil
}

func validateFilenameStrategy(strategy, fieldName string) error {
	switch strategy {
	case "", "fixed", "from_request":
		return nil
	default:
		return fmt.Errorf("%s must be 'fixed' or 'from_request', got %q", fieldName, strategy)
	}
}

func (l *Language) EffectiveSourceFilename(fromRequest string) string {
	if l.SourceFilenameStrategy == "from_request" {
		return fromRequest
	}
	return l.SourceFilename
}

func (l *Language) EffectiveArtifactFilename(fromRequest string) string {
	if l.ArtifactFilenameStrategy == "from_request" {
		return fromRequest
	}
	return l.ArtifactFilename
}

func (l *Language) EffectiveBuildLimits(override *models.LimitOverride) Limits {
	if l.Build == nil {
		return Limits{}
	}
	return mergeLimits(l.Build.Limits, override)
}

func (l *Language) EffectiveRunLimits(override *models.LimitOverride) Limits {
	return mergeLimits(l.Run.Limits, override)
}

func clampOverride(value, max int) int {
	if value < 1 {
		return 1
	}
	if value > max {
		return max
	}
	return value
}

func mergeLimits(defaults Limits, override *models.LimitOverride) Limits {
	result := defaults
	if override == nil {
		return result
	}
	if override.WallTimeS != nil {
		result.WallTimeS = clampOverride(*override.WallTimeS, defaults.WallTimeS)
	}
	if override.MemoryKB != nil {
		result.MemoryKB = clampOverride(*override.MemoryKB, defaults.MemoryKB)
	}
	if override.MaxProcesses != nil {
		result.MaxProcesses = clampOverride(*override.MaxProcesses, defaults.MaxProcesses)
	}
	return result
}

type ProbeResult struct {
	OK      bool
	Version string
	Err     string
}

// probeTimeout bounds every toolchain probe subprocess. /info and /readyz
// are unauthenticated and run one of these per configured language on every
// request, so a hung binary must not be able to hang the handler.
const probeTimeout = 5 * time.Second

func ProbeLanguage(lang *Language) ProbeResult {
	binary := lang.Run.Cmd
	if lang.Build != nil {
		binary = lang.Build.Cmd
	}
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, binary, lang.Check).CombinedOutput()
	if err != nil {
		return ProbeResult{
			OK:  false,
			Err: fmt.Sprintf("%s %s failed: %v", binary, lang.Check, err),
		}
	}
	version := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]
	return ProbeResult{OK: true, Version: version}
}

func ProbeNsjail(path string) (ok bool, version string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	out, execErr := exec.CommandContext(ctx, path, "--help").CombinedOutput()
	if execErr != nil {
		return false, "", fmt.Errorf("%s --help failed: %w", path, execErr)
	}
	v := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]
	return true, v, nil
}

func (l *Language) ToLanguageInfo(version string) models.LanguageInfo {
	return models.LanguageInfo{
		ID:      l.ID,
		Name:    l.Name,
		Version: version,
		DefaultRunLimits: models.RunLimits{
			WallTimeS:    l.Run.Limits.WallTimeS,
			MemoryKB:     l.Run.Limits.MemoryKB,
			MaxProcesses: l.Run.Limits.MaxProcesses,
		},
	}
}
