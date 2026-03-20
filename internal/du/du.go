package du

import (
	"du_ue/internal/common/logger"
	"du_ue/internal/uecontext"
	"du_ue/pkg/config"
	"fmt"
	"strconv"
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

	ID             int64
	Name           string
	State          string
	Config         *config.DUConfig
	UEConfig       *config.UEConfig
	f1Client       F1Client
	ueMgr          *UeManager
	resourceMgr    *ResourceManager
	lastDuUeF1apId uint32
	mu             sync.Mutex
}

type UeChannel struct {
	UE                   *uecontext.UeContext
	ReceiveFromUeChannel chan []byte // almost rrc msg from ue is encoded to F1 msg then send to CU-CP
	SendToUeChannel      chan []byte
}

// NewDU creates a new DU simulator instance
func NewDU(duCfg *config.DUConfig, ueCfg *config.UEConfig) (*DU, error) {
	du := &DU{
		ID:       duCfg.ID,
		Name:     duCfg.Name,
		State:    DU_INACTIVE,
		Config:   duCfg,
		UEConfig: ueCfg,
		Logger: logger.InitLogger("info", map[string]string{
			"mod":   "du",
			"du_id": fmt.Sprintf("%d", duCfg.ID),
		}),
	}

	// Create F1AP client
	f1Client, err := NewF1APClient(duCfg.CUCPAddr, duCfg.CUCPPort, duCfg.LocalAddr, duCfg.LocalPort, du)
	if err != nil {
		return nil, fmt.Errorf("create F1AP client: %w", err)
	}
	du.f1Client = f1Client
	du.ueMgr = NewUeManager()
	du.resourceMgr = NewResourceManager()
	if du.Config.MockCU.TeidStart > 0 {
		du.resourceMgr.SetTeidCounter(du.Config.MockCU.TeidStart)
	}

	du.lastDuUeF1apId = 0

	return du, nil
}

// InitUE creates UE context and initializes channels
func (du *DU) InitUE(duUeF1apId, cuUeF1apId, cRnti int64) error {
	if du.UEConfig == nil {
		return fmt.Errorf("UE config not set")
	}

	// Create channels for UE communication
	toUE := make(chan []byte, 100)   // DU -> UE (RRC messages)
	fromUE := make(chan []byte, 100) // UE -> DU (RRC messages)

	// Set up UE channel structure
	ueChan := &UeChannel{
		ReceiveFromUeChannel: fromUE,
		SendToUeChannel:      toUE,
	}

	// Create and add UE context to manager
	ctx := &DuUeContext{
		DuUeF1apId:  duUeF1apId,
		CuUeF1apId:  cuUeF1apId,
		CRnti:       cRnti,
		UeChannel:   ueChan,
		PduSessions: make(map[int64]*GnbPDUSession),
		State:       UE_STATE_CONNECTED,
	}
	du.ueMgr.AddContext(ctx)

	// Start goroutine to handle RRC messages from this specific UE
	go du.HandleRrcFromUE(ctx)

	time.Sleep(100 * time.Millisecond)

	// Start UE simulator in a goroutine as it blocks waiting for RRC handshake
	go func() {
		ueCtx := uecontext.InitUE(toUE, fromUE, *du.UEConfig)
		if ueCtx == nil {
			du.Error("Failed to initialize UE simulator context (DU_UE_ID=%d)", duUeF1apId)
			return
		}
		// Update the store with the simulator context
		ueChan.UE = ueCtx
	}()

	du.Info("UE context initialized (DU_UE_ID=%d, CU_UE_ID=%d, C-RNTI=%d)",
		duUeF1apId, cuUeF1apId, cRnti)
	return nil
}

// HandleRrcFromUE handles RRC messages received from a specific UE channel
// (Implementation remains in du_rrc_handler.go, but signature changes)

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

	// DO NOT set du.State = DU_ACTIVE here!
	// We must wait for the F1 Setup Response from the CU.
	// OnF1SetupResponse() will set it to ACTIVE when the response is received.

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

	du.Info("DU is ready for UE connections")
}

