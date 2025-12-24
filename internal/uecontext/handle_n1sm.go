package uecontext

import (
	"github.com/reogac/nas"
)

// handleNas_n1sm handles NAS Session Management messages
func (ue *UeContext) handleNas_n1sm(nasMsg *nas.NasMessage) {
	gsm := nasMsg.Gsm
	if gsm == nil {
		ue.Error("Err in DL NAS Transport, N1Sm is missing")
		return
	}

	switch gsm.MsgType {
	case nas.PduSessionEstablishmentAcceptMsgType:
		ue.Info("Receive PDU Session Establishment Accept")
		ue.handlePduSessionEstablishmentAccept(gsm.PduSessionEstablishmentAccept)

	case nas.PduSessionEstablishmentRejectMsgType:
		ue.Error("Receive PDU Session Establishment Reject")
		ue.handlePduSessionEstablishmentReject(gsm.PduSessionEstablishmentReject)

	case nas.PduSessionReleaseCommandMsgType:
		ue.Info("Receive PDU Session Release Command")
		ue.handlePduSessionReleaseCommand(gsm.PduSessionReleaseCommand)

	case nas.PduSessionAuthenticationCommandMsgType:
		ue.Info("Receive PDU Session Authentication Command")
		ue.handlePduSessionAuthenticationCommand(gsm.PduSessionAuthenticationCommand)

	case nas.PduSessionAuthenticationResultMsgType:
		ue.Info("Receive PDU Session Authentication Result")
		ue.handlePduSessionAuthenticationResult(gsm.PduSessionAuthenticationResult)

	case nas.PduSessionModificationCommandMsgType:
		ue.Info("Receive PDU Session Modification Command")
		ue.handlePduSessionModificationCommand(gsm.PduSessionModificationCommand)

	case nas.PduSessionModificationRejectMsgType:
		ue.Error("Receive PDU Session Modification Reject")
		ue.handlePduSessionModificationReject(gsm.PduSessionModificationReject)

	case nas.PduSessionReleaseRejectMsgType:
		ue.Error("Receive PDU Session Release Reject")
		ue.handlePduSessionReleaseReject(gsm.PduSessionReleaseReject)

	case nas.GsmStatusMsgType:
		ue.Info("Receive 5GSM Status")
		ue.handleGsmStatus(gsm.GsmStatus)

	default:
		ue.Warn("Unknown 5GSM message type: 0x%x", gsm.MsgType)
	}
}

