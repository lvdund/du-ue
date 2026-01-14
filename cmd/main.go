package main

import (
	"du_ue/internal/du"
	"du_ue/pkg/config"
	"flag"
	"os"
	"os/signal"
	"syscall"

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

	log.Info().Msg("Starting DU-UE Simulator")

	// Create DU simulator
	duSim, err := du.NewDU(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create DU simulator")
	}

	// Start DU simulator
	if err := duSim.Start(); err != nil {
		log.Fatal().Err(err).Msg("Failed to start DU simulator")
		return
	}

	//NOTE: look file example.go
	// create ue: uecontext.InitUE()
	// list events:
	// 		ue.TriggerEvents(EventInfo{eventype: rrc_setup, delay: time.Second})
	// 		ue.TriggerEvents(EventInfo{eventype: registration, delay: time.Second})
	// 		ue.TriggerEvents(EventInfo{eventype: pdu_esta, delay: time.Second})

	// Wait for interrupt signal to keep the program running
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	log.Info().Msg("DU-UE Simulator is running. Press Ctrl+C to stop.")

	// Block until interrupt signal is received
	<-sigChan
	log.Info().Msg("Shutting down DU-UE Simulator")
	os.Exit(0)
}
