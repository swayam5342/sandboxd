package runner

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/swayam5342/sandboxd/internal/config"
	"github.com/swayam5342/sandboxd/internal/models"
	"github.com/swayam5342/sandboxd/internal/util"
	"github.com/swayam5342/sandboxd/internal/validator"
)

var (
	MaxOutputBytes   int           = util.EnvIntOr("MAX_OUTPUT_SIZE", 4*1024*1024)
	TruncationMarker string        = util.EnvOr("TRUNC_OUTPUT", "\n[output truncated]")
	JailBaseDir      string        = util.EnvOr("NSJAIL_BASE_DIR", "/tmp/sandboxd-jails")
	OrphanMaxAge     time.Duration = time.Duration(util.EnvIntOr("ORPHAN_PROC_TIME", 10)) * time.Minute
)
const (
	sandboxUID = 65534
	sandboxGID = 65534
)

type phaseArgs struct {
	sandboxDir       string
	cmd              string
	args             []string
	limits           config.Limits
	extraFlags       []string
	sourceFilename   string
	artifactFilename string
	stdin            string
	isBuild          bool
}

type phaseResult struct {
	Status       string
	Stdout       string
	Stderr       string
	ExitCode     int
	DurationMs   int64
	MemoryPeakKB int64
}

type limitedWriter struct {
	w       io.Writer
	limit   int
	written int
}

func (r *Runner) execute(ctx context.Context, job Job) (*models.RunResponse, error) {
	sandboxDir, err := createSandboxDir(job.RequestID)
	if err != nil {
		return nil, fmt.Errorf("sandbox: create dir: %w", err)
	}
	defer func() {
		if removeErr := os.RemoveAll(sandboxDir); removeErr != nil {
			r.logger.Error("sandbox: cleanup failed", "dir", sandboxDir, "error", removeErr)
		}
	}()
	r.logger.Info("sandbox created",
		"request_id", job.RequestID,
		"dir", sandboxDir,
		"language", job.Language.ID,
	)
	sourceFilename := job.SourceFilename
	if sourceFilename == "" {
		sourceFilename = job.Language.SourceFilename
	}
	sourcePath := filepath.Join(sandboxDir, sourceFilename)
	if !strings.HasPrefix(sourcePath, sandboxDir+string(filepath.Separator)) {
		return nil, fmt.Errorf("sandbox: path escape detected: %s", sourcePath)
	}
	if err := os.WriteFile(sourcePath, []byte(job.Source), 0644); err != nil {
		return nil, fmt.Errorf("sandbox: write source: %w", err)
	}
	//build
	var buildResult *models.BuildResult
	if job.Language.Build != nil {
		phaseRes, err := r.runPhase(ctx, phaseArgs{
			sandboxDir:       sandboxDir,
			cmd:              job.Language.Build.Cmd,
			args:             job.Language.Build.Args,
			limits:           job.BuildLimits,
			extraFlags:       job.BuildFlags,
			sourceFilename:   sourceFilename,
			artifactFilename: job.ArtifactFilename,
			stdin:            "",
			isBuild:          true,
		})
		if err != nil {
			return nil, fmt.Errorf("sandbox: build phase: %w", err)
		}
		buildResult = &models.BuildResult{
			Status:     phaseRes.Status,
			Stdout:     phaseRes.Stdout,
			Stderr:     phaseRes.Stderr,
			DurationMs: phaseRes.DurationMs,
		}
		if buildResult.Status != models.BuildOK {
			tests := make([]models.TestResult, len(job.Tests))
			for i := range tests {
				tests[i] = models.TestResult{Status: models.TestNotExecuted}
			}
			return &models.RunResponse{
				Status: models.StatusBuildFailed,
				Build:  buildResult,
				Tests:  tests,
			}, nil
		}
	}

	//run test
	testResults := make([]models.TestResult, len(job.Tests))
	for i, test := range job.Tests {
		result, err := r.runPhase(ctx, phaseArgs{
			sandboxDir:       sandboxDir,
			cmd:              job.Language.Run.Cmd,
			args:             job.Language.Run.Args,
			limits:           job.RunLimits,
			extraFlags:       job.RunFlags,
			sourceFilename:   sourceFilename,
			artifactFilename: job.ArtifactFilename,
			stdin:            test.Stdin,
			isBuild:          false,
		})
		if err != nil {
			return nil, fmt.Errorf("sandbox: run phase test[%d]: %w", i, err)
		}
		if result.Status == models.TestAccepted {
			result.Status = validator.CompareOutput(result.Stdout, test.ExpectedStdout)
		}
		testResults[i] = models.TestResult{
			Status:       result.Status,
			Stdout:       result.Stdout,
			Stderr:       result.Stderr,
			ExitCode:     result.ExitCode,
			DurationMs:   result.DurationMs,
			MemoryPeakKB: result.MemoryPeakKB,
		}
	}

	topStatus := validator.TopLevelStatus(models.BuildOK, testResults)
	return &models.RunResponse{
		Status: topStatus,
		Build:  buildResult,
		Tests:  testResults,
	}, nil
}

