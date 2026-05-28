package models

import (
	"io"
	"net/http"
	"time"
)

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type HealthResponse struct {
	Status string `json:"status"`
}

type ErrorResponse struct {
	Error APIError `json:"error"`
}

type HttpConfig struct {
	Addr         string
	Handler      http.Handler
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

type LoggerConfig struct {
	Level  string
	JSON   bool
	Output io.Writer
}

type RunRequest struct {
	Language         string      `json:"language"`
	Source           string      `json:"source"`
	SourceFilename   string      `json:"source_filename"`
	ArtifactFilename string      `json:"artifact_filename"`
	Build            *PhaseInput `json:"build"`
	Run              *PhaseInput `json:"run"`
	Tests            []TestInput `json:"tests"`
}

type PhaseInput struct {
	Limits *LimitOverride `json:"limits"`
	Flags  []string       `json:"flags"`
}

type LimitOverride struct {
	WallTimeS    *int `json:"wall_time_s"`
	MemoryKB     *int `json:"memory_kb"`
	MaxProcesses *int `json:"max_processes"`
}

type TestInput struct {
	Stdin          string `json:"stdin"`
	ExpectedStdout string `json:"expected_stdout"`
}

type RunResponse struct {
	Status string       `json:"status"`
	Build  *BuildResult `json:"build"`
	Tests  []TestResult `json:"tests"`
}

type BuildResult struct {
	Status     string `json:"status"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	DurationMs int64  `json:"duration_ms"`
}

type TestResult struct {
	Status       string `json:"status"`
	Stdout       string `json:"stdout"`
	Stderr       string `json:"stderr"`
	ExitCode     int    `json:"Exitcode"`
	DurationMs   int64  `json:"duration_ms"`
	MemoryPeakKB int64  `json:"memory_peak_kb"`
}

const (
	BuildOK            = "ok"
	BuildFailed        = "failed"
	BuildInternalError = "internal_error"
)

const (
	TestAccepted           = "accepted"
	TestWrongOutput        = "wrong_output"
	TestWhitespaceMismatch = "output_whitespace_mismatch"
	TestTimeExceeded       = "time_exceeded"
	TestMemoryExceeded     = "memory_exceeded"
	TestRuntimeError       = "runtime_error"
	TestNotExecuted        = "not_executed"
	TestInternalError      = "internal_error"
)

const (
	StatusAccepted           = "accepted"
	StatusBuildFailed        = "build_failed"
	StatusWrongOutput        = "wrong_output"
	StatusWhitespaceMismatch = "output_whitespace_mismatch"
	StatusTimeExceeded       = "time_exceeded"
	StatusMemoryExceeded     = "memory_exceeded"
	StatusRuntimeError       = "runtime_error"
	StatusInternalError      = "internal_error"
)

const (
	ErrUnknownLanguage = "unknown_language"
	ErrInvalidFilename = "invalid_filename"
	ErrSourceTooLarge  = "source_too_large"
	ErrTooManyTests    = "too_many_tests"
	ErrDisallowedFlag  = "disallowed_flag"
	ErrInvalidJSON     = "invalid_json"
	ErrMissingField    = "missing_field"
	ErrInternalError   = "internal_error"
)

type ReadyzResponse struct {
	Status    string                    `json:"status"`
	Nsjail    NsjailStatus              `json:"nsjail"`
	Languages map[string]LanguageStatus `json:"languages"`
}

type NsjailStatus struct {
	OK      bool   `json:"ok"`
	Version string `json:"version,omitempty"`
}

type LanguageStatus struct {
	OK      bool   `json:"ok"`
	Version string `json:"version,omitempty"`
	Error   string `json:"error,omitempty"`
}

type InfoResponse struct {
	BuildInfo BuildInfo      `json:"build_info"`
	Nsjail    NsjailInfo     `json:"nsjail"`
	Languages []LanguageInfo `json:"languages"`
	Limits    GlobalLimits   `json:"limits"`
	Stats     ServerStats    `json:"stats"`
}

type BuildInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	GoVersion string `json:"go_version"`
}

type NsjailInfo struct {
	Path    string `json:"path"`
	Version string `json:"version"`
}

type LanguageInfo struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Version          string    `json:"version"`
	DefaultRunLimits RunLimits `json:"default_run_limits"`
}

type RunLimits struct {
	WallTimeS    int `json:"wall_time_s"`
	MemoryKB     int `json:"memory_kb"`
	MaxProcesses int `json:"max_processes"`
}

type GlobalLimits struct {
	MaxSourceBytes    int `json:"max_source_bytes"`
	MaxTests          int `json:"max_tests"`
	MaxConcurrentJobs int `json:"max_concurrent_jobs"`
}

type ServerStats struct {
	InFlightJobs        int        `json:"in_flight_jobs"`
	JobsTotal           int64      `json:"jobs_total"`
	JobsFailedInternal  int64      `json:"jobs_failed_internal"`
	LastInternalErrorAt *time.Time `json:"last_internal_error_at,omitempty"`
	DiskFreeByteJailDir int64      `json:"disk_free_bytes_jail_dir"`
}
