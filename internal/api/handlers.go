package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/swayam5342/sandboxd/internal/config"
	"github.com/swayam5342/sandboxd/internal/models"
	"github.com/swayam5342/sandboxd/internal/runner"
	"github.com/swayam5342/sandboxd/internal/validator"
)

type Handler struct {
	cfg                *config.Config
	runner             *runner.Runner
	logger             *slog.Logger
	nsjailPath         string
	version            string
	commit             string
	jobsTotal          atomic.Int64
	jobsFailedInternal atomic.Int64
	lastInternalErrAt  atomic.Pointer[time.Time]
}

func (h *Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, models.HealthResponse{Status: "ok"})
}

func (h *Handler) Info(w http.ResponseWriter, r *http.Request) {
	langs := make([]models.LanguageInfo, 0, len(h.cfg.Languages))
	for i := range h.cfg.Languages {
		lang := &h.cfg.Languages[i]
		probe := config.ProbeLanguage(lang)
		langs = append(langs, lang.ToLanguageInfo(probe.Version))
	}
	_, nsjailVersion, _ := config.ProbeNsjail(h.nsjailPath)

	var lastErrAt *time.Time
	if t := h.lastInternalErrAt.Load(); t != nil {
		lastErrAt = t
	}

	resp := models.InfoResponse{
		BuildInfo: models.BuildInfo{
			Version:   h.version,
			Commit:    h.commit,
			GoVersion: runtime.Version(),
		},
		Nsjail: models.NsjailInfo{
			Path:    h.nsjailPath,
			Version: nsjailVersion,
		},
		Languages: langs,
		Limits: models.GlobalLimits{
			MaxSourceBytes:    validator.MaxSourceBytes,
			MaxTests:          validator.MaxTests,
			MaxConcurrentJobs: h.runner.MaxConcurrent(),
		},
		Stats: models.ServerStats{
			InFlightJobs:        h.runner.InFlight(),
			JobsTotal:           h.jobsTotal.Load(),
			JobsFailedInternal:  h.jobsFailedInternal.Load(),
			LastInternalErrorAt: lastErrAt,
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) Readyz(w http.ResponseWriter, r *http.Request) {
	resp := models.ReadyzResponse{
		Status:    "ok",
		Languages: make(map[string]models.LanguageStatus, len(h.cfg.Languages)),
	}
	ok, version, err := config.ProbeNsjail(h.nsjailPath)
	resp.Nsjail = models.NsjailStatus{OK: ok, Version: version}
	if !ok {
		resp.Status = "degraded"
		h.logger.Error("nsjail probe failed", "error", err)
	}
	for i := range h.cfg.Languages {
		lang := &h.cfg.Languages[i]
		result := config.ProbeLanguage(lang)
		resp.Languages[lang.ID] = models.LanguageStatus{
			OK:      result.OK,
			Version: result.Version,
			Error:   result.Err,
		}
		if !result.OK {
			resp.Status = "degraded"
			h.logger.Error("language probe failed",
				"language", lang.ID,
				"error", result.Err,
			)
		}
	}
	status := http.StatusOK
	if resp.Status == "degraded" {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, resp)
}

func (h *Handler) Run(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFrom(r.Context())
	var req models.RunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if isBodyTooLarge(err) {
			writeError(w, http.StatusRequestEntityTooLarge, models.APIError{
				Code:    models.ErrSourceTooLarge,
				Message: "request body exceeds maximum allowed size",
			})
			return
		}
		writeError(w, http.StatusBadRequest, models.APIError{
			Code:    models.ErrInvalidJSON,
			Message: "request body is not valid JSON: " + err.Error(),
		})
		return
	}
	if apiErr := validator.ValidateRunRequest(&req, h.cfg.KnownLanguages, h.cfg.AllowedBuildFlags, h.cfg.AllowedRunFlags, h.cfg.DeniedBuildFlags, h.cfg.DeniedRunFlags); apiErr != nil {
		writeError(w, http.StatusBadRequest, *apiErr)
		return
	}
	lang := h.cfg.LanguagesByID[req.Language]
	sourceFilename := lang.EffectiveSourceFilename(req.SourceFilename)
	artifactFilename := lang.EffectiveArtifactFilename(req.ArtifactFilename)
	if lang.SourceFilenameStrategy == "from_request" && sourceFilename == "" {
		writeError(w, http.StatusBadRequest, models.APIError{
			Code:    models.ErrMissingField,
			Message: "source_filename is required for language: " + lang.ID,
		})
		return
	}
	if lang.ArtifactFilenameStrategy == "from_request" && artifactFilename == "" {
		writeError(w, http.StatusBadRequest, models.APIError{
			Code:    models.ErrMissingField,
			Message: "artifact_filename is required for language: " + lang.ID,
		})
		return
	}
	var buildOverride *models.LimitOverride
	var buildFlags []string
	if req.Build != nil {
		buildOverride = req.Build.Limits
		buildFlags = req.Build.Flags
	}
	var runOverride *models.LimitOverride
	var runFlags []string
	if req.Run != nil {
		runOverride = req.Run.Limits
		runFlags = req.Run.Flags
	}

	job := runner.Job{
		RequestID:        requestID,
		Language:         lang,
		Source:           req.Source,
		SourceFilename:   sourceFilename,
		ArtifactFilename: artifactFilename,
		Tests:            req.Tests,
		BuildLimits:      lang.EffectiveBuildLimits(buildOverride),
		RunLimits:        lang.EffectiveRunLimits(runOverride),
		BuildFlags:       buildFlags,
		RunFlags:         runFlags,
	}
	h.jobsTotal.Add(1)

	result, err := h.runner.Run(r.Context(), job)
	if err != nil {
		now := time.Now()
		h.lastInternalErrAt.Store(&now)
		h.jobsFailedInternal.Add(1)

		h.logger.Error("runner internal error",
			"request_id", requestID,
			"language", req.Language,
			"error", err,
		)
		writeError(w, http.StatusInternalServerError, models.APIError{
			Code:    models.ErrInternalError,
			Message: "internal server error — the problem is on our side",
		})
		return
	}
	writeJSON(w, http.StatusOK, result)
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

func isBodyTooLarge(err error) bool {
	if err == nil {
		return false
	}
	return err.Error() == "http: request body too large"
}