func (r *Runner) runPhase(ctx context.Context, args phaseArgs) (*phaseResult, error) {
	jailSourcePath := "/" + args.sourceFilename
	jailArtifactPath := "/" + args.artifactFilename
	jailWorkdir := "/"
	expandedArgs := expandArgs(
		args.args,
		jailSourcePath,
		jailArtifactPath,
		args.extraFlags,
		jailWorkdir,
	)

	jailCmd := args.cmd
	jailCmd = strings.ReplaceAll(jailCmd, "{{artifact}}", jailArtifactPath)
	jailCmd = strings.ReplaceAll(jailCmd, "{{source}}", jailSourcePath)

	nsjailArgs := NsjailArgs(r.nsjailPath, args.sandboxDir, args.limits, jailCmd, expandedArgs)

	//? for debug only and if posilbe switch to protobuff for config
	//r.logger.Info("nsjail command", "full_cmd", strings.Join(nsjailArgs, " "))

	cmd := exec.CommandContext(ctx, nsjailArgs[0], nsjailArgs[1:]...)
	if args.stdin != "" {
		cmd.Stdin = strings.NewReader(args.stdin)
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &limitedWriter{w: &stdoutBuf, limit: MaxOutputBytes}
	cmd.Stderr = &limitedWriter{w: &stderrBuf, limit: MaxOutputBytes}

	start := time.Now()
	runErr := cmd.Run()
	durationMs := time.Since(start).Milliseconds()

	var memoryPeakKB int64
	if cmd.ProcessState != nil {
		if rusage, ok := cmd.ProcessState.SysUsage().(*syscall.Rusage); ok {
			memoryPeakKB = int64(rusage.Maxrss) //nolint
		}
	}

	stdout := truncateIfNeeded(stdoutBuf.String())
	stderr := truncateIfNeeded(stderrBuf.String())
	status := mapExitStatus(runErr, args.isBuild)

	r.logger.Info("phase result",
		"is_build", args.isBuild,
		"status", status,
		"exit_err", runErr,
		"duration_ms", durationMs,
		"memory_peak_kb", memoryPeakKB,
		"stdout_preview", firstN(stdout, 100),
		"stderr_preview", firstN(stderr, 200),
	)
	exitCode := -1

	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}

	return &phaseResult{
		Status:       status,
		Stdout:       stdout,
		Stderr:       stderr,
		ExitCode:     exitCode,
		DurationMs:   durationMs,
		MemoryPeakKB: memoryPeakKB,
	}, nil
}