// handlePduSessionEstablishmentAccept processes PDU Session Establishment Accept
func (ue *UeContext) handlePduSessionEstablishmentAccept(msg *nas.PduSessionEstablishmentAccept) {
	if msg == nil {
		ue.Error("PDU Session Establishment Accept is nil")
		return
	}

	ue.Info("Receiving PDU Session Establishment Accept")

	if msg.GetPti() != 1 {
		ue.Error("Error in PDU Session Establishment Accept, PTI not the expected value")
		return
	}
	if msg.SelectedPduSessionType != 1 {
		ue.Error("Error in PDU Session Establishment Accept, PDU Session Type not the expected value")
		return
	}

	// Update PDU Session information
	pduSessionId := msg.GetSessionId()

	// Helper method (to be implemented)
	pduSession := ue.getPduSession(pduSessionId)
	if pduSession == nil {
		ue.Error("Receiving PDU Session Establishment Accept about an unknown PDU Session, id: %d", pduSessionId)
		return
	}

	// Get PDU Address (IP)
	if msg.PduAddress != nil {
		ueIp := msg.PduAddress
		pduSession.setIp(ueIp.Content())
		pduSession.Info("PDU address received: %s", pduSession.ueIP)
	}

	// Get QoS Rules and store as AuthorizedQosRules
	pduSession.AuthorizedQosRules = &msg.AuthorizedQosRules
	ue.Info("  Authorized QoS Rules: %v", msg.AuthorizedQosRules.Bytes)

	// Get Authorized QoS Flow Descriptions
	if msg.AuthorizedQosFlowDescriptions != nil {
		pduSession.AuthorizedQosFlowDescriptions = msg.AuthorizedQosFlowDescriptions
		ue.Info("  Authorized QoS Flow Descriptions: %v", msg.AuthorizedQosFlowDescriptions.Bytes)
	}

	// Get Extended Protocol Configuration Options
	if msg.ExtendedProtocolConfigurationOptions != nil {
		pduSession.ExtendedProtocolConfigurationOptions = msg.ExtendedProtocolConfigurationOptions
		for _, unit := range msg.ExtendedProtocolConfigurationOptions.Units() {
			ue.Info("  PCO Unit: Id=0x%x Len=%d", unit.Id, len(unit.Content))
		}
	}

	// Get DNN
	if msg.Dnn != nil {
		pduSession.Dnn = msg.Dnn
		pduSession.Info("PDU session DNN: %s", pduSession.Dnn)
	}

	// Get S-NSSAI
	if msg.SNssai != nil {
		sst := msg.SNssai.Sst
		sd := msg.SNssai.GetSd()
		// Use helper to set (creates a deep copy logic via Set)
		if err := pduSession.SetSNssai(sst, sd); err != nil {
			ue.Error("Failed to set SNssai: %v", err)
		}
		pduSession.Info("PDU session NSSAI -- sst:%d sd:%s", sst, sd)

		if msg.SNssai.Mapped != nil {
			mappedSd := msg.SNssai.GetMappedSd()
			if err := pduSession.SetMappedSNssai(msg.SNssai.Mapped.Sst, mappedSd); err != nil {
				ue.Error("Failed to set Mapped SNssai: %v", err)
			}
			pduSession.Info("  Mapped NSSAI -- sst:%d sd:%s", msg.SNssai.Mapped.Sst, mappedSd)
		}
	}

	// Get Session-AMBR
	// SessionAmbr is Mandatory, so it is not a pointer in the message struct
	pduSession.SessionAmbr = &msg.SessionAmbr
	pduSession.Info("PDU session AMBR: %v", msg.SessionAmbr.Bytes)

	// Get Selected SSC Mode (Mandatory)
	// SSC modes: 1, 2, 3 (TS 23.501)
	if msg.SelectedSscMode < 1 || msg.SelectedSscMode > 3 {
		ue.Error("Error in PDU Session Establishment Accept, Invalid Selected SSC Mode: %d", msg.SelectedSscMode)
		// We could release here, but for now we'll just log and verify the build
	}
	pduSession.SscMode = msg.SelectedSscMode
	pduSession.Info("Selected SSC Mode: %d", pduSession.SscMode)

	// Log Unhandled Optional IEs
	if msg.GsmCause != nil {
		ue.Info("  [Unhandled IE] 5GSM Cause: %d", *msg.GsmCause)
	}
	if msg.RqTimerValue != nil {
		ue.Info("  [Unhandled IE] RQ Timer Value: %d", *msg.RqTimerValue)
	}
	if msg.AlwaysOnPduSessionIndication != nil {
		ue.Info("  [Unhandled IE] Always-on PDU Session Indication: %d", *msg.AlwaysOnPduSessionIndication)
	}
	if len(msg.MappedEpsBearerContexts) > 0 {
		ue.Info("  [Unhandled IE] Mapped EPS Bearer Contexts present")
	}
	if len(msg.EapMessage) > 0 {
		ue.Info("  [Unhandled IE] EAP Message present")
	}
	if len(msg.GsmNetworkFeatureSupport) > 0 {
		ue.Info("  [Unhandled IE] 5GSM Network Feature Support present")
	}
	if msg.ServingPlmnRateControl != nil {
		ue.Info("  [Unhandled IE] Serving PLMN Rate Control: %d", *msg.ServingPlmnRateControl)
	}
	if len(msg.AtsssContainer) > 0 {
		ue.Info("  [Unhandled IE] ATSSS Container present")
	}
	if msg.ControlPlaneOnlyIndication != nil {
		ue.Info("  [Unhandled IE] Control Plane Only Indication: %d", *msg.ControlPlaneOnlyIndication)
	}
	if len(msg.IpHeaderCompressionConfiguration) > 0 {
		ue.Info("  [Unhandled IE] IP Header Compression Configuration present")
	}
	if msg.EthernetHeaderCompressionConfiguration != nil {
		ue.Info("  [Unhandled IE] Ethernet Header Compression Configuration: %d", *msg.EthernetHeaderCompressionConfiguration)
	}
	if len(msg.ServiceLevelAaContainer) > 0 {
		ue.Info("  [Unhandled IE] Service Level AA Container present")
	}
	if len(msg.ReceivedMbsContainer) > 0 {
		ue.Info("  [Unhandled IE] Received MBS Container present")
	}

	// Change state to ACTIVE
	pduSession.SetState(PDUSessionActive)
	pduSession.Info("PDU Session established successfully")
}

