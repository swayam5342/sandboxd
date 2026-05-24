package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/swayam5342/sandboxd/internal/models"
)

type Handler struct {
	logger *slog.Logger
}

func (h *Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, models.HealthResponse{Status: "ok"})
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		_ = err
	}
}

func writeError(w http.ResponseWriter, status int, apiErr models.APIError) {
	writeJSON(w, status, models.ErrorResponse{Error: apiErr})
}
