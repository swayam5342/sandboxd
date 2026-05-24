package config

import (
	"net/http"
	"time"

	"github.com/swayam5342/sandboxd/internal/models"
	"github.com/swayam5342/sandboxd/internal/util"
)

func NewHttpConfig(h http.Handler) *models.HttpConfig {
	addr := util.EnvOr("PORT", ":8089")

	readTimeout := time.Duration(
		util.EnvIntOr("READ_TIMEOUT", 30),
	) * time.Second

	writeTimeout := time.Duration(
		util.EnvIntOr("WRITE_TIMEOUT", 120),
	) * time.Second

	idleTimeout := time.Duration(
		util.EnvIntOr("IDLE_TIMEOUT", 60),
	) * time.Second

	return &models.HttpConfig{
		Addr:         addr,
		Handler:      h,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
	}
}
