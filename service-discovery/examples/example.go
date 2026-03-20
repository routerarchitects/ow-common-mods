package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/routerarchitects/ow-common-mods/servicediscovery"
	"github.com/routerarchitects/ra-common-mods/kafka"
	ra_logger "github.com/routerarchitects/ra-common-mods/logger"
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

	// Create discovery from env (via interface)
	discovery, err := servicediscovery.NewService(cfg.Discovery, cfg.Kafka, serviceDiscoveryLogger)
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

	// Periodically query a single discovered instance based on ordering strategy
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				instance := discovery.Store().GetServiceInstances("owanalytics")
				if instance == nil {
					logger.Info("no discovered instance", "service", "owanalytics")
					continue
				}
				logger.Info("selected instance", "service", "owanalytics", "instance", *instance)
			}
		}
	}()

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
