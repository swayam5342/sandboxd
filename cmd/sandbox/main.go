package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/swayam5342/sandboxd/internal/api"
	"github.com/swayam5342/sandboxd/internal/config"
)

func main() {
	fmt.Println("INIT")

	sc := &api.ServerConfig{}
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
