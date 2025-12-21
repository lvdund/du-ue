package uecontext

import (
	"fmt"

	"github.com/reogac/nas"
)

// triggerInitPduSessionRequest initiates a new PDU Session
func (ue *UeContext) triggerInitPduSessionRequest(params *map[string]any) error {
	ue.Info("Initiating New PDU Session")

	pduSession, err := ue.createPDUSession()
	if err != nil {
		ue.Error("[UE][NAS] %v", err)
		return err
	}

	pduSession.Info("PDU Session Initiating")

	// Change state to ACTIVE_PENDING
	pduSession.SetState(PDUSessionActivePending)

	// Build and send PDU Session Establishment Request
	ue.triggerInitPduSessionRequestInner(pduSession, params)

	return nil
}

// triggerInitPduSessionRequestInner builds and sends PDU Session Establishment Request
func (ue *UeContext) triggerInitPduSessionRequestInner(
	pduSession *PduSession,
	params *map[string]any,
) {
	n1Sm := new(nas.PduSessionEstablishmentRequest)

	// Set PDU Session Type
	pduType := nas.PduSessionTypeIpv4
	n1Sm.PduSessionType = &pduType

	// Set Integrity Protection Maximum Data Rate
	n1Sm.IntegrityProtectionMaximumDataRate = nas.NewIntegrityProtectionMaximumDataRate(0xff, 0xff)

	// Set PTI and Session ID
	n1Sm.SetPti(1) // Always use PTI = 1 for initial request
	n1Sm.SetSessionId(pduSession.id)

	// Encode N1 SM message
	n1SmPdu, err := nas.EncodeSm(n1Sm)
	if err != nil {
		pduSession.Error("Failed to encode PDU Session Establishment Request: %v", err)
		return
	}

	// Send via UL NAS Transport
	requestType := nas.UlNasTransportRequestTypeInitialRequest
	ue.sendN1Sm(n1SmPdu, pduSession.id, &requestType, params)
}

// triggerInitPduSessionReleaseRequest initiates PDU Session Release
func (ue *UeContext) triggerInitPduSessionReleaseRequest(pduSession *PduSession) {
	ue.Info("Initiating Release of PDU Session %d", pduSession.id)

	if pduSession.GetState() != PDUSessionActive {
		ue.Warn("Skipping releasing PDU Session ID %d as it's not active", pduSession.id)
		return
	}

	// Change state to INACTIVE_PENDING
	pduSession.SetState(PDUSessionInactivePending)

	// Build PDU Session Release Request
	n1Sm := new(nas.PduSessionReleaseRequest)
	n1Sm.SetPti(1)
	n1Sm.SetSessionId(pduSession.id)

	n1SmPdu, _ := nas.EncodeSm(n1Sm)
	ue.sendN1Sm(n1SmPdu, pduSession.id, nil, nil)
}

// triggerInitPduSessionReleaseComplete sends PDU Session Release Complete
func (ue *UeContext) triggerInitPduSessionReleaseComplete(pduSession *PduSession) {
	ue.Info("Initiating PDU Session Release Complete for PDU Session: %d", pduSession.id)

	if pduSession.GetState() == PDUSessionInactive {
		ue.Warn("Unable to send PDU Session Release Complete for a PDU Session which is inactive")
		return
	}

	// Build PDU Session Release Complete
	n1Sm := new(nas.PduSessionReleaseComplete)
	n1Sm.SetPti(1) // Must be same as received command message
	n1Sm.SetSessionId(pduSession.id)

	n1SmPdu, _ := nas.EncodeSm(n1Sm)
	ue.sendN1Sm(n1SmPdu, pduSession.id, nil, nil)

	// Mark session as inactive and release
	pduSession.SetState(PDUSessionInactive)
	ue.releasePduSession(pduSession.id)
}

// TriggerDefaultPduSession triggers PDU session with default parameters
func (ue *UeContext) TriggerDefaultPduSession() error {
	params := map[string]any{
		"dnn": "internet", // Default DNN
	}

	return ue.triggerInitPduSessionRequest(&params)
}

