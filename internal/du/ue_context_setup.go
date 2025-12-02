package du

import (
	"fmt"

	f1ap "github.com/JocelynWS/f1-gen"
	"github.com/JocelynWS/f1-gen/ies"
)

// HandleUeContextSetupRequest handles UE Context Setup Request from CU-CP
func (du *DU) HandleUeContextSetupRequest(f1apPdu *f1ap.F1apPdu) error {
	du.Info("Handling UE Context Setup Request")

	if f1apPdu.Present != ies.F1apPduInitiatingMessage {
		du.Error("Invalid F1AP PDU present type for UE Context Setup Request")
		return fmt.Errorf("invalid PDU type")
	}

	msg, ok := f1apPdu.Message.Msg.(*ies.UEContextSetupRequest)
	if !ok {
		du.Error("Failed to cast message to UEContextSetupRequest")
		return fmt.Errorf("invalid message type")
	}

	du.Info("UE Context Setup Request: CU-UE-ID=%d, DU-UE-ID=%d",
		msg.GNBCUUEF1APID, msg.GNBDUUEF1APID)

	// Extract RRC container if present (RRCReconfiguration)
	if len(msg.RRCContainer) > 0 {
		du.Info("UE Context Setup Request contains RRC container, forwarding to UE")
		if du.ue != nil && du.ue.SendToUeChannel != nil {
			du.ue.SendToUeChannel <- msg.RRCContainer
		}
	}

	// Send UE Context Setup Response
	duUeId := int64(DU_UE_F1AP_ID)
	if msg.GNBDUUEF1APID != nil {
		duUeId = *msg.GNBDUUEF1APID
	}
	return du.sendUeContextSetupResponse(msg.GNBCUUEF1APID, duUeId)
}

// sendUeContextSetupResponse sends UE Context Setup Response to CU-CP
func (du *DU) sendUeContextSetupResponse(cuUeId, duUeId int64) error {
	du.Info("Sending UE Context Setup Response")

	// Create UE Context Setup Response
	msg := ies.UEContextSetupResponse{
		GNBCUUEF1APID: cuUeId,
		GNBDUUEF1APID: duUeId,
		// TODO: Add other required fields if needed
	}

	// Encode F1AP message directly (F1apEncode takes the message struct)
	f1apBytes, err := f1ap.F1apEncode(&msg)
	if err != nil {
		return fmt.Errorf("encode UE Context Setup Response: %w", err)
	}

	// Send via SCTP (PPID=62 is already set in Send method)
	return du.f1Client.Send(f1apBytes)
}

func (du *DU) HandleUeContextSetupResponse(f1apPdu *f1ap.F1apPdu) error {
	du.Info("Handling UE Context Setup Response")
	// This is typically not received by DU, but handle if needed
	return nil
}