// handlePduSessionEstablishmentReject processes PDU Session Establishment Reject
func (ue *UeContext) handlePduSessionEstablishmentReject(msg *nas.PduSessionEstablishmentReject) {
	if msg == nil {
		ue.Error("PDU Session Establishment Reject is nil")
		return
	}

	ue.Info("Receiving PDU Session Establishment Reject")

	// Check PTI
	if msg.GetPti() != 1 {
		ue.Error("Error in PDU Session Establishment Reject, PTI not the expected value")
		return
	}

	pduSessionId := msg.GetSessionId()

	pduSession := ue.getPduSession(pduSessionId)
	if pduSession == nil {
		ue.Error("Receiving PDU Session Establishment Reject about an unknown PDU Session, id: %d", pduSessionId)
		return
	}

	ue.Error("Receiving PDU Session Establishment Reject for session id %d 5GSM Cause: %s",
		pduSessionId, cause5GSMToString(uint8(msg.GsmCause)))

	// Log Unhandled Optional IEs
	if msg.BackOffTimerValue != nil {
		// New Logic: Decode and store Back-off Timer
		pduSession.BackOffTimer = DecodeGPRSTimer3(msg.BackOffTimerValue.Value)
		ue.Info("  [Handled IE] Back-off Timer Value: %v (Decoded: %s)", *msg.BackOffTimerValue, pduSession.BackOffTimer)
	}
	if msg.AllowedSscMode != nil {
		// New Logic: Store Allowed SSC Mode
		pduSession.AllowedSscMode = *msg.AllowedSscMode
		ue.Info("  [Handled IE] Allowed SSC Mode: %d", pduSession.AllowedSscMode)
	}
	if len(msg.EapMessage) > 0 {
		ue.Info("  [Unhandled IE] EAP Message present")
	}
	if msg.GsmCongestionReAttemptIndicator != nil {
		ue.Info("  [Unhandled IE] 5GSM Congestion Re-attempt Indicator: %d", *msg.GsmCongestionReAttemptIndicator)
	}
	if msg.ExtendedProtocolConfigurationOptions != nil {
		ue.Info("  [Unhandled IE] Extended Protocol Configuration Options present")
		for _, unit := range msg.ExtendedProtocolConfigurationOptions.Units() {
			ue.Info("    PCO Unit: Id=0x%x Len=%d", unit.Id, len(unit.Content))
		}
	}
	if msg.ReAttemptIndicator != nil {
		ue.Info("  [Unhandled IE] Re-attempt Indicator: %d", *msg.ReAttemptIndicator)
	}
	if len(msg.ServiceLevelAaContainer) > 0 {
		ue.Info("  [Unhandled IE] Service Level AA Container present")
	}

	// Release the session
	pduSession.SetState(PDUSessionInactive)
	ue.releasePduSession(pduSessionId)
}

// handlePduSessionReleaseCommand processes PDU Session Release Command
func (ue *UeContext) handlePduSessionReleaseCommand(msg *nas.PduSessionReleaseCommand) {
	if msg == nil {
		ue.Error("PDU Session Release Command is nil")
		return
	}

	// Check PTI
	// PTI is located in the SM Header
	if msg.GetPti() == 0 {
		ue.Warn("PDU Session Release Command with PTI=0 (reserved)")
		// Proceeding anyway as it might be network initiated with PTI 0?
		// But usually network initiated uses the PTI of the request if it's a response,
		// or a specific value.
		// For Release Command (Network Initiated), PTI typically identifies the procedure.
		// Proceeding for now but logging warning.
	}

	pduSessionId := msg.GetSessionId()
	// Log 5GSM Cause (Mandatory)
	ue.Info("Receiving PDU Session Release Command for session id %d 5GSM Cause: %s",
		pduSessionId, cause5GSMToString(msg.GsmCause))

	pduSession := ue.getPduSession(pduSessionId)
	if pduSession == nil {
		ue.Error("Unable to delete PDU Session from UE as the PDU Session was not found. Ignoring.")
		return
	}

	// Handle Optional IEs

	// Back-off Timer
	if msg.BackOffTimerValue != nil {
		pduSession.BackOffTimer = DecodeGPRSTimer3(msg.BackOffTimerValue.Value)
		ue.Info("  [Handled IE] Back-off Timer Value: %v (Decoded: %s)", *msg.BackOffTimerValue, pduSession.BackOffTimer)
	}

	// Log Unhandled Optional IEs
	if len(msg.EapMessage) > 0 {
		ue.Info("  [Unhandled IE] EAP Message present")
	}
	if msg.GsmCongestionReAttemptIndicator != nil {
		ue.Info("  [Unhandled IE] 5GSM Congestion Re-attempt Indicator: %d", *msg.GsmCongestionReAttemptIndicator)
	}
	if msg.ExtendedProtocolConfigurationOptions != nil {
		ue.Info("  [Unhandled IE] Extended Protocol Configuration Options present")
		for _, unit := range msg.ExtendedProtocolConfigurationOptions.Units() {
			ue.Info("    PCO Unit: Id=0x%x Len=%d", unit.Id, len(unit.Content))
		}
	}
	if msg.AccessType != nil {
		ue.Info("  [Unhandled IE] Access Type: %d", *msg.AccessType)
	}
	if len(msg.ServiceLevelAaContainer) > 0 {
		ue.Info("  [Unhandled IE] Service Level AA Container present")
	}

	// Send PDU Session Release Complete
	ue.triggerInitPduSessionReleaseComplete(pduSession)
}

// handleCause5GSM processes 5GSM cause
func (ue *UeContext) handleCause5GSM(cause *uint8) {
	if cause != nil {
		ue.Error("UE received a 5GSM Failure, cause: %s", cause5GSMToString(uint8(*cause)))
	}
}

// handlePduSessionAuthenticationCommand handles PDU Session Authentication Command
func (ue *UeContext) handlePduSessionAuthenticationCommand(msg *nas.PduSessionAuthenticationCommand) {
	if msg == nil {
		ue.Error("PDU Session Authentication Command is nil")
		return
	}

	// Check PTI
	if msg.GetPti() == 0 {
		ue.Warn("PDU Session Authentication Command with PTI=0")
	}

	pduSessionId := msg.GetSessionId()
	ue.Info("Receiving PDU Session Authentication Command for session id %d", pduSessionId)

	// Log EAP Message (Mandatory)
	ue.Info("  EAP Message Length: %d", len(msg.EapMessage))

	// Log Optional IEs
	if msg.ExtendedProtocolConfigurationOptions != nil {
		ue.Info("  Extended Protocol Configuration Options present")
		for _, unit := range msg.ExtendedProtocolConfigurationOptions.Units() {
			ue.Info("    PCO Unit: Id=0x%x Len=%d", unit.Id, len(unit.Content))
		}
	}

	// NOTE: We do not implement EAP handling, so we cannot respond.
	// In a real scenario, we would process the EAP payload and send an Authentication Complete.
}

