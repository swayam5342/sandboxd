package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/swayam5342/sandboxd/internal/api"
	"github.com/swayam5342/sandboxd/internal/config"
	logger "github.com/swayam5342/sandboxd/internal/loger"
	"github.com/swayam5342/sandboxd/internal/models"
	"github.com/swayam5342/sandboxd/internal/runner"
	"github.com/swayam5342/sandboxd/internal/util"
)

var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	mkdirErr := os.MkdirAll("log", os.ModePerm)
	logFile, openErr := os.OpenFile("log/app.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	logWriter := io.Writer(os.Stdout)
	if openErr == nil {
		logWriter = io.MultiWriter(os.Stdout, logFile)
	}
	logger := logger.New(models.LoggerConfig{
		Level:  util.EnvOr("LOG_LEVEL", "info"),
		JSON:   util.EnvOr("LOG_JSON", "true") == "true",
		Output: logWriter,
	})
	if openErr == nil {
		defer logFile.Close()
	}
	if mkdirErr != nil {
		logger.Warn("failed to create log directory, file logging disabled", "error", mkdirErr)
	} else if openErr != nil {
		logger.Warn("failed to open log file, file logging disabled", "error", openErr)
	}
	err := godotenv.Load()
	if err != nil {
		logger.Error("Error loading .env file")
	}

	cfgPath := util.EnvOr("LANG_CONFIG", "config/lang.yaml")
	cfg, err := config.LoadFile(cfgPath)
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger.Info("config loaded", "languages", len(cfg.Languages))
	nsjailPath := util.EnvOr("NSJAIL_PATH", "/usr/sbin/nsjail")
	maxConcurrent := util.EnvIntOr("MAX_CONCURRENT", 100)
	apiKey := util.EnvOr("API_KEY", "")
	if apiKey == "" {
		logger.Warn("API_KEY is not set — /run is unauthenticated and open to anyone who can reach this port")
	}

	nsjailCfgPath := util.EnvOr("NSJAIL_CONFIG", "config/nsjail.yaml")
	nsjailCfg, err := config.LoadNsjailConfig(nsjailCfgPath)
	if err != nil {
		logger.Error("failed to load nsjail config", "path", nsjailCfgPath, "error", err)
		os.Exit(1)
	}
	logger.Info("nsjail config loaded", "path", nsjailCfgPath, "bind_mounts", len(nsjailCfg.BindMountsRO), "extra_flags", len(nsjailCfg.ExtraFlags))

	r := runner.New(runner.Options{
		NsjailPath:    nsjailPath,
		NsjailConfig:  &nsjailCfg,
		MaxConcurrent: maxConcurrent,
		Logger:        logger,
	})

	sc := &api.ServerConfig{
		Config:     cfg,
		Runner:     r,
		Logger:     logger,
		Version:    version,
		Commit:     commit,
		NsjailPath: nsjailPath,
		APIKey:     apiKey,
	}
	router := api.NewRouter(sc)

	httpConfig := config.NewHttpConfig(router)

	srv := &http.Server{
		Addr:         httpConfig.Addr,
		Handler:      httpConfig.Handler,
		ReadTimeout:  httpConfig.ReadTimeout,
		WriteTimeout: httpConfig.WriteTimeout,
		IdleTimeout:  httpConfig.IdleTimeout,
	}
	logger.Info("running startup probes...")
	ok, nsjailVer, probeErr := config.ProbeNsjail(nsjailPath)
	if !ok {
		logger.Error("nsjail not found at startup", "path", nsjailPath, "error", probeErr)
		os.Exit(1)
	}
	logger.Info("nsjail ok", "version", nsjailVer)

	allOK := true
	for i := range cfg.Languages {
		lang := &cfg.Languages[i]
		result := config.ProbeLanguage(lang)
		if result.OK {
			logger.Info("language ok", "id", lang.ID, "version", result.Version)
		} else {
			logger.Error("language probe failed", "id", lang.ID, "error", result.Err)
			allOK = false
		}
	}
	if !allOK {
		logger.Error("one or more language probes failed — fix the toolchain or remove the language from config")
		os.Exit(1)
	}

	runner.SweepOrphanDirs(logger)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(quit)

	go func() {
		if err := srv.ListenAndServe(); err != nil &&
			!errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed to start", "error", err)
			os.Exit(1)
		}
	}()

	<-quit

	ctx, cancel := context.WithTimeout(
		context.Background(),
		30*time.Second,
	)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}
}
