package du

import (
	"sync"
)

type HandoverRole int

const (
	HANDOVER_ROLE_NONE HandoverRole = iota
	HANDOVER_ROLE_SOURCE
	HANDOVER_ROLE_TARGET
)

type HandoverState int

const (
	HO_STATE_IDLE HandoverState = iota
	HO_STATE_PREPARATION
	HO_STATE_EXECUTION
	HO_STATE_COMPLETION
	HO_STATE_COMPLETED
	HO_STATE_FAILED
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

func (du *DU) InitHandoverContext() {
	du.hoCtx = &HandoverContext{
		role:  HANDOVER_ROLE_NONE,
		state: HO_STATE_IDLE,
	}
}

func (du *DU) SetSourceHandoverState(state HandoverState) {
	if du.hoCtx == nil {
		du.InitHandoverContext()
	}

	du.hoCtx.mutex.Lock()
	defer du.hoCtx.mutex.Unlock()

	du.hoCtx.role = HANDOVER_ROLE_SOURCE
	oldState := du.hoCtx.state
	du.hoCtx.state = state

	du.Info("[SOURCE DU] State transition: %s -> %s",
		handoverStateToString(oldState),
		handoverStateToString(state))
}

func (du *DU) SetTargetHandoverState(state HandoverState) {
	if du.hoCtx == nil {
		du.InitHandoverContext()
	}

	du.hoCtx.mutex.Lock()
	defer du.hoCtx.mutex.Unlock()

	du.hoCtx.role = HANDOVER_ROLE_TARGET
	oldState := du.hoCtx.state
	du.hoCtx.state = state

	du.Info("[TARGET DU] State transition: %s -> %s",
		handoverStateToString(oldState),
		handoverStateToString(state))
}

func (du *DU) GetHandoverState() HandoverState {
	if du.hoCtx == nil {
		return HO_STATE_IDLE
	}

	du.hoCtx.mutex.RLock()
	defer du.hoCtx.mutex.RUnlock()
	return du.hoCtx.state
}

func (du *DU) GetHandoverRole() HandoverRole {
	if du.hoCtx == nil {
		return HANDOVER_ROLE_NONE
	}

	du.hoCtx.mutex.RLock()
	defer du.hoCtx.mutex.RUnlock()
	return du.hoCtx.role
}

func (du *DU) IsSourceDU() bool {
	return du.GetHandoverRole() == HANDOVER_ROLE_SOURCE
}

func (du *DU) IsTargetDU() bool {
	return du.GetHandoverRole() == HANDOVER_ROLE_TARGET
}

func handoverStateToString(state HandoverState) string {
	switch state {
	case HO_STATE_IDLE:
		return "IDLE"
	case HO_STATE_PREPARATION:
		return "PREPARATION"
	case HO_STATE_EXECUTION:
		return "EXECUTION"
	case HO_STATE_COMPLETION:
		return "COMPLETION"
	case HO_STATE_COMPLETED:
		return "COMPLETED"
	case HO_STATE_FAILED:
		return "FAILED"
	default:
		return "UNKNOWN"
	}
}

func (du *DU) ResetHandoverContext() {
	if du.hoCtx != nil {
		du.hoCtx.mutex.Lock()
		defer du.hoCtx.mutex.Unlock()

		du.Info("Resetting handover context")
		du.hoCtx.role = HANDOVER_ROLE_NONE
		du.hoCtx.state = HO_STATE_IDLE
		du.hoCtx.rachCompleted = false
	}
}
