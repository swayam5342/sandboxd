package config

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/swayam5342/sandboxd/internal/models"
	"github.com/swayam5342/sandboxd/internal/util"
	"go.yaml.in/yaml/v3"
)

type Config struct {
	Languages      []Language
	LanguagesByID  map[string]*Language
	KnownLanguages map[string]bool
	AllowedFlags   map[string][]string
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
		Languages:      raw.Languages,
		LanguagesByID:  make(map[string]*Language, len(raw.Languages)),
		KnownLanguages: make(map[string]bool, len(raw.Languages)),
		AllowedFlags:   make(map[string][]string, len(raw.Languages)),
	}
	for i := range cfg.Languages {
		lang := &cfg.Languages[i]
		//! To do validate the lang config
		cfg.LanguagesByID[lang.ID] = lang
		cfg.KnownLanguages[lang.ID] = true
		var combined []string
		if lang.Build != nil {
			combined = append(combined, lang.Build.FlagAllowlist...)
		}
		combined = append(combined, lang.Run.FlagAllowlist...)
		cfg.AllowedFlags[lang.ID] = combined
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
