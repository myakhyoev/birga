package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	envconfig "github.com/sethvargo/go-envconfig"

	"gitlab.com/loyihalar/birga/backend/internal/bootstrap"
	"gitlab.com/loyihalar/birga/backend/internal/config"
)

// @title Birga API
// @version v1
// @description Backend for the Birga caregiver app.
// @BasePath /
// @securityDefinitions.apikey AdminKey
// @in header
// @name X-Admin-Key
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Access token from /v1/auth/signup or /v1/auth/refresh, as "Bearer <token>".
func main() {
	var cfg config.Application
	if err := envconfig.Process(context.Background(), &cfg); err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	bootstrap.New(cfg).Run(ctx)
}
