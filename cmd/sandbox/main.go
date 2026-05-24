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
	"github.com/swayam5342/sandboxd/internal/util"
)

func main() {
	os.MkdirAll("log", os.ModePerm)
	file1, _ := os.OpenFile("log/app.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	multiWriter := io.MultiWriter(os.Stdout, file1)
	logger := logger.New(models.LoggerConfig{
		Level:  util.EnvOr("LOG_LEVEL", "info"),
		JSON:   util.EnvOr("LOG_JSON", "true") == "true",
		Output: multiWriter,
	})

	err := godotenv.Load()
	if err != nil {
		logger.Error("Error loading .env file")
	}

	sc := &api.ServerConfig{
		Logger: logger,
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

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(quit)

	go func() {
		if err := srv.ListenAndServe(); err != nil &&
			!errors.Is(err, http.ErrServerClosed) {
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
		os.Exit(1)
	}
}
