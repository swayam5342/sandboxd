package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/swayam5342/sandboxd/internal/config"
	"github.com/swayam5342/sandboxd/internal/runner"
)

type ServerConfig struct {
	Config     *config.Config
	Runner     *runner.Runner
	Logger     *slog.Logger
	Version    string
	Commit     string
	NsjailPath string
}

func NewRouter(sc *ServerConfig) http.Handler {
	r := chi.NewRouter()
	r.Use(RecoverPanic(sc.Logger))
	r.Use(RequestID)
	r.Use(Logger(sc.Logger))
	r.Use(chimiddleware.CleanPath)
	r.Use(MaxBodySize(512 * 1024))
	h := &Handler{
		cfg:        sc.Config,
		runner:     sc.Runner,
		logger:     sc.Logger,
		version:    sc.Version,
		commit:     sc.Commit,
		nsjailPath: sc.NsjailPath,
	}

	r.Get("/healthz", h.Healthz)
	r.Get("/info", h.Info)
	r.Get("/readyz", h.Readyz)
	r.Post("/run", h.Run)
	return r
}
