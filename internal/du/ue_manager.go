package du

import (
	"sync"
)

// DuUeContext represents the DU-side context for a UE
type DuUeContext struct {
	DuUeF1apId    int64
	CuUeF1apId    int64
	CRnti         int64
	UeChannel     *UeChannel
	HoCtx         *HandoverContext
	PduSessions   map[int64]*GnbPDUSession // PDU Sessions, mapped by PDU Session ID
	State         UeState
	SpCellID      string // NRCGI (PLMN + CellID)
	ServCellIndex int64
	Srb1Active    bool
	Srb2Active    bool
	CuToDuRrcInfo []byte        // Cache CU-to-DU RRC constraints
	SrbPriorities map[int64]int // Maps SRB ID to Scheduling Priority (1=Highest)
	mu            sync.RWMutex
}

// SetState updates the UE state thread-safely
func (ctx *DuUeContext) SetState(newState UeState) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.State = newState
}

// GetState retrieves the UE state thread-safely
func (ctx *DuUeContext) GetState() UeState {
	ctx.mu.RLock()
	defer ctx.mu.RUnlock()
	return ctx.State
}

// UeManager manages multiple UE contexts in the DU
type UeManager struct {
	contexts map[int64]*DuUeContext // Keyed by DuUeF1apId (Primary)
	byCuId   map[int64]*DuUeContext // Keyed by CuUeF1apId (Secondary)
	byCrnti  map[int64]*DuUeContext // Keyed by CRnti (Secondary)
	mu       sync.RWMutex
}

// NewUeManager creates a new UE Manager
func NewUeManager() *UeManager {
	return &UeManager{
		contexts: make(map[int64]*DuUeContext),
		byCuId:   make(map[int64]*DuUeContext),
		byCrnti:  make(map[int64]*DuUeContext),
	}
}

// AddContext adds or updates a UE context
func (m *UeManager) AddContext(ctx *DuUeContext) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// If context already exists, we might need to clean up old secondary keys if they changed (unlikely for now but good practice)
	// For simplicity, we just overwrite/set all keys
	m.contexts[ctx.DuUeF1apId] = ctx

	if ctx.CuUeF1apId > 0 {
		m.byCuId[ctx.CuUeF1apId] = ctx
	}
	if ctx.CRnti > 0 {
		m.byCrnti[ctx.CRnti] = ctx
	}
}

// GetContextByDuId retrieves a context by DU-side F1AP ID
func (m *UeManager) GetContextByDuId(duId int64) *DuUeContext {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.contexts[duId]
}

// GetContextByCuId retrieves a context by CU-side F1AP ID
func (m *UeManager) GetContextByCuId(cuId int64) *DuUeContext {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.byCuId[cuId]
}

// GetContextByCRnti retrieves a context by C-RNTI
func (m *UeManager) GetContextByCRnti(crnti int64) *DuUeContext {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.byCrnti[crnti]
}

// RemoveContext removes a UE context
func (m *UeManager) RemoveContext(duId int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	ctx, exists := m.contexts[duId]
	if !exists {
		return
	}

	delete(m.contexts, duId)
	if ctx.CuUeF1apId > 0 {
		delete(m.byCuId, ctx.CuUeF1apId)
	}
	if ctx.CRnti > 0 {
		delete(m.byCrnti, ctx.CRnti)
	}
}

// GetAllContexts returns all tracked contexts
func (m *UeManager) GetAllContexts() []*DuUeContext {
	m.mu.RLock()
	defer m.mu.RUnlock()
	all := make([]*DuUeContext, 0, len(m.contexts))
	for _, ctx := range m.contexts {
		all = append(all, ctx)
	}
	return all
}

// UeState represents the RRC state of the UE as perceived by the DU
// Based on 3GPP TS 38.331
type UeState int

const (
	// UE_STATE_IDLE: No RRC connection established. Context exists but is not active.
	UE_STATE_IDLE UeState = iota

	// UE_STATE_INACTIVE: RRC connection suspended. Context saved.
	// (Placeholder for future implementation)
	UE_STATE_INACTIVE

	// UE_STATE_CONNECTED: RRC connection established. SRBs/DRBs active.
	UE_STATE_CONNECTED
)

var stateNames = map[UeState]string{
	UE_STATE_IDLE:      "RRC_IDLE",
	UE_STATE_INACTIVE:  "RRC_INACTIVE",
	UE_STATE_CONNECTED: "RRC_CONNECTED",
}

func (s UeState) String() string {
	if name, ok := stateNames[s]; ok {
		return name
	}
	return "UNKNOWN"
}
