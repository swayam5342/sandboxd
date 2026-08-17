package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadNsjailConfig_MissingFile_ReturnsDefaults(t *testing.T) {
	cfg, err := LoadNsjailConfig(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err != nil {
		t.Fatalf("LoadNsjailConfig() error: %v", err)
	}
	if !reflect.DeepEqual(cfg, DefaultNsjailConfig()) {
		t.Errorf("want defaults when file is absent, got %+v", cfg)
	}
}

func TestLoadNsjailConfig_PartialOverride_KeepsOtherDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nsjail.yaml")
	if err := os.WriteFile(path, []byte("user: 1000\n"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	cfg, err := LoadNsjailConfig(path)
	if err != nil {
		t.Fatalf("LoadNsjailConfig() error: %v", err)
	}
	if cfg.User != 1000 {
		t.Errorf("want overridden User=1000, got %d", cfg.User)
	}
	defaults := DefaultNsjailConfig()
	if cfg.Group != defaults.Group {
		t.Errorf("want untouched default Group=%d, got %d", defaults.Group, cfg.Group)
	}
	if !reflect.DeepEqual(cfg.Rlimits, defaults.Rlimits) {
		t.Errorf("want untouched default Rlimits, got %+v", cfg.Rlimits)
	}
	if !reflect.DeepEqual(cfg.BindMountsRO, defaults.BindMountsRO) {
		t.Errorf("want untouched default BindMountsRO, got %v", cfg.BindMountsRO)
	}
}

func TestLoadNsjailConfig_ListFieldsReplaceWholesale(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nsjail.yaml")
	yaml := `
bind_mounts_ro:
  - /opt/custom-toolchain
extra_flags:
  - "--verbose"
`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	cfg, err := LoadNsjailConfig(path)
	if err != nil {
		t.Fatalf("LoadNsjailConfig() error: %v", err)
	}
	if !reflect.DeepEqual(cfg.BindMountsRO, []string{"/opt/custom-toolchain"}) {
		t.Errorf("want bind_mounts_ro fully replaced, got %v", cfg.BindMountsRO)
	}
	if !reflect.DeepEqual(cfg.ExtraFlags, []string{"--verbose"}) {
		t.Errorf("want extra_flags set, got %v", cfg.ExtraFlags)
	}
}

func TestLoadNsjailConfig_FullOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nsjail.yaml")
	yaml := `
user: 2000
group: 2000
rlimits:
  fsize_mb: 256
  nofile: 128
  stack_mb: 32
  as_floor_mb: 2048
  as_multiplier: 8
env:
  - "PATH=/custom/bin"
bind_mounts_ro:
  - /opt/toolchain
extra_flags:
  - "--time_limit_ns"
`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	cfg, err := LoadNsjailConfig(path)
	if err != nil {
		t.Fatalf("LoadNsjailConfig() error: %v", err)
	}
	want := NsjailConfig{
		User:  2000,
		Group: 2000,
		Rlimits: NsjailRlimits{
			FsizeMB:      256,
			Nofile:       128,
			StackMB:      32,
			ASFloorMB:    2048,
			ASMultiplier: 8,
		},
		Env:          []string{"PATH=/custom/bin"},
		BindMountsRO: []string{"/opt/toolchain"},
		ExtraFlags:   []string{"--time_limit_ns"},
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Errorf("want %+v, got %+v", want, cfg)
	}
}

func TestLoadNsjailConfig_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nsjail.yaml")
	if err := os.WriteFile(path, []byte("user: [this is not: valid"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if _, err := LoadNsjailConfig(path); err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoadNsjailConfig_NegativeUser_Rejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nsjail.yaml")
	if err := os.WriteFile(path, []byte("user: -1\n"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if _, err := LoadNsjailConfig(path); err == nil {
		t.Fatal("expected error for negative user")
	}
}

func TestLoadNsjailConfig_ZeroRlimit_Rejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nsjail.yaml")
	if err := os.WriteFile(path, []byte("rlimits:\n  fsize_mb: 0\n"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if _, err := LoadNsjailConfig(path); err == nil {
		t.Fatal("expected error for a zero rlimit value")
	}
}

func TestLoadNsjailConfig_ZeroASFloorOrMultiplier_Rejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nsjail.yaml")
	if err := os.WriteFile(path, []byte("rlimits:\n  as_multiplier: 0\n"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if _, err := LoadNsjailConfig(path); err == nil {
		t.Fatal("expected error for a zero as_multiplier")
	}
}

func TestDefaultNsjailConfig_MatchesPreviousHardcodedValues(t *testing.T) {
	// Pins the exact values that used to be hardcoded in NsjailArgs, so a
	// future edit here can't silently change zero-config (no nsjail.yaml)
	// behavior without a test failing.
	cfg := DefaultNsjailConfig()
	if cfg.User != 65534 || cfg.Group != 65534 {
		t.Errorf("want user/group 65534/65534, got %d/%d", cfg.User, cfg.Group)
	}
	if cfg.Rlimits != (NsjailRlimits{FsizeMB: 128, Nofile: 64, StackMB: 64, ASFloorMB: 1536, ASMultiplier: 4}) {
		t.Errorf("unexpected default rlimits: %+v", cfg.Rlimits)
	}
	if len(cfg.Env) != 1 || cfg.Env[0] != "PATH=/usr/bin:/bin:/usr/sbin:/sbin" {
		t.Errorf("unexpected default env: %v", cfg.Env)
	}
	if len(cfg.ExtraFlags) != 0 {
		t.Errorf("want no default extra_flags, got %v", cfg.ExtraFlags)
	}
}
