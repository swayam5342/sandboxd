package config

import (
	"fmt"
	"os"

	"go.yaml.in/yaml/v3"
)

// NsjailConfig holds the operational nsjail settings that are safe to let
// operators tune without a Go code change: bind mounts, base rlimit values,
// injected environment, and the uid/gid the jailed process drops to.
//
// Deliberately NOT here: anything isolation-critical (the fresh /proc mount,
// the synthetic /etc, the root-vs-unprivileged CLONE_NEWUSER decision, cgroup
// detection). Those stay hardcoded in internal/runner/sandbox.go on purpose —
// a bad edit to this file must not be able to silently re-open a host
// information leak the way a bad edit to internal/runner's Go code (reviewed
// via PR) can't accidentally either.
type NsjailConfig struct {
	User  int `yaml:"user"`
	Group int `yaml:"group"`

	Rlimits NsjailRlimits `yaml:"rlimits"`

	// Env is injected into the jail via repeated --env flags.
	Env []string `yaml:"env"`

	// BindMountsRO is bind-mounted read-only into the jail. Entries may be a
	// literal path or a glob pattern (matched with filepath.Glob); any entry
	// that doesn't exist on this host is silently skipped rather than
	// failing nsjail startup, since which paths exist legitimately varies
	// by host/image (e.g. the OpenJDK version glob).
	BindMountsRO []string `yaml:"bind_mounts_ro"`

	// ExtraFlags is an escape hatch: appended verbatim to the nsjail argv,
	// before the "--" separator, completely unvalidated. It exists so a
	// newly-added nsjail flag can be used immediately, without waiting for
	// a first-class field here. It carries the same trust level as the rest
	// of this file: whoever can edit it already has significant blast
	// radius (same as lang.yaml's flag_allowlist or docker-compose.yml's
	// capabilities), so nothing here is denylisted or sanity-checked — it's
	// on the person editing this file not to reintroduce a closed hole
	// (e.g. don't put "--bindmount_ro" "/etc" in here).
	ExtraFlags []string `yaml:"extra_flags"`
}

// NsjailRlimits are the nsjail rlimit values not derived per-request from a
// language's configured memory_kb/max_processes (those stay computed in
// internal/runner).
type NsjailRlimits struct {
	FsizeMB int `yaml:"fsize_mb"` // --rlimit_fsize: max file write size, in MB
	Nofile  int `yaml:"nofile"`   // --rlimit_nofile: max open file descriptors
	StackMB int `yaml:"stack_mb"` // --rlimit_stack: max stack size, in MB

	// ASFloorMB/ASMultiplier tune the --rlimit_as (virtual address space)
	// heuristic: max(memoryKB/1024 * ASMultiplier, ASFloorMB). See
	// rlimitASMB in internal/runner/sandbox.go for why this needs to be
	// generous rather than tight.
	ASFloorMB    int `yaml:"as_floor_mb"`
	ASMultiplier int `yaml:"as_multiplier"`
}

// NsjailArgs before this file existed. It is a baseline for tests/dev
// convenience (and the seed the checked-in config/nsjail.yaml was written
// from) — production code never falls back to it silently. LoadNsjailConfig
// requires an actual config/nsjail.yaml to exist, the same way LoadFile
// requires lang.yaml to exist; there is no "config file absent, use
// defaults" path in the running server.
func DefaultNsjailConfig() NsjailConfig {
	return NsjailConfig{
		User:  65534,
		Group: 65534,
		Rlimits: NsjailRlimits{
			FsizeMB:      128,
			Nofile:       64,
			StackMB:      64,
			ASFloorMB:    1536,
			ASMultiplier: 4,
		},
		Env: []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin"},
		BindMountsRO: []string{
			"/usr",
			"/lib",
			"/lib64",
			"/bin",
			"/etc/alternatives",
			"/etc/java-*-openjdk",
			"/dev/urandom",
			"/dev/null",
		},
	}
}

// LoadNsjailConfig loads config/nsjail.yaml (or the given path). The file
// must exist — same as LoadFile for lang.yaml, this does not fall back to
// DefaultNsjailConfig on a missing file, so nsjail's actual runtime
// configuration always traces back to a file an operator can read, review,
// and edit, never an invisible in-code default.
//
// Fields are merged on top of DefaultNsjailConfig as a base, so a document
// that only sets bind_mounts_ro, for example, still gets the default
// rlimits/env/user/group for anything it didn't set. List fields (env,
// bind_mounts_ro, extra_flags) are replaced wholesale when present in the
// document, not appended to the defaults.
func LoadNsjailConfig(path string) (NsjailConfig, error) {
	cfg := DefaultNsjailConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		return NsjailConfig{}, fmt.Errorf("nsjail config: cannot read %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return NsjailConfig{}, fmt.Errorf("nsjail config: invalid YAML: %w", err)
	}
	if err := validateNsjailConfig(&cfg); err != nil {
		return NsjailConfig{}, fmt.Errorf("nsjail config: %w", err)
	}
	return cfg, nil
}

func validateNsjailConfig(cfg *NsjailConfig) error {
	if cfg.User < 0 || cfg.Group < 0 {
		return fmt.Errorf("user/group must not be negative")
	}
	if cfg.Rlimits.FsizeMB <= 0 || cfg.Rlimits.Nofile <= 0 || cfg.Rlimits.StackMB <= 0 {
		return fmt.Errorf("rlimits.fsize_mb, nofile, and stack_mb must all be positive")
	}
	if cfg.Rlimits.ASFloorMB <= 0 || cfg.Rlimits.ASMultiplier <= 0 {
		return fmt.Errorf("rlimits.as_floor_mb and as_multiplier must both be positive")
	}
	return nil
}
