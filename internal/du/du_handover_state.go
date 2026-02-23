package du

import (
	"sync"
)

type HandoverRole string

const (
	HANDOVER_ROLE_NONE   HandoverRole = "NONE"
	HANDOVER_ROLE_SOURCE HandoverRole = "SOURCE"
	HANDOVER_ROLE_TARGET HandoverRole = "TARGET"
)

type HandoverState string

const (
	HO_STATE_IDLE        HandoverState = "IDLE"
	HO_STATE_PREPARATION HandoverState = "PREPARATION"
	HO_STATE_EXECUTION   HandoverState = "EXECUTION"
	HO_STATE_COMPLETION  HandoverState = "COMPLETION"
	HO_STATE_COMPLETED   HandoverState = "COMPLETED"
	HO_STATE_FAILED      HandoverState = "FAILED"
)

type HandoverContext struct {
	role          HandoverRole
	state         HandoverState
	targetCellId  int64
	sourceCellId  int64
	cuUeF1apId    int64
	duUeF1apId    int64
	newCRNTI      int64
	rachCompleted bool
	mutex         sync.RWMutex
}

func (ctx *DuUeContext) InitHandoverContext() {
	ctx.HoCtx = &HandoverContext{
		role:  HANDOVER_ROLE_NONE,
		state: HO_STATE_IDLE,
	}
}

func (du *DU) SetSourceHandoverState(ctx *DuUeContext, state HandoverState) {
	if ctx.HoCtx == nil {
		ctx.InitHandoverContext()
	}

	ctx.HoCtx.mutex.Lock()
	defer ctx.HoCtx.mutex.Unlock()

	ctx.HoCtx.role = HANDOVER_ROLE_SOURCE
	oldState := ctx.HoCtx.state
	ctx.HoCtx.state = state

	du.Info("[SOURCE DU][UE %d] State transition: %s -> %s",
		ctx.DuUeF1apId,
		oldState,
		state)
}

func (du *DU) SetTargetHandoverState(ctx *DuUeContext, state HandoverState) {
	if ctx.HoCtx == nil {
		ctx.InitHandoverContext()
	}

	ctx.HoCtx.mutex.Lock()
	defer ctx.HoCtx.mutex.Unlock()

	ctx.HoCtx.role = HANDOVER_ROLE_TARGET
	oldState := ctx.HoCtx.state
	ctx.HoCtx.state = state

	du.Info("[TARGET DU][UE %d] State transition: %s -> %s",
		ctx.DuUeF1apId,
		oldState,
		state)
}

func (du *DU) GetHandoverState(ctx *DuUeContext) HandoverState {
	if ctx == nil || ctx.HoCtx == nil {
		return HO_STATE_IDLE
	}

	ctx.HoCtx.mutex.RLock()
	defer ctx.HoCtx.mutex.RUnlock()
	return ctx.HoCtx.state
}

func (du *DU) GetHandoverRole(ctx *DuUeContext) HandoverRole {
	if ctx == nil || ctx.HoCtx == nil {
		return HANDOVER_ROLE_NONE
	}

	ctx.HoCtx.mutex.RLock()
	defer ctx.HoCtx.mutex.RUnlock()
	return ctx.HoCtx.role
}

func (du *DU) IsSourceDU(ctx *DuUeContext) bool {
	return du.GetHandoverRole(ctx) == HANDOVER_ROLE_SOURCE
}

func (du *DU) IsTargetDU(ctx *DuUeContext) bool {
	return du.GetHandoverRole(ctx) == HANDOVER_ROLE_TARGET
}

func (du *DU) ResetHandoverContext(ctx *DuUeContext) {
	if ctx != nil && ctx.HoCtx != nil {
		ctx.HoCtx.mutex.Lock()
		defer ctx.HoCtx.mutex.Unlock()

		du.Info("[UE %d] Resetting handover context", ctx.DuUeF1apId)
		ctx.HoCtx.role = HANDOVER_ROLE_NONE
		ctx.HoCtx.state = HO_STATE_IDLE
		ctx.HoCtx.rachCompleted = false
	}
}
