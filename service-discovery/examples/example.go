package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/gofiber/fiber/v3"
	"github.com/routerarchitects/ow-common-mods/servicediscovery"
	"github.com/routerarchitects/ra-common-mods/kafka"
	ra_logger "github.com/routerarchitects/ra-common-mods/logger"
	logger_routes "github.com/routerarchitects/ra-common-mods/logger-routes"
)

type Config struct {
	Log       ra_logger.Config
	Kafka     kafka.Config
	Discovery servicediscovery.Config
}

func main() {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		panic(err)
	}
	logger, shutdown, err := ra_logger.Init(cfg.Log)
	if err != nil {
		panic(err)
	}
	defer shutdown()

	logger.Info("Logger Initiated")

	serviceDiscoveryLogger := ra_logger.Subsystem("service-discovery")
	serviceDiscoveryLogger.Info("Service Discovery Logger Initiated")
	serviceDiscoveryLogger.Debug("DEBUG Service Discovery Logger Initiated")

	// Create discovery from env
	discovery, err := servicediscovery.New(cfg.Discovery, cfg.Kafka, serviceDiscoveryLogger)
	if err != nil {
		log.Fatalf("failed to create discovery: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start discovery
	if err := discovery.Start(ctx); err != nil {
		log.Fatalf("failed to start discovery: %v", err)
	}
	logger.Info("service discovery started")

	// Periodically query discovered instances
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				instances := discovery.Store().GetServiceInstances("owanalytics")
				logger.Info(
					"discovered instances",
					"count", len(instances),
					"instances", instances,
				)
			}
		}
	}()

	app := fiber.New()
	logger_routes.RegisterFiberRoutes(app.Group("/logger"))
	app.Listen(":8080")

	// Wait for SIGTERM / SIGINT
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigCh
	logger.Info("received shutdown signal", "signal", sig.String())

	// Graceful stop (publishes leave)
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := discovery.Stop(shutdownCtx); err != nil {
		logger.Error("error during discovery shutdown", "error", err)
	}

	logger.Info("service stopped cleanly")
}