// todo
func mapExitStatus(err error, isBuild bool) string {
	if err == nil {
		if isBuild {
			return models.BuildOK
		}
		return models.TestAccepted
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		if isBuild {
			return models.BuildInternalError
		}
		return models.TestInternalError
	}
	switch exitErr.ExitCode() {
	case 2:
		if isBuild {
			return models.BuildFailed
		}
		return models.TestTimeExceeded
	case 3:
		if isBuild {
			return models.BuildFailed
		}
		return models.TestMemoryExceeded
	default:
		if isBuild {
			return models.BuildFailed
		}
		if ws, ok := exitErr.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			return models.TestRuntimeError
		}
		return models.TestRuntimeError
	}
}
func createSandboxDir(requestID string) (string, error) {
	if err := os.MkdirAll(JailBaseDir, 0700); err != nil {
		return "", fmt.Errorf("create jail base dir: %w", err)
	}
	dirPath := filepath.Join(
		JailBaseDir,
		requestID,
	)
	if err := os.Mkdir(dirPath, 0700); err != nil {
		return "", fmt.Errorf("create sandbox dir: %w", err)
	}
	// From here on, clean up the partially-built dir on any failure instead
	// of leaking it — the caller only gets to defer a cleanup once this
	// function returns successfully.
	succeeded := false
	defer func() {
		if !succeeded {
			_ = os.RemoveAll(dirPath)
		}
	}()
	// This directory becomes the jailed process's chroot root ("/"), and
	// compilers/interpreters need to read AND write directly in it (e.g.
	// gcc/rustc temp and object files next to the source). When we run as
	// root (the Docker image), NsjailArgs disables CLONE_NEWUSER and the
	// jailed process really runs as uid/gid 65534 (not remapped to uid 0
	// outside), so it needs to actually own this directory. When we're
	// NOT root (e.g. local/WSL dev as a regular user), nsjail instead uses
	// its normal unprivileged CLONE_NEWUSER path, which maps the caller's
	// own real uid into the jail — files we already own are accessible as
	// they are, and chowning to an arbitrary uid would fail anyway (EPERM).
	if os.Geteuid() == 0 {
		if err := os.Chown(dirPath, sandboxUID, sandboxGID); err != nil {
			return "", fmt.Errorf("chown sandbox dir: %w", err)
		}
	}
	tmpPath := filepath.Join(dirPath, "tmp")
	if err := os.Mkdir(tmpPath, 0777); err != nil {
		return "", fmt.Errorf("create tmp dir: %w", err)
	}
	// os.Mkdir's mode is masked by the process umask (typically 022), so
	// the 0777 above usually lands as 0755 (root-writable only) — chmod it
	// explicitly so the jailed uid 65534 process can actually write temp
	// files here (rustc, iverilog, etc. all need a writable TMPDIR).
	if err := os.Chmod(tmpPath, 0777); err != nil {
		return "", fmt.Errorf("chmod tmp dir: %w", err)
	}
	if err := writeMinimalEtc(dirPath); err != nil {
		return "", fmt.Errorf("create jail etc: %w", err)
	}
	succeeded = true
	return dirPath, nil
}

func writeMinimalEtc(sandboxDir string) error {
	etcPath := filepath.Join(sandboxDir, "etc")
	if err := os.Mkdir(etcPath, 0755); err != nil {
		return err
	}
	files := map[string]string{
		"passwd":        "nobody:x:65534:65534:nobody:/:/usr/sbin/nologin\n",
		"group":         "nobody:x:65534:\n",
		"nsswitch.conf": "passwd: files\ngroup: files\nhosts: files\n",
		"hosts":         "127.0.0.1 localhost\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(etcPath, name), []byte(content), 0644); err != nil {
			return err
		}
	}
	return nil
}

func expandArgs(templateArgs []string, sourcePath, artifactPath string, flags []string, workdir string) []string {
	var result []string
	for _, arg := range templateArgs {
		switch arg {
		case "{{flags}}":
			result = append(result, flags...)
		case "{{source}}":
			result = append(result, sourcePath)
		case "{{artifact}}":
			result = append(result, artifactPath)
		case "{{workdir}}":
			result = append(result, workdir)
		default:
			result = append(result, arg)
		}
	}
	return result
}

func (lw *limitedWriter) Write(p []byte) (int, error) {
	orig := len(p)
	if lw.written >= lw.limit {
		return orig, nil
	}
	remaining := lw.limit - lw.written
	if len(p) > remaining {
		p = p[:remaining]
	}
	n, err := lw.w.Write(p)
	lw.written += n
	if err != nil {
		return n, err
	}
	return orig, nil
}

func truncateIfNeeded(s string) string {
	if len(s) >= MaxOutputBytes {
		return s + TruncationMarker
	}
	return s
}

