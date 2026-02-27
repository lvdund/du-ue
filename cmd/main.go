package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"du_ue/internal/du"
	"du_ue/pkg/config"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	configPath := flag.String("config", "config/config.yml", "Path to configuration file")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	// Initialize logger
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	// Set log level from config
	level, err := zerolog.ParseLevel(cfg.Logging.Level)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	log.Info().Msg("Starting DU-UE Simulator")

	// Create DU simulator
	duSim, err := du.NewDU(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create DU simulator")
	}

	// Start DU simulator
	if err := duSim.Start(); err != nil {
		log.Fatal().Err(err).Msg("Failed to start DU simulator")
	}

	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		for duSim.State != du.DU_ACTIVE {
			time.Sleep(100 * time.Millisecond)
		}

		log.Info().Msg("DU became ACTIVE. Starting Initial Access for configured UEs...")
		duSim.StartInitialAccess(cfg)
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	log.Info().Msg("DU-UE Simulator is running. Press Ctrl+C to stop.")

	// Block until interrupt signal is received
	<-sigChan

	log.Info().Msg("Shutting down DU-UE Simulator")

	duSim.Stop()
	cancel()

	log.Info().Msg("Shutdown complete")
	os.Exit(0)
}
