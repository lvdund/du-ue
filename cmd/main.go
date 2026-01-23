package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"du_ue/internal/du"
	"du_ue/internal/uecontext"
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

	// Create context for UE management
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize UE Manager
	ueMgr := uecontext.NewUEManager(ctx, cfg, duSim)

	// Create all UEs based on configuration
	log.Info().Msgf("Creating %d UE(s) with base MSIN %s", cfg.UE.NUE, cfg.UE.MSIN)
	if err := ueMgr.CreateAllUEs(); err != nil {
		log.Fatal().Err(err).Msg("Failed to create UEs")
	}

	log.Info().Msgf("Successfully created %d UE(s)", ueMgr.GetUECount())
	
	// Log applied scenarios
	if len(cfg.UE.Scenarios) > 0 {
		log.Info().Msgf("Configured %d scenario(s):", len(cfg.UE.Scenarios))
		for _, scenario := range cfg.UE.Scenarios {
			log.Info().Msgf("  - %s: %s (apply_to: %s, events: %d)", 
				scenario.Name, 
				scenario.Description,
				scenario.ApplyTo,
				len(scenario.Events))
		}
	} else {
		log.Warn().Msg("No scenarios configured. UEs will be idle.")
	}

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	log.Info().Msg("DU-UE Simulator is running. Press Ctrl+C to stop.")

	// Block until interrupt signal is received
	<-sigChan

	log.Info().Msg("Shutting down DU-UE Simulator")

	// Cleanup
	ueMgr.Shutdown()
	duSim.Stop()
	cancel()

	log.Info().Msg("Shutdown complete")
	os.Exit(0)
}