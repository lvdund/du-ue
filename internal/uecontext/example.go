package uecontext

import (
	"du_ue/pkg/config"
	"time"
)

func CreateUEs(cfg *config.UEConfig) []*UeContext {
	for range cfg.NUE {
		// create ue
	}
	return nil
}

type EventType string

type EventInfo struct {
	EventType EventType
	Delay     time.Duration
}

func (ue *UeContext) TriggerEvents(event *EventInfo) {
	// For loop
	// ue.TriggerRegistrationRequest()
	// event.Delay excute
}
