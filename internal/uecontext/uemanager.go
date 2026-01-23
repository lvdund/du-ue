package uecontext

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"
	"strings"

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

func (mgr *UEManager) CreateUE(conf config.UEConfig) (*UeContext, error) {
	mgr.mutex.Lock()
	defer mgr.mutex.Unlock()

	if _, exists := mgr.ues[conf.MSIN]; exists {
		return nil, fmt.Errorf("UE with MSIN %s already exists", conf.MSIN)
	}

	ue := CreateUe(conf, mgr.ctx)
	if ue == nil {
		return nil, fmt.Errorf("failed to create UE context")
	}

	ue.ReceiveFromDuChannel = make(chan []byte, 100)
	ue.SendToDuChannel = make(chan []byte, 100)
	ue.IsReadyConn = make(chan bool, 1)

	// Start goroutines for RRC handler and event processor
	go ue.handleRrcFromDU()
	go ue.processEvents()

	mgr.ues[conf.MSIN] = ue
	mgr.Info("Created and registered UE: MSIN=%s, SUPI=%s", conf.MSIN, ue.supi)

	return ue, nil
}

func (mgr *UEManager) CreateAllUEs() error {
	nue := mgr.config.UE.NUE
	baseMSIN := mgr.config.UE.MSIN

	mgr.Info("Creating %d UEs starting from MSIN %s", nue, baseMSIN)

	baseMSINInt, err := strconv.ParseUint(baseMSIN, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid base MSIN: %w", err)
	}

	for i := 0; i < nue; i++ {
		// Auto-increment MSIN for each UE
		msin := fmt.Sprintf("%010d", baseMSINInt+uint64(i))

		ueConf := mgr.config.UE
		ueConf.MSIN = msin

		ue, err := mgr.CreateUE(ueConf)
		if err != nil {
			return fmt.Errorf("failed to create UE %d (MSIN %s): %w", i, msin, err)
		}

		mgr.Info("Created UE %d/%d: MSIN=%s", i+1, nue, msin)

		if err := mgr.applyScenarios(ue, i, nue); err != nil {
			mgr.Error("Failed to apply scenarios to UE %s: %v", msin, err)
		}
	}

	return nil
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

			if err != nil {
				return fmt.Errorf("invalid event type: %w", err)
			}

			eventInfo := EventInfo{
				EventType: eventType,
				Delay:     delay,
				Params:    eventEntry.Params,
			}

			ue.TriggerEvents(eventInfo)
			mgr.Info("Scheduled event '%s' for UE %s with delay %v", 
				eventEntry.Type, msin, delay)
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
	
	// Wait for goroutines to finish before closing channels
	time.Sleep(100 * time.Millisecond)

	close(ue.ReceiveFromDuChannel)
	close(ue.SendToDuChannel)
	close(ue.IsReadyConn)
	if ue.eventQueue != nil {
		close(ue.eventQueue.events)
	}

	delete(mgr.ues, msin)

	mgr.Info("Removed UE: MSIN=%s", msin)
	return nil
}

func (mgr *UEManager) Shutdown() {
	mgr.mutex.Lock()
	defer mgr.mutex.Unlock()

	mgr.Info("Shutting down UE Manager, terminating %d UEs", len(mgr.ues))

	// Signal all UEs to terminate
	for msin, ue := range mgr.ues {
		ue.Terminate()
		mgr.Info("Terminated UE: MSIN=%s", msin)
	}

	// Wait for goroutines to finish
	time.Sleep(200 * time.Millisecond)

	// Close all channels
	for _, ue := range mgr.ues {
		close(ue.ReceiveFromDuChannel)
		close(ue.SendToDuChannel)
		close(ue.IsReadyConn)
		if ue.eventQueue != nil {
			close(ue.eventQueue.events)
		}
	}

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