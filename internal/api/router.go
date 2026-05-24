package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type ServerConfig struct {
	Logger *slog.Logger
}

func NewRouter(sc *ServerConfig) http.Handler {
	r := chi.NewRouter()
	h := &Handler{
		logger: sc.Logger,
	}

	r.Get("/healthz", h.Healthz)
	return r
}