// handlePduSessionAuthenticationResult handles PDU Session Authentication Result
func (ue *UeContext) handlePduSessionAuthenticationResult(msg *nas.PduSessionAuthenticationResult) {
	if msg == nil {
		ue.Error("PDU Session Authentication Result is nil")
		return
	}

	// Check PTI
	if msg.GetPti() == 0 {
		ue.Warn("PDU Session Authentication Result with PTI=0")
	}

	pduSessionId := msg.GetSessionId()
	ue.Info("Receiving PDU Session Authentication Result for session id %d", pduSessionId)

	// Log EAP Message (Optional)
	if len(msg.EapMessage) > 0 {
		ue.Info("  EAP Message Length: %d", len(msg.EapMessage))
	}

	// Log Optional IEs
	if msg.ExtendedProtocolConfigurationOptions != nil {
		ue.Info("  Extended Protocol Configuration Options present")
		for _, unit := range msg.ExtendedProtocolConfigurationOptions.Units() {
			ue.Info("    PCO Unit: Id=0x%x Len=%d", unit.Id, len(unit.Content))
		}
	}
}

// handlePduSessionModificationCommand handles PDU Session Modification Command
func (ue *UeContext) handlePduSessionModificationCommand(msg *nas.PduSessionModificationCommand) {
	if msg == nil {
		ue.Error("PDU Session Modification Command is nil")
		return
	}

	// Check PTI
	if msg.GetPti() == 0 {
		ue.Warn("PDU Session Modification Command with PTI=0")
	}

	pduSessionId := msg.GetSessionId()
	ue.Info("Receiving PDU Session Modification Command for session id %d", pduSessionId)

	pduSession := ue.getPduSession(pduSessionId)
	if pduSession == nil {
		ue.Error("PDU Session Modification Command for unknown PDU Session ID %d", pduSessionId)
		return
	}

	// Change state to MODIFICATION_PENDING
	pduSession.SetState(PDUSessionModificationPending)

	// Log Optional IEs
	if msg.GsmCause != nil {
		ue.Info("  5GSM Cause: %s", cause5GSMToString(*msg.GsmCause))
	}

	// Store Session AMBR
	if msg.SessionAmbr != nil {
		pduSession.SessionAmbr = msg.SessionAmbr
		ue.Info("  Session AMBR: %v", msg.SessionAmbr.Bytes)
	}

	if msg.RqTimerValue != nil {
		ue.Info("  RQ Timer Value: %d", *msg.RqTimerValue)
	}

	// Store Always-on PDU Session Indication
	if msg.AlwaysOnPduSessionIndication != nil {
		pduSession.AlwaysOnPduSessionIndication = *msg.AlwaysOnPduSessionIndication
		ue.Info("  Always-on PDU Session Indication: %d", *msg.AlwaysOnPduSessionIndication)
	}

	// Store Authorized QoS Rules
	if msg.AuthorizedQosRules != nil {
		pduSession.AuthorizedQosRules = msg.AuthorizedQosRules
		ue.Info("  Authorized QoS Rules present")
	}

	if len(msg.MappedEpsBearerContexts) > 0 {
		ue.Info("  Mapped EPS Bearer Contexts present")
	}

	// Store Authorized QoS Flow Descriptions
	if msg.AuthorizedQosFlowDescriptions != nil {
		pduSession.AuthorizedQosFlowDescriptions = msg.AuthorizedQosFlowDescriptions
		ue.Info("  Authorized QoS Flow Descriptions present")
	}

	// Decode/Log Extended Protocol Configuration Options
	if msg.ExtendedProtocolConfigurationOptions != nil {
		pduSession.ExtendedProtocolConfigurationOptions = msg.ExtendedProtocolConfigurationOptions
		ue.Info("  Extended Protocol Configuration Options present")
		for _, unit := range msg.ExtendedProtocolConfigurationOptions.Units() {
			ue.Info("    PCO Unit: Id=0x%x Len=%d", unit.Id, len(unit.Content))
		}
	}

	if len(msg.AtsssContainer) > 0 {
		ue.Info("  ATSSS Container present")
	}
	if len(msg.IpHeaderCompressionConfiguration) > 0 {
		ue.Info("  IP Header Compression Configuration present")
	}
	if len(msg.PortManagementInformationContainer) > 0 {
		ue.Info("  Port Management Information Container present")
	}
	if msg.ServingPlmnRateControl != nil {
		ue.Info("  Serving PLMN Rate Control: %d", *msg.ServingPlmnRateControl)
	}
	if msg.EthernetHeaderCompressionConfiguration != nil {
		ue.Info("  Ethernet Header Compression Configuration: %d", *msg.EthernetHeaderCompressionConfiguration)
	}
	if len(msg.ReceivedMbsContainer) > 0 {
		ue.Info("  Received MBS Container present")
	}
	if len(msg.ServiceLevelAaContainer) > 0 {
		ue.Info("  Service Level AA Container present")
	}

	// Send PDU Session Modification Complete
	ue.triggerInitPduSessionModificationComplete(pduSession)

	// Change state back to ACTIVE
	pduSession.SetState(PDUSessionActive)
}

