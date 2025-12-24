package uecontext

import (
	"github.com/reogac/nas"
)

// handleDlNasTransport extracts N1 SM message from DL NAS Transport
func (ue *UeContext) handleDlNasTransport(message *nas.DlNasTransport) {
	// Check payload container type
	if uint8(message.PayloadContainerType) != nas.PayloadContainerTypeN1SMInfo {
		ue.Error("Error in DL NAS Transport, Payload Container Type not expected value")
		return
	}

	if message.PduSessionId == nil {
		ue.Error("Error in DL NAS Transport, PDU Session ID is missing")
		return
	}

	// Decode N1 SM message
	nasMsg, err := nas.Decode(ue.getNasContext(), message.PayloadContainer, true)
	if err != nil {
		ue.Error("Error in DL NAS Transport, fail to decode N1Sm: %v", err)
		return
	}

	// Handle the N1 SM message
	ue.handleNas_n1sm(&nasMsg)
}
