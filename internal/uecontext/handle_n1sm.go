package uecontext

import (
	"fmt"
	"github.com/reogac/nas"
)

// handleNas_n1sm handles 5G Session Management messages
func (ue *UeContext) handleNas_n1sm(nasMsg *nas.NasMessage) {
	gsm := nasMsg.Gsm
	if gsm == nil {
		ue.Error("NAS message has no GSM content")
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

	sessionId := msg.GetSessionId()
	ue.Info("PDU Session %d established successfully", sessionId)

	// Validations from incoming branch
	if msg.GetPti() != 1 {
		ue.Warn("Warning: PDU Session Establishment Accept, PTI not the expected value (expected 1)")
	}
	if msg.SelectedPduSessionType != 1 {
		ue.Warn("Warning: PDU Session Establishment Accept, PDU Session Type not the expected value (expected 1)")
	}

	// Log received info
	if msg.PduAddress != nil {
		ue.Info("  PDU Address: %v", msg.PduAddress.Content())
	}

	ue.Info("  QoS Rules: %v", msg.AuthorizedQosRules.Bytes)

	if msg.Dnn != nil {
		ue.Info("  DNN: %s", msg.Dnn.String())
	}
	if msg.SNssai != nil {
		ue.Info("  S-NSSAI: SST=%d, SD=%s", msg.SNssai.Sst, msg.SNssai.GetSd())
	}

	// Store session in UE context
	// Using local logic because incoming logic uses missing setters
	var pduAddr string
	if msg.PduAddress != nil {
		// PDU Address is typically 5 bytes: [PDU Type + 4 bytes IPv4]
		content := msg.PduAddress.Content()
		if len(content) >= 5 {
			pduAddr = fmt.Sprintf("%d.%d.%d.%d", content[1], content[2], content[3], content[4])
		}
	}

	var dnnStr string
	if msg.Dnn != nil {
		dnnStr = msg.Dnn.String()
	}

	newSession := &PduSession{
		Id:         uint8(sessionId),
		PduAddress: pduAddr,
		Dnn:        dnnStr,
		SNssai:     msg.SNssai,
		State:      SM5G_PDU_SESSION_ACTIVE,
	}

	ue.mutex.Lock()
	ue.PduSessions[uint8(sessionId)] = newSession
	ue.mutex.Unlock()
}

// handlePduSessionEstablishmentReject processes PDU Session Establishment Reject
func (ue *UeContext) handlePduSessionEstablishmentReject(msg *nas.PduSessionEstablishmentReject) {
	if msg == nil {
		ue.Error("PDU Session Establishment Reject is nil")
		return
	}

	sessionId := msg.GetSessionId()
	cause := msg.GsmCause
	
	// Use the helper from incoming branch for better logging
	ue.Error("PDU Session %d rejected, cause: %d (%s)", sessionId, cause, cause5GSMToString(cause))

	// Remove any pending session state if it exists
	ue.mutex.Lock()
	delete(ue.PduSessions, uint8(sessionId))
	ue.mutex.Unlock()
}

// handlePduSessionReleaseCommand processes PDU Session Release Command
func (ue *UeContext) handlePduSessionReleaseCommand(msg *nas.PduSessionReleaseCommand) {
	if msg == nil {
		ue.Error("PDU Session Release Command is nil")
		return
	}

	sessionId := msg.GetSessionId()
	ue.Info("PDU Session %d release commanded", sessionId)

	// Clean up session from UE context
	ue.mutex.Lock()
	delete(ue.PduSessions, uint8(sessionId))
	ue.mutex.Unlock()

	// TODO: Send PDU Session Release Complete
	// (Incoming branch had logic here, but required methods missing in ue.go)
}

// handleCause5GSM processes 5GSM cause (From incoming branch)
func (ue *UeContext) handleCause5GSM(cause *uint8) {
	if cause != nil {
		ue.Error("UE received a 5GSM Failure, cause: %s", cause5GSMToString(uint8(*cause)))
	}
}

// cause5GSMToString converts 5GSM cause code to string (From incoming branch)
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