// handleGsmStatus handles 5GSM Status
func (ue *UeContext) handleGsmStatus(msg *nas.GsmStatus) {
	if msg == nil {
		ue.Error("5GSM Status is nil")
		return
	}

	id := msg.GetSessionId()
	pduSession := ue.getPduSession(id)

	if pduSession != nil {
		ue.Info("5GSM Status for PDU Session ID %d (IP: %s): %s",
			id, pduSession.ueIP, cause5GSMToString(msg.GsmCause))
	} else {
		ue.Error("5GSM Status for unknown PDU Session ID %d: %s",
			id, cause5GSMToString(msg.GsmCause))
	}
}

// cause5GSMToString converts 5GSM cause code to string
func cause5GSMToString(cause uint8) string {
	// Common 5GSM causes from TS 24.501
	switch cause {
	case 26:
		return "Insufficient resources"
	case 27:
		return "Missing or unknown DNN"
	case 28:
		return "Unknown PDU session type"
	case 29:
		return "User authentication or authorization failed"
	case 31:
		return "Request rejected, unspecified"
	case 32:
		return "Service option not supported"
	case 33:
		return "Requested service option not subscribed"
	case 35:
		return "PTI already in use"
	case 36:
		return "Regular deactivation"
	case 38:
		return "Network failure"
	case 39:
		return "Reactivation requested"
	case 41:
		return "Semantic error in the TFT operation"
	case 42:
		return "Syntactical error in the TFT operation"
	case 43:
		return "Invalid PDU session identity"
	case 44:
		return "Semantic errors in packet filter"
	case 45:
		return "Syntactical error in packet filter"
	case 46:
		return "Out of LADN service area"
	case 47:
		return "PTI mismatch"
	case 50:
		return "PDU session type IPv4 only allowed"
	case 51:
		return "PDU session type IPv6 only allowed"
	case 54:
		return "PDU session does not exist"
	case 67:
		return "Insufficient resources for specific slice and DNN"
	case 68:
		return "Not supported SSC mode"
	case 69:
		return "Insufficient resources for specific slice"
	case 70:
		return "Missing or unknown DNN in a slice"
	case 81:
		return "Invalid PTI value"
	case 82:
		return "Maximum data rate per UE for user-plane integrity protection is too low"
	case 83:
		return "Semantic error in the QoS operation"
	case 84:
		return "Syntactical error in the QoS operation"
	case 85:
		return "Invalid mapped EPS bearer identity"
	case 95:
		return "Semantically incorrect message"
	case 96:
		return "Invalid mandatory information"
	case 97:
		return "Message type non-existent or not implemented"
	case 98:
		return "Message type not compatible with the protocol state"
	case 99:
		return "Information element non-existent or not implemented"
	case 100:
		return "Conditional IE error"
	case 101:
		return "Message not compatible with the protocol state"
	case 111:
		return "Protocol error, unspecified"
	default:
		return "Unknown cause"
	}
}

