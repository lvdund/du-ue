package uecontext

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"du_ue/internal/common/logger"
	"du_ue/pkg/config"
)

type UEManager struct {
	*logger.Logger
	ctx       context.Context
	cancel    context.CancelFunc
	ues       map[string]*UeContext // key: MSIN
	mutex     sync.RWMutex
	duSim     interface{}
	config    *config.Config
	scenarios []config.UEScenario
}

func NewUEManager(ctx context.Context, cfg *config.Config, duSim interface{}) *UEManager {
	ctx, cancel := context.WithCancel(ctx)

	return &UEManager{
		ctx:       ctx,
		cancel:    cancel,
		ues:       make(map[string]*UeContext),
		Logger:    logger.InitLogger("", map[string]string{"mod": "ue_manager"}),
		duSim:     duSim,
		config:    cfg,
		scenarios: cfg.UE.Scenarios,
	}
}

// DUChannels holds the channel pair the DU simulator needs to talk to a UE.
type DUChannels struct {
	ToUE   chan []byte // DU writes inbound RRC here
	FromUE chan []byte // DU reads outbound RRC from here
	Ready  chan bool   // DU signals readiness here
}

// CreateUE creates a UeContext, connects it to conf.DUID, and returns the
// DUChannels so the DU side can wire up immediately.
func (mgr *UEManager) CreateUE(conf config.UEConfig) (*UeContext, *DUChannels, error) {
	mgr.mutex.Lock()
	defer mgr.mutex.Unlock()

	if _, exists := mgr.ues[conf.MSIN]; exists {
		return nil, nil, fmt.Errorf("UE with MSIN %s already exists", conf.MSIN)
	}

	// pciToDUID removed — UE no longer resolves DU ID from PCI
	ue := CreateUe(conf, mgr.ctx)
	if ue == nil {
		return nil, nil, fmt.Errorf("failed to create UE context")
	}

	duID := conf.DUID
	if duID == "" {
		duID = "du-0"
	}

	conn, err := ue.ConnectToDU(duID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect UE to DU %s: %w", duID, err)
	}

	go ue.processEvents()

	mgr.ues[conf.MSIN] = ue
	mgr.Info("Created and registered UE: MSIN=%s, SUPI=%s, DU=%s", conf.MSIN, ue.supi, duID)

	return ue, connToChannels(conn), nil
}

// CreateAllUEs creates N UEs with auto-incremented MSINs.
func (mgr *UEManager) CreateAllUEs() (map[string]*DUChannels, error) {
	nue := mgr.config.UE.NUE
	baseMSIN := mgr.config.UE.MSIN

	mgr.Info("Creating %d UEs starting from MSIN %s", nue, baseMSIN)

	baseMSINInt, err := strconv.ParseUint(baseMSIN, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid base MSIN: %w", err)
	}

	allChannels := make(map[string]*DUChannels, nue)

	for i := 0; i < nue; i++ {
		msin := fmt.Sprintf("%010d", baseMSINInt+uint64(i))

		ueConf := mgr.config.UE
		ueConf.MSIN = msin

		ue, ch, err := mgr.CreateUE(ueConf)
		if err != nil {
			return nil, fmt.Errorf("failed to create UE %d (MSIN %s): %w", i, msin, err)
		}

		allChannels[msin] = ch
		mgr.Info("Created UE %d/%d: MSIN=%s DU=%s", i+1, nue, msin, ue.GetActiveDUID())

		if err := mgr.applyScenarios(ue, i, nue); err != nil {
			mgr.Error("Failed to apply scenarios to UE %s: %v", msin, err)
		}
	}

	return allChannels, nil
}

func (mgr *UEManager) applyScenarios(ue *UeContext, ueIndex int, totalUEs int) error {
	msin := ue.GetMsin()

	for _, scenario := range mgr.scenarios {
		if !scenario.ShouldApplyToUE(msin, ueIndex, totalUEs) {
			continue
		}

		mgr.Info("Applying scenario '%s' to UE %s", scenario.Name, msin)

		for _, eventEntry := range scenario.Events {
			delay, err := eventEntry.ParseDelay()
			if err != nil {
				return fmt.Errorf("invalid delay for event %s: %w", eventEntry.Type, err)
			}

			eventType := EventType(strings.ToUpper(strings.TrimSpace(eventEntry.Type)))

			eventInfo := EventInfo{
				EventType: eventType,
				Delay:     delay,
				Params:    eventEntry.Params,
			}

			ue.TriggerEvents(eventInfo)
			mgr.Info("Scheduled event '%s' for UE %s with delay %v", eventEntry.Type, msin, delay)
		}
	}

	return nil
}

func (mgr *UEManager) GetUE(msin string) (*UeContext, error) {
	mgr.mutex.RLock()
	defer mgr.mutex.RUnlock()

	ue, exists := mgr.ues[msin]
	if !exists {
		return nil, fmt.Errorf("UE with MSIN %s not found", msin)
	}
	return ue, nil
}

func (mgr *UEManager) GetAllUEs() []*UeContext {
	mgr.mutex.RLock()
	defer mgr.mutex.RUnlock()

	ues := make([]*UeContext, 0, len(mgr.ues))
	for _, ue := range mgr.ues {
		ues = append(ues, ue)
	}
	return ues
}

func (mgr *UEManager) RemoveUE(msin string) error {
	mgr.mutex.Lock()
	defer mgr.mutex.Unlock()

	ue, exists := mgr.ues[msin]
	if !exists {
		return fmt.Errorf("UE with MSIN %s not found", msin)
	}

	ue.Terminate()
	delete(mgr.ues, msin)
	mgr.Info("Removed UE: MSIN=%s", msin)
	return nil
}

func (mgr *UEManager) Shutdown() {
	mgr.mutex.Lock()
	defer mgr.mutex.Unlock()

	mgr.Info("Shutting down UE Manager, terminating %d UEs", len(mgr.ues))

	for msin, ue := range mgr.ues {
		ue.Terminate()
		mgr.Info("Terminated UE: MSIN=%s", msin)
	}

	time.Sleep(200 * time.Millisecond)
	mgr.ues = make(map[string]*UeContext)
	mgr.cancel()
}

func (mgr *UEManager) GetUECount() int {
	mgr.mutex.RLock()
	defer mgr.mutex.RUnlock()
	return len(mgr.ues)
}

func (mgr *UEManager) GetUEsByState(state uint8) []*UeContext {
	mgr.mutex.RLock()
	defer mgr.mutex.RUnlock()

	var result []*UeContext
	for _, ue := range mgr.ues {
		if ue.GetState() == state {
			result = append(result, ue)
		}
	}
	return result
}

// HandoverUEToDU is called by DU target after it receives UE Context Setup Request
// from CU-CP (F1AP). It creates a new DUConnection, injects it into the UE,
// and returns the channels so DU target can wire up its side.
//
// Flow:
//   CU-CP → UE Context Setup Request → DU target
//   DU target → mgr.HandoverUEToDU(msin, targetDUID)  ← this function
//   UE.performRandomAccess unblocks, switches to new conn
//   UE → RRC Reconfiguration Complete → DU target
func (mgr *UEManager) HandoverUEToDU(msin, targetDUID string) (*DUChannels, error) {
	ue, err := mgr.GetUE(msin)
	if err != nil {
		return nil, err
	}

	oldDUID := ue.GetActiveDUID()
	if oldDUID == targetDUID {
		return nil, fmt.Errorf("UE %s is already on DU %s", msin, targetDUID)
	}

	// Create new connection for target DU
	conn := newDUConnection(targetDUID, mgr.ctx)

	// Inject into UE — unblocks WaitForHandoverConnection in performRandomAccess
	ue.InjectHandoverConnection(conn)

	mgr.Info("Handover initiated: UE %s  %s → %s", msin, oldDUID, targetDUID)
	return connToChannels(conn), nil
}

// GetDUChannels returns the live channel pair for a UE's current DU connection.
func (mgr *UEManager) GetDUChannels(msin string) (*DUChannels, error) {
	ue, err := mgr.GetUE(msin)
	if err != nil {
		return nil, err
	}

	conn := ue.GetActiveDUConn()
	if conn == nil {
		return nil, fmt.Errorf("UE %s has no active DU connection", msin)
	}
	return connToChannels(conn), nil
}

func (mgr *UEManager) TriggerEventForAll(eventType EventType, delay time.Duration, params map[string]interface{}) {
	mgr.mutex.RLock()
	defer mgr.mutex.RUnlock()

	eventInfo := EventInfo{
		EventType: eventType,
		Delay:     delay,
		Params:    params,
	}

	for _, ue := range mgr.ues {
		ue.TriggerEvents(eventInfo)
	}
	mgr.Info("Triggered event %s for all %d UEs", eventType, len(mgr.ues))
}

func (mgr *UEManager) TriggerEventForUE(msin string, eventType EventType, delay time.Duration, params map[string]interface{}) error {
	ue, err := mgr.GetUE(msin)
	if err != nil {
		return err
	}

	eventInfo := EventInfo{
		EventType: eventType,
		Delay:     delay,
		Params:    params,
	}

	ue.TriggerEvents(eventInfo)
	mgr.Info("Triggered event %s for UE %s", eventType, msin)
	return nil
}

func connToChannels(conn *DUConnection) *DUChannels {
	return &DUChannels{
		ToUE:   conn.ReceiveFromDu,
		FromUE: conn.SendToDu,
		Ready:  conn.IsReady,
	}
}