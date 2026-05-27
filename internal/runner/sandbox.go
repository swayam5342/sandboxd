package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/swayam5342/sandboxd/internal/models"
	"github.com/swayam5342/sandboxd/internal/util"
)

var (
	MaxOutputBytes   int           = util.EnvIntOr("MAX_OUTPUT_SIZE", 4*1024*1024)
	TruncationMarker string        = util.EnvOr("TRUNC_OUTPUT", "\n[output truncated]")
	JailBaseDir      string        = util.EnvOr("NSJAIL_BASE_DIR", "/tmp/sandboxd-jails")
	OrphanMaxAge     time.Duration = time.Duration(util.EnvIntOr("ORPHAN_PROC_TIME", 10)) * time.Minute
)
var jobCounter atomic.Uint64

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

}

func createSandboxDir(requestID string) (string, error) {
	if err := os.MkdirAll(JailBaseDir, 0700); err != nil {
		return "", fmt.Errorf("create jail base dir: %w", err)
	}
	dirPath := filepath.Join(
		JailBaseDir,
		fmt.Sprintf("%s-%d", requestID, time.Now().UnixNano()),
	)
	if err := os.Mkdir(dirPath, 0700); err != nil {
		return "", fmt.Errorf("create sandbox dir: %w", err)
	}
	tmpPath := filepath.Join(dirPath, "tmp")
	if err := os.Mkdir(tmpPath, 0777); err != nil {
		return "", fmt.Errorf("create tmp dir: %w", err)
	}
	return dirPath, nil
}