// StartInitialAccess starts the Multi-UE Initial Access flow asynchronously
func (du *DU) StartInitialAccess(ueCfg *config.UEConfig) {
	nue := ueCfg.NUE
	baseMSIN, _ := strconv.ParseUint(ueCfg.MSIN, 10, 64)

	du.Info("Starting Initial Access for %d UEs", nue)

	var wg sync.WaitGroup
	for i := 0; i < nue; i++ {
		msin := fmt.Sprintf("%010d", baseMSIN+uint64(i))

		// Create a copy of UEConfig for this specific UE
		ueConf := *ueCfg
		ueConf.MSIN = msin

		wg.Add(1)
		go func(ueIndex int, conf config.UEConfig) {
			defer wg.Done()
			// 1. Allocate DU-UE F1AP ID and C-RNTI
			duUeF1apId := du.allocateDuUeF1apId()
			cRnti, _ := du.resourceMgr.AllocateCRNTI()

			du.Info("[UE %s] Starting Initial Access (DU_UE_ID: %d, C-RNTI: %d)", conf.MSIN, duUeF1apId, cRnti)

			// 2. Setup channels
			toUE := make(chan []byte, 100)   // DU -> UE
			fromUE := make(chan []byte, 100) // UE -> DU

			ueChan := &UeChannel{
				ReceiveFromUeChannel: fromUE,
				SendToUeChannel:      toUE,
			}

			// 3. Register UE Context in DU
			ctx := &DuUeContext{
				DuUeF1apId:  duUeF1apId,
				CuUeF1apId:  0,
				CRnti:       cRnti,
				UeChannel:   ueChan,
				PduSessions: make(map[int64]*GnbPDUSession),
				State:       UE_STATE_CONNECTED,
			}
			du.ueMgr.AddContext(ctx)

			// 4. Start DU RRC handler for this UE
			go du.HandleRrcFromUE(ctx)

			// 5. Initialize UE (RRCSetupRequest -> RRCSetup -> RRCSetupComplete)
			ueCtx := uecontext.InitUE(toUE, fromUE, conf)
			if ueCtx == nil {
				du.Error("[UE %s] Failed to initialize UE Context", conf.MSIN)
				return
			}
			ueChan.UE = ueCtx

			du.Info("[UE %s] RRC Connection Established successfully", conf.MSIN)

			// 6. Execute Scenarios
			du.executeScenarios(ueCtx, ueIndex, nue, ueCfg.Scenarios)

		}(i, ueConf)

		// Stagger UE launches to avoid overwhelming the AMF proxy
		time.Sleep(1 * time.Second)
	}

	// Wait for all UEs to finish their scenarios in a separate goroutine
	// so we don't block the caller of StartInitialAccess (main loop)
	go func() {
		wg.Wait()
		du.Info("=========================================================")
		du.Info("[SIMULATION] === ALL SCENARIOS COMPLETE FOR ALL UEs ===")
		du.Info("=========================================================")
	}()
}

func (du *DU) executeScenarios(ue *uecontext.UeContext, ueIndex int, totalUEs int, scenarios []config.UEScenario) {
	msin := ue.GetMsin()
	var ueWg sync.WaitGroup

	// Find DU-side UE context to get F1AP IDs
	var duUeCtx *DuUeContext
	for _, c := range du.ueMgr.GetAllContexts() {
		if c.UeChannel.UE == ue {
			duUeCtx = c
			break
		}
	}

	for _, scenario := range scenarios {
		if !scenario.ShouldApplyToUE(msin, ueIndex, totalUEs) {
			continue
		}

		du.Info("[UE %s] Applying scenario: %s", msin, scenario.Name)

		for _, event := range scenario.Events {
			delay, _ := event.ParseDelay()

			ueWg.Add(1)
			go func(ev config.EventEntry, d time.Duration) {
				defer ueWg.Done()
				time.Sleep(d)
				du.Info("[UE %s] Executing delayed event: %s", msin, ev.Type)

				switch ev.Type {
				case "registration":
					// RRCSetupComplete with RegistrationRequest was ALREADY sent in InitUE via TriggerInitRegistration.
					// Calling it again won't transmit it over RRC anyway, so we safely skip it.
					du.Info("[UE %s] Initial Registration was handled automatically during RRC bootstrap", msin)
				case "pdu_establishment":
					ue.TriggerPduSession()
				case "pdu_release":
					ue.TriggerReleaseAllPduSessions()
				case "du_pdu_release":
					// DRB ID 1 is the default for the first PDU session
					if duUeCtx != nil {
						du.TriggerDuInitiatedModification(duUeCtx.DuUeF1apId, 1)
					} else {
						du.Error("[UE %s] Cannot trigger DU-initiated modification: context not found", msin)
					}
				case "deregistration":
					ue.Terminate()
				case "handover":
					// Optional target_pci parse for extended testing
					ue.TriggerMeasurement()
				}
			}(event, delay)
		}
	}
	// Wait for all events for THIS UE to finish before returning
	ueWg.Wait()
}

func (du *DU) SetUEChannelForTest(ue *UeChannel) {
	du.mu.Lock()
	defer du.mu.Unlock()
	// Add/Update default UE context for testing
	ctx := du.ueMgr.GetContextByDuId(1)
	if ctx == nil {
		ctx = &DuUeContext{
			DuUeF1apId: 1,
			UeChannel:  ue,
		}
		du.ueMgr.AddContext(ctx)
	} else {
		ctx.UeChannel = ue
	}
}

func (du *DU) SetF1ClientForTest(client F1Client) {
	du.mu.Lock()
	defer du.mu.Unlock()
	du.f1Client = client
}

func (du *DU) GetUEChannelForTest() *UeChannel {
	// For testing, return a channel from the first UE context if available
	all := du.ueMgr.GetAllContexts()
	if len(all) > 0 {
		return all[0].UeChannel
	}
	return nil
}

// allocateDuUeF1apId allocates a new DU-side UE F1AP ID
func (du *DU) allocateDuUeF1apId() int64 {
	du.mu.Lock()
	defer du.mu.Unlock()
	du.lastDuUeF1apId++
	return int64(du.lastDuUeF1apId)
}

// GetUeManager returns the UE Manager (for testing)
func (du *DU) GetUeManager() *UeManager {
	return du.ueMgr
}
