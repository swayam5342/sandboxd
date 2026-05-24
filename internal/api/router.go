package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

type ServerConfig struct {
}

func NewRouter(sc *ServerConfig) http.Handler {
	r := chi.NewRouter()
	h := &Handler{}
	r.Get("/healthz", h.Healthz)
	return r
}