// handlePduSessionReleaseReject handles PDU Session Release Reject
func (ue *UeContext) handlePduSessionReleaseReject(msg *nas.PduSessionReleaseReject) {
	if msg == nil {
		ue.Error("PDU Session Release Reject is nil")
		return
	}

	// Check PTI
	if msg.GetPti() == 0 {
		ue.Warn("PDU Session Release Reject with PTI=0")
	}

	pduSessionId := msg.GetSessionId()
	ue.Info("Receiving PDU Session Release Reject for session id %d", pduSessionId)

	// Log Cause
	ue.Error("  5GSM Cause: %s", cause5GSMToString(uint8(msg.GsmCause)))

	pduSession := ue.getPduSession(pduSessionId)
	if pduSession != nil {
		// Revert state to ACTIVE if we were pending release
		if pduSession.GetState() == PDUSessionInactivePending {
			ue.Warn("  Reverting PDU Session %d state to ACTIVE due to Release Reject", pduSessionId)
			pduSession.SetState(PDUSessionActive)
		}
	}

	// Log Optional IEs
	if msg.ExtendedProtocolConfigurationOptions != nil {
		ue.Info("  Extended Protocol Configuration Options present")
		for _, unit := range msg.ExtendedProtocolConfigurationOptions.Units() {
			ue.Info("    PCO Unit: Id=0x%x Len=%d", unit.Id, len(unit.Content))
		}
	}
}

// handlePduSessionModificationReject handles PDU Session Modification Reject
func (ue *UeContext) handlePduSessionModificationReject(msg *nas.PduSessionModificationReject) {
	if msg == nil {
		ue.Error("PDU Session Modification Reject is nil")
		return
	}

	// Check PTI
	if msg.GetPti() == 0 {
		ue.Warn("PDU Session Modification Reject with PTI=0")
	}

	pduSessionId := msg.GetSessionId()
	ue.Info("Receiving PDU Session Modification Reject for session id %d", pduSessionId)

	// Log Cause
	ue.Error("  5GSM Cause: %s", cause5GSMToString(uint8(msg.GsmCause)))

	pduSession := ue.getPduSession(pduSessionId)
	if pduSession != nil {
		// Revert state to ACTIVE if we were modification pending
		if pduSession.GetState() == PDUSessionModificationPending {
			ue.Warn("  Reverting PDU Session %d state to ACTIVE due to Modification Reject", pduSessionId)
			pduSession.SetState(PDUSessionActive)
		}
	}

	// Log Optional IEs
	if msg.ExtendedProtocolConfigurationOptions != nil {
		ue.Info("  Extended Protocol Configuration Options present")
		for _, unit := range msg.ExtendedProtocolConfigurationOptions.Units() {
			ue.Info("    PCO Unit: Id=0x%x Len=%d", unit.Id, len(unit.Content))
		}
	}
	if msg.BackOffTimerValue != nil {
		ue.Info("  Back-off Timer Value present")
	}
	if msg.GsmCongestionReAttemptIndicator != nil {
		ue.Info("  5GSM Congestion Re-attempt Indicator present")
	}
	if msg.ReAttemptIndicator != nil {
		ue.Info("  Re-attempt Indicator present")
	}
}