func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func NsjailArgs(nsjailPath, sandboxDir string, limits config.Limits, userCmd string, userArgs []string) []string {
	args := []string{
		nsjailPath,
		"--mode", "o",
		"--log", "/dev/null",
		"--chroot", sandboxDir,
		"--rw",
		"--cwd", "/",
		"--user", "65534",
		"--group", "65534",
		"--iface_no_lo",
		"--time_limit", fmt.Sprintf("%d", limits.WallTimeS),

		// rlimit-based limits work on WSL2 and Docker without cgroup v2
		"--rlimit_fsize", "128", // max file write: 128 MB
		"--rlimit_nofile", "64", // max open file descriptors
		"--rlimit_stack", "64", // max stack: 64 MB

		"--rlimit_as", fmt.Sprintf("%d", rlimitASMB(limits.MemoryKB)), // max address space, in MB
		"--rlimit_nproc", fmt.Sprintf("%d", clampMinInt(limits.MaxProcesses, 1)), // max processes/threads
		"--bindmount_ro", "/usr",
		"--bindmount_ro", "/lib",
		"--bindmount_ro", "/lib64",
		"--bindmount_ro", "/bin",
		// A source of entropy for anything doing crypto/SecureRandom init
		// (notably the JVM) — narrowly scoped, not the whole host /dev.
		"--bindmount_ro", "/dev/urandom",
		"--bindmount_ro", "/dev/null",
		"--mount", "none:/proc:proc:",

		// Inject safe environment path so compilers/runtimes can find toolchain binaries (e.g. ld)
		"--env", "PATH=/usr/bin:/bin:/usr/sbin:/sbin",
	}
	if os.Geteuid() == 0 {
		// Skip creating a nested user namespace: writing its uid_map/gid_map
		// and the securebits it needs (CAP_SETUID/SETGID/SETPCAP) require
		// broader effective privilege than we want to grant the container.
		// We still drop straight to uid/gid 65534 via setuid/setgid, just
		// without the extra user-namespace isolation layer; the mount, PID,
		// net, IPC and UTS namespaces plus the chroot remain fully active.
		// Only safe/needed when nsjail itself runs as euid 0 (the Docker
		// image); an unprivileged caller uses nsjail's normal unprivileged
		// CLONE_NEWUSER path instead, which needs no special capabilities.
		args = append(args, "--disable_clone_newuser")
	}
	// Debian's update-alternatives points toolchain entrypoints (cc, javac,
	// ...) through /etc/alternatives/*, and OpenJDK's own conf/ is symlinked
	// into /etc/java-17-openjdk; without these, those symlinks dangle inside
	// the jail. Just toolchain indirection/config, not host secrets — but
	// only bind-mount what's actually present on this host/image.
	for _, etcDir := range []string{"/etc/alternatives", "/etc/java-17-openjdk"} {
		if _, err := os.Stat(etcDir); err == nil {
			args = append(args, "--bindmount_ro", etcDir)
		}
	}
	if nsjailCgroupWorks() {
		args = append(args,
			"--cgroup_mem_max", fmt.Sprintf("%d", int64(limits.MemoryKB)*1024),
			"--cgroup_pids_max", fmt.Sprintf("%d", limits.MaxProcesses),
		)
	}

	args = append(args, "--", userCmd)
	return append(args, userArgs...)
}

// rlimitASMB converts a language's configured "real" memory budget into a
// virtual-address-space rlimit, in MB. RLIMIT_AS bounds virtual memory, not
// resident/physical memory, and runtimes like V8 (node) and the JVM reserve
// large virtual ranges up front regardless of actual usage (e.g. plain
// `node -e "1"` needs >512MB of address space just to start). So this is a
// coarse backstop against truly pathological allocation/fork-bomb-style
// virtual memory abuse when cgroups aren't available — not a precise cap;
// precise physical-memory enforcement is cgroup_mem_max's job when it works.
func rlimitASMB(memoryKB int) int {
	const floorMB = 1536
	mb := (memoryKB / 1024) * 4
	if mb < floorMB {
		return floorMB
	}
	return mb
}

func clampMinInt(value, min int) int {
	if value < min {
		return min
	}
	return value
}

func nsjailCgroupWorks() bool {
	version, err := os.ReadFile("/proc/version")
	if err == nil && strings.Contains(strings.ToLower(string(version)), "microsoft") {
		return false
	}
	controllers, err := os.ReadFile("/sys/fs/cgroup/cgroup.controllers")
	if err != nil {
		return false
	}
	content := string(controllers)
	return strings.Contains(content, "memory") && strings.Contains(content, "pids")
}

func SweepOrphanDirs(logger interface{ Info(string, ...any) }) {
	entries, err := os.ReadDir(JailBaseDir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-OrphanMaxAge)
	swept := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			if os.RemoveAll(filepath.Join(JailBaseDir, entry.Name())) == nil {
				swept++
			}
		}
	}
	if swept > 0 {
		logger.Info("swept orphan sandbox dirs", "count", swept)
	}
}
