package du

import (
	"du_ue/internal/common/logger"
	"du_ue/internal/uecontext"
	"du_ue/pkg/config"
	"fmt"
	"sync"
	"time"
)

const (
	DU_INACTIVE = "DU_INACTIVE"
	DU_ACTIVE   = "DU_ACTIVE"
	DU_LOST     = "DU_LOST"
)

// DU represents the Distributed Unit simulator
type DU struct {
	*logger.Logger

	ID       int64
	Name     string
	State    string
	Config   *config.DUConfig
	UEConfig *config.UEConfig
	f1Client *F1APClient
	ue       *UeChannel
	mu       sync.Mutex
}

type UeChannel struct {
	UE                   *uecontext.UeContext
	ReceiveFromUeChannel chan []byte // almost rrc msg from ue is encoded to F1 msg then send to CU-CP
	SendToUeChannel      chan []byte
}

// NewDU creates a new DU simulator instance
func NewDU(cfg *config.Config) (*DU, error) {
	du := &DU{
		ID:       cfg.DU.ID,
		Name:     cfg.DU.Name,
		State:    DU_INACTIVE,
		Config:   &cfg.DU,
		UEConfig: &cfg.UE,
		Logger: logger.InitLogger("info", map[string]string{
			"mod":   "du",
			"du_id": fmt.Sprintf("%d", cfg.DU.ID),
		}),
	}

	// Create F1AP client
	f1Client, err := NewF1APClient(cfg.DU.CUCPAddr, cfg.DU.CUCPPort, cfg.DU.LocalAddr, cfg.DU.LocalPort, du)
	if err != nil {
		return nil, fmt.Errorf("create F1AP client: %w", err)
	}
	du.f1Client = f1Client

	return du, nil
}

// InitUE creates UE context and initializes channels
// This should be called after F1 Setup Procedure is complete
func (du *DU) InitUE() error {
	if du.UEConfig == nil {
		return fmt.Errorf("UE config not set")
	}

	// Create channels for UE communication
	toUE := make(chan []byte, 100)   // DU -> UE (RRC messages)
	fromUE := make(chan []byte, 100) // UE -> DU (RRC messages)
	// Set up UE channel structure
	du.ue = &UeChannel{
		ReceiveFromUeChannel: fromUE,
		SendToUeChannel:      toUE,
	}
	// Start goroutine to handle RRC messages from UE
	go du.handleRrcFromUE()

	time.Sleep(1 * time.Second)
	
	// Create UE context
	ueCtx := uecontext.InitUE(toUE, fromUE, *du.UEConfig)
	if ueCtx == nil {
		return fmt.Errorf("failed to initialize UE context")
	}
	du.ue.UE = ueCtx

	du.Info("UE context initialized after F1 Setup")
	return nil
}

// handleRrcFromUE handles RRC messages received from UE channel
func (du *DU) handleRrcFromUE() {
	du.Info("==== Started listening for RRC messages from UE ===")

	var isInitialMessage bool = true // First message is Initial UL RRC Message Transfer

	for {
		select {
		case rrcBytes, ok := <-du.ue.ReceiveFromUeChannel:
			if !ok {
				du.Warn("ReceiveFromUeChannel closed, stopping RRC handler")
				return
			}
			du.Info("Received RRC message from UE, length: %d, %v", len(rrcBytes), rrcBytes)
			if isInitialMessage {
				// First RRC message (RRCSetupRequest) -> Initial UL RRC Message Transfer
				if err := du.sendInitialULRRCMessageTransfer(rrcBytes); err != nil {
					du.Error("Failed to send Initial UL RRC Message Transfer: %v", err)
				}
				isInitialMessage = false
			} else {
				// Subsequent RRC messages -> UL RRC Message Transfer
				if err := du.sendULRRCMessageTransfer(rrcBytes); err != nil {
					du.Error("Failed to send UL RRC Message Transfer: %v", err)
				}
			}
		}
	}
}

// Start starts the DU simulator
func (du *DU) Start() error {
	du.mu.Lock()
	defer du.mu.Unlock()

	if du.State != DU_INACTIVE {
		return fmt.Errorf("DU is not in INACTIVE state")
	}

	// Connect to CU-CP
	if err := du.f1Client.Connect(); err != nil {
		return fmt.Errorf("connect to CU-CP: %w", err)
	}
	// Start message reading loop
	go du.f1Client.ReadLoop()

	// Send F1 Setup Request
	if err := du.SendF1SetupRequest(); err != nil {
		return fmt.Errorf("send F1 Setup Request: %s", err.Error())
	}
	

	du.State = DU_ACTIVE
	return nil
}

// Stop stops the DU simulator
func (du *DU) Stop() error {
	du.mu.Lock()
	defer du.mu.Unlock()

	if du.f1Client != nil {
		du.f1Client.Close()
	}

	du.State = DU_INACTIVE
	return nil
}

// SendF1SetupRequest sends F1 Setup Request to CU-CP
func (du *DU) SendF1SetupRequest() error {
	return du.f1Client.SendF1SetupRequest()
}

// OnF1SetupResponse handles F1 Setup Response from CU-CP
func (du *DU) OnF1SetupResponse() {
	du.mu.Lock()
	defer du.mu.Unlock()

	if du.State == DU_INACTIVE {
		du.State = DU_ACTIVE
		du.Info("F1 Setup completed successfully")
	}

	// Initialize UE context and channels after F1 Setup is complete
	if du.ue == nil {
		if err := du.InitUE(); err != nil {
			du.Error("Failed to initialize UE context after F1 Setup: %v", err)
			return
		}
		du.Info("UE context and channels initialized after F1 Setup")
	}
}
