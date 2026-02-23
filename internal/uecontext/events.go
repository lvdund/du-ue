package uecontext

import (
	"time"
)

type EventType string

const (
	EventTypeRRCSetup         EventType = "RRC_SETUP"
	EventTypeRegistration     EventType = "REGISTRATION"
	EventTypePDUEstablishment EventType = "PDU_ESTABLISHMENT"
	EventTypePDURelease       EventType = "PDU_RELEASE"
	EventTypeDeregistration   EventType = "DEREGISTRATION"
	EventTypeHandover         EventType = "HANDOVER"
	EventTypeServiceRequest   EventType = "SERVICE_REQUEST"
)

type EventInfo struct {
	EventType EventType
	Delay     time.Duration
	Params    map[string]interface{}
}

type eventQueue struct {
	events chan EventInfo
}

func (ue *UeContext) initEventQueue() {
	ue.eventQueue = &eventQueue{
		events: make(chan EventInfo, 100),
	}
}

func (ue *UeContext) TriggerEvents(event EventInfo) {
	go func() {
		time.Sleep(event.Delay)

		select {
		case ue.eventQueue.events <- event:
			ue.Info("Event queued: %s", event.EventType)
		case <-ue.ctx.Done():
			ue.Info("Context cancelled, event discarded: %s", event.EventType)
		}
	}()
}

func (ue *UeContext) processEvents() {
	ue.Info("Started event processor")

	for {
		select {
		case event := <-ue.eventQueue.events:
			ue.Info("Processing event: %s", event.EventType)
			ue.handleEvent(event)

		case <-ue.ctx.Done():
			ue.Info("Context cancelled, stopping event processor")
			return
		}
	}
}

func (ue *UeContext) handleEvent(event EventInfo) {
	switch event.EventType {
	case EventTypeRRCSetup:
		ue.handleRRCSetupEvent()
	case EventTypeRegistration:
		ue.handleRegistrationEvent()
	case EventTypePDUEstablishment:
		ue.handlePDUEstablishmentEvent(event.Params)
	case EventTypePDURelease:
		ue.handlePDUReleaseEvent(event.Params)
	case EventTypeDeregistration:
		ue.handleDeregistrationEvent()
	case EventTypeHandover:
		ue.handleHandoverEvent(event.Params)
	case EventTypeServiceRequest:
		ue.handleServiceRequestEvent()
	default:
		ue.Warn("Unknown event type: %s", event.EventType)
	}
}

func (ue *UeContext) handleRRCSetupEvent() {
	ue.Info("Executing RRC Setup event")

	// if the UE has already registered, no need to set up RRC anymore
	if ue.GetState() == UE_STATE_REGISTERED {
		ue.Warn("UE already registered, skipping RRC Setup")
		return
	}

	// call InitRRCConn() to send RRCSetupRequest
	if err := ue.InitRRCConn(); err != nil {
		ue.Error("Failed to initialize RRC connection: %v", err)
	}
}

func (ue *UeContext) handleRegistrationEvent() {
	ue.Info("Executing Registration event")

	if ue.GetState() == UE_STATE_REGISTERED {
		ue.Warn("UE already registered, skipping")
		return
	}

	if err := ue.TriggerInitRegistration(); err != nil {
		ue.Error("Failed to trigger registration: %v", err)
	}
}

func (ue *UeContext) handlePDUEstablishmentEvent(params map[string]interface{}) {
	ue.Info("Executing PDU Establishment event")

	if ue.GetState() != UE_STATE_REGISTERED {
		ue.Warn("UE not registered, cannot establish PDU session")
		return
	}

	if params != nil {
		if dnn, ok := params["dnn"].(string); ok && dnn != "" {
			ue.Info("Establishing PDU session with custom DNN: %s", dnn)
			if err := ue.TriggerCustomPduSession(dnn); err != nil {
				ue.Error("Failed to trigger custom PDU session: %v", err)
			}
			return
		}
	}

	if err := ue.TriggerPduSession(); err != nil {
		ue.Error("Failed to trigger default PDU session: %v", err)
	}
}

func (ue *UeContext) handlePDUReleaseEvent(params map[string]interface{}) {
	ue.Info("Executing PDU Release event")

	if params != nil {
		if sessionId, ok := params["session_id"].(uint8); ok {
			ue.TriggerReleasePduSession(sessionId)
			return
		}
	}

	ue.TriggerReleaseAllPduSessions()
}

func (ue *UeContext) handleDeregistrationEvent() {
	ue.Info("Executing Deregistration event")

	if ue.GetState() != UE_STATE_REGISTERED {
		ue.Warn("UE not registered, cannot deregister")
		return
	}

	ue.TriggerReleaseAllPduSessions()

	time.Sleep(500 * time.Millisecond)

	ue.Info("Deregistration not yet implemented")
}

func (ue *UeContext) handleHandoverEvent(params map[string]interface{}) {
	ue.Info("Executing Handover event")

	// Check if UE is registered
	if ue.GetState() != UE_STATE_REGISTERED {
		ue.Warn("UE not registered, cannot perform handover")
		return
	}

	//Trigger measurement report (prerequisite for handover)
	ue.Info("Triggering measurement report")
	if err := ue.TriggerMeasurement(); err != nil {
		ue.Error("Failed to trigger measurement: %v", err)
		return
	}

	ue.Info("Handover event initiated - waiting for RRC Reconfiguration from network")
}

func (ue *UeContext) handleServiceRequestEvent() {
	ue.Info("Executing Service Request event")
	ue.Info("Service Request not yet implemented")
}
