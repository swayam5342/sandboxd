package runner

import (
	"context"
	"log/slog"
	"runtime"
	"sync/atomic"

	"github.com/swayam5342/sandboxd/internal/config"
	"github.com/swayam5342/sandboxd/internal/models"
)

type Job struct {
	RequestID        string
	Language         *config.Language
	Source           string
	SourceFilename   string
	ArtifactFilename string
	Tests            []models.TestInput
	BuildLimits      config.Limits
	RunLimits        config.Limits
	BuildFlags       []string
	RunFlags         []string
}

type Options struct {
	NsjailPath    string
	NsjailConfig  *config.NsjailConfig // nil uses config.DefaultNsjailConfig()
	MaxConcurrent int
	Logger        *slog.Logger
}

type Runner struct {
	nsjailPath    string
	nsjailConfig  config.NsjailConfig
	sem           chan struct{}
	maxConcurrent int
	inFlight      atomic.Int32
	logger        *slog.Logger
}

func New(opts Options) *Runner {
	max := opts.MaxConcurrent
	if max <= 0 {
		max = runtime.NumCPU()
	}
	nsjailCfg := config.DefaultNsjailConfig()
	if opts.NsjailConfig != nil {
		nsjailCfg = *opts.NsjailConfig
	}
	return &Runner{
		nsjailPath:    opts.NsjailPath,
		nsjailConfig:  nsjailCfg,
		sem:           make(chan struct{}, max),
		maxConcurrent: max,
		logger:        opts.Logger,
	}
}

func (r *Runner) MaxConcurrent() int {
	return r.maxConcurrent
}

func (r *Runner) InFlight() int {
	return int(r.inFlight.Load())
}

func (r *Runner) Run(ctx context.Context, job Job) (*models.RunResponse, error) {
	select {
	case r.sem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() {
		<-r.sem
		r.inFlight.Add(-1)
	}()
	r.inFlight.Add(1)
	return r.execute(ctx, job)
}
