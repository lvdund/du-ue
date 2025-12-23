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
		ue.Error("Receive 5GSM Status: cause=%d", gsm.GsmStatus.GsmCause)

	default:
		ue.Warn("Unknown 5GSM message type: 0x%x", gsm.MsgType)
	}
}

// handlePduSessionEstablishmentAccept processes PDU Session Accept
func (ue *UeContext) handlePduSessionEstablishmentAccept(msg *nas.PduSessionEstablishmentAccept) {
	if msg == nil {
		ue.Error("PDU Session Establishment Accept is nil")
		return
	}

	sessionId := msg.GetSessionId()
	ue.Info("PDU Session %d established successfully", sessionId)

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

// handlePduSessionEstablishmentReject processes PDU Session Reject
func (ue *UeContext) handlePduSessionEstablishmentReject(msg *nas.PduSessionEstablishmentReject) {
	if msg == nil {
		ue.Error("PDU Session Establishment Reject is nil")
		return
	}

	sessionId := msg.GetSessionId()
	cause := msg.GsmCause
	ue.Error("PDU Session %d rejected, cause: %d", sessionId, cause)

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
}
