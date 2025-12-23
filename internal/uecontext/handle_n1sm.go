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

	case nas.GsmStatusMsgType:
		ue.Error("Receive 5GSM Status")
		if gsm.GsmStatus != nil {
			ue.handleCause5GSM(&gsm.GsmStatus.GsmCause)
		}

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

	ue.Info("  QoS Rules: %v", msg.AuthorizedQosRules.Bytes)
	// Get QoS Rules (comment out if field doesn't exist)
	// if msg.AuthorizedQosRules != nil {
	// 	pduSession.Info("PDU session QoS RULES received")
	// }

	// Get DNN
	if msg.Dnn != nil {
		pduSession.Info("PDU session DNN: %s", msg.Dnn.String())
	}

	// Get S-NSSAI
	if msg.SNssai != nil {
		sst := msg.SNssai.Sst
		sd := msg.SNssai.GetSd()
		pduSession.Info("PDU session NSSAI -- sst:%d sd:%s", sst, sd)
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

	pduSessionId := msg.GetSessionId()
	ue.Error("Receiving PDU Session Establishment Reject for session id %d 5GSM Cause: %s",
		pduSessionId, cause5GSMToString(uint8(msg.GsmCause)))

	pduSession := ue.getPduSession(pduSessionId)
	if pduSession == nil {
		ue.Error("Cannot retry PDU Session Request for PDU Session after Reject")
		return
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

	pduSessionId := msg.GetSessionId()
	ue.Info("Receiving PDU Session Release Command for session id = %d", pduSessionId)

	pduSession := ue.getPduSession(pduSessionId)
	if pduSession == nil {
		ue.Error("Unable to delete PDU Session from UE as the PDU Session was not found. Ignoring.")
		return
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