package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	du_logger "du_ue/internal/common/logger"
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
	if cfg.Logging.TimeFormat != "" {
		du_logger.SetTimeFormat(cfg.Logging.TimeFormat)
	}
	zerolog.TimeFieldFormat = "2006-01-02 15:04:05"
	log.Logger = log.Output(du_logger.NewConsoleWriter())

	// Set log level from config
	level, err := zerolog.ParseLevel(cfg.Logging.Level)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	log.Info().Msg("Starting DU-UE Simulator")

	// Create DU simulators
	dus := make(map[string]*du.DU)
	for i, duCfg := range cfg.DUs {
		duSim, err := du.NewDU(&cfg.DUs[i], &cfg.UE)
		if err != nil {
			log.Fatal().Err(err).Msgf("Failed to create DU simulator %s", duCfg.Name)
		}
		
		dus[duCfg.Name] = duSim

		// Start DU simulator
		if err := duSim.Start(); err != nil {
			log.Fatal().Err(err).Msgf("Failed to start DU simulator %s", duCfg.Name)
		}

		// Stagger DU starts if interval is configured
		if i < len(cfg.DUs)-1 && duCfg.Interval != "" {
			if interval, err := time.ParseDuration(duCfg.Interval); err == nil && interval > 0 {
				log.Info().Msgf("Waiting %v before starting next DU...", interval)
				time.Sleep(interval)
			}
		}
	}

	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		// Wait for ALL DUs to become Active
		for {
			allActive := true
			for _, duSim := range dus {
				if duSim.State != du.DU_ACTIVE {
					allActive = false
					break
				}
			}
			if allActive {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}

		log.Info().Msg("All DUs became ACTIVE. Starting Initial Access for configured UEs...")
		
		// Find the initial DU to attach UEs to
		initialDuName := cfg.UE.DUID
		initialDu, exists := dus[initialDuName]
		if !exists {
			log.Warn().Msgf("Initial DU '%s' not found. Falling back to first available DU.", initialDuName)
			for _, d := range dus {
				initialDu = d
				break
			}
		}

		if initialDu != nil {
			initialDu.StartInitialAccess(&cfg.UE)
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	log.Info().Msg("DU-UE Simulator is running. Press Ctrl+C to stop.")

	// Block until interrupt signal is received
	<-sigChan

	log.Info().Msg("Shutting down DU-UE Simulators")

	for _, duSim := range dus {
		duSim.Stop()
	}
	cancel()

	log.Info().Msg("Shutdown complete")
	os.Exit(0)
}