// TriggerCustomPduSession triggers PDU session with custom parameters
func (ue *UeContext) TriggerCustomPduSession(dnn string) error {
	params := map[string]any{
		"dnn": dnn,
	}

	return ue.triggerInitPduSessionRequest(&params)
}

// TriggerReleasePduSession releases a specific PDU session
func (ue *UeContext) TriggerReleasePduSession(sessionId uint8) {
	pduSession := ue.getPduSession(sessionId)
	if pduSession == nil {
		ue.Warn("PDU Session %d not found", sessionId)
		return
	}

	ue.triggerInitPduSessionReleaseRequest(pduSession)
}

// TriggerReleaseAllPduSessions releases all active PDU sessions
func (ue *UeContext) TriggerReleaseAllPduSessions() {
	activeSessions := ue.getActivePduSessions()
	if len(activeSessions) == 0 {
		ue.Info("No active PDU sessions to release")
		return
	}

	ue.Info("Releasing %d active PDU sessions", len(activeSessions))
	for _, session := range activeSessions {
		ue.triggerInitPduSessionReleaseRequest(session)
	}
}

// PDU Session management helpers

// createPDUSession creates a new PDU session with available ID
func (ue *UeContext) createPDUSession() (*PduSession, error) {
	ue.mutex.Lock()
	defer ue.mutex.Unlock()

	// Find available session ID (1-15)
	for i := 1; i <= 15; i++ {
		if ue.sessions[i] == nil {
			session := NewPduSession(ue, uint8(i))
			ue.sessions[i] = session
			ue.Info("Created PDU Session with ID: %d", i)
			return session, nil
		}
	}

	return nil, fmt.Errorf("no available PDU Session ID (max 15 sessions)")
}

// getPduSession retrieves PDU session by ID
func (ue *UeContext) getPduSession(sessionId uint8) *PduSession {
	if sessionId == 0 || sessionId > 15 {
		return nil
	}
	ue.mutex.Lock()
	defer ue.mutex.Unlock()
	return ue.sessions[sessionId]
}

// releasePduSession releases a PDU session
func (ue *UeContext) releasePduSession(sessionId uint8) {
	if sessionId == 0 || sessionId > 15 {
		return
	}
	ue.mutex.Lock()
	defer ue.mutex.Unlock()

	if ue.sessions[sessionId] != nil {
		ue.Info("Released PDU Session ID: %d", sessionId)
		ue.sessions[sessionId] = nil
	}
}

// getActivePduSessions returns all active PDU sessions
func (ue *UeContext) getActivePduSessions() []*PduSession {
	ue.mutex.Lock()
	defer ue.mutex.Unlock()

	var active []*PduSession
	for i := 1; i <= 15; i++ {
		if ue.sessions[i] != nil && ue.sessions[i].IsActive() {
			active = append(active, ue.sessions[i])
		}
	}
	return active
}

// sendN1Sm wraps N1 SM message in UL NAS Transport and sends it
func (ue *UeContext) sendN1Sm(
	n1SmPdu []byte,
	pduSessionId uint8,
	requestType *uint8,
	params *map[string]any,
) {
	// Create UL NAS Transport
	ulNasTransport := &nas.UlNasTransport{
		PayloadContainerType: nas.PayloadContainerTypeN1SMInfo,
		PayloadContainer:     n1SmPdu,
		PduSessionId:         &pduSessionId,
	}

	if requestType != nil {
		ulNasTransport.RequestType = requestType  
	}

	// Add optional parameters from params map
	if params != nil {
		if dnn, ok := (*params)["dnn"].(string); ok && dnn != "" {
			ulNasTransport.Dnn = nas.NewDnn(dnn)  
		}
	}

	// Set security header (integrity + cipher)
	ulNasTransport.SetSecurityHeader(nas.NasSecBoth)

	// Encode with security context
	nasCtx := ue.getNasContext()
	nasPdu, err := nas.EncodeMm(nasCtx, ulNasTransport)
	if err != nil {
		ue.Error("Failed to encode UL NAS Transport: %v", err)
		return
	}

	ue.Info("Sending N1 SM message (session %d) via UL NAS Transport", pduSessionId)
	ue.Send_UlInformationTransfer_To_Du(nasPdu)
}
