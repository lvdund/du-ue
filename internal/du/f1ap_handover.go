package du

import (
	"fmt"

	f1ap "github.com/JocelynWS/f1-gen"
	"github.com/JocelynWS/f1-gen/ies"
)

// HandleUeContextModificationRequest handles UE Context Modification Request (contains RRC Reconfiguration for handover)
func (du *DU) HandleUeContextModificationRequest(f1apPdu *f1ap.F1apPdu) error {
	du.Info("Handling UE Context Modification Request (Handover)")

	if f1apPdu.Present != ies.F1apPduInitiatingMessage {
		du.Error("Invalid F1AP PDU present type")
		return fmt.Errorf("invalid PDU type")
	}

	msg, ok := f1apPdu.Message.Msg.(*ies.UEContextModificationRequest)
	if !ok {
		du.Error("Failed to cast message to UEContextModificationRequest")
		return fmt.Errorf("invalid message type")
	}

	du.Info("UE Context Modification Request: CU-UE-ID=%d, DU-UE-ID=%d",
		msg.GNBCUUEF1APID, msg.GNBDUUEF1APID)

	// Check if this is handover-related (contains RRC Reconfiguration)
	if len(msg.RRCContainer) > 0 {
		du.Info("Contains RRC Reconfiguration for Handover, forwarding to UE")
		// Forward RRC Reconfiguration to UE
		if du.ue != nil && du.ue.SendToUeChannel != nil {
			du.ue.SendToUeChannel <- msg.RRCContainer
		} else {
			return fmt.Errorf("UE channel not initialized")
		}

		// Stop scheduling UE on source cell
		du.Info("Stop scheduling UE on source cell")
		// TODO: Actual scheduling stop logic here
	}

	// Send UE Context Modification Response
	return du.sendUeContextModificationResponse(msg.GNBCUUEF1APID, msg.GNBDUUEF1APID)
}

// sendUeContextModificationResponse sends response back to CU-CP
func (du *DU) sendUeContextModificationResponse(cuUeId, duUeId int64) error {
	du.Info("Sending UE Context Modification Response")

	// Build mandatory DUtoCURRCInformation
	duToCuRrcInfo := &ies.DUtoCURRCInformation{
		CellGroupConfig: []byte{}, // Empty for now
	}

	msg := &ies.UEContextModificationResponse{
		GNBCUUEF1APID:        cuUeId,
		GNBDUUEF1APID:        duUeId,
		DUtoCURRCInformation: duToCuRrcInfo, // Add mandatory field (pointer)
	}

	f1apBytes, err := f1ap.F1apEncode(msg)
	if err != nil {
		return fmt.Errorf("encode UE Context Modification Response: %w", err)
	}

	// Send only if f1Client is available (for testing)
	if du.f1Client != nil {
		return du.f1Client.Send(f1apBytes)
	}

	du.Info("F1 client not available, skipping send (test mode)")
	return nil
}

// HandleUeContextReleaseCommand handles UE Context Release Command (from source CU-CP after handover)
func (du *DU) HandleUeContextReleaseCommand(f1apPdu *f1ap.F1apPdu) error {
	du.Info("Handling UE Context Release Command")

	if f1apPdu.Present != ies.F1apPduInitiatingMessage {
		du.Error("Invalid F1AP PDU present type")
		return fmt.Errorf("invalid PDU type")
	}

	msg, ok := f1apPdu.Message.Msg.(*ies.UEContextReleaseCommand)
	if !ok {
		du.Error("Failed to cast message to UEContextReleaseCommand")
		return fmt.Errorf("invalid message type")
	}

	du.Info("UE Context Release Command: CU-UE-ID=%d, DU-UE-ID=%d",
		msg.GNBCUUEF1APID, msg.GNBDUUEF1APID)

	// Release UE context and resources
	du.Info("Releasing UE context and resources")
	// TODO: Actual resource release logic

	// Send UE Context Release Complete
	return du.sendUeContextReleaseComplete(msg.GNBCUUEF1APID, msg.GNBDUUEF1APID)
}

// sendUeContextReleaseComplete sends release complete to CU-CP
func (du *DU) sendUeContextReleaseComplete(cuUeId, duUeId int64) error {
	du.Info("Sending UE Context Release Complete")

	msg := &ies.UEContextReleaseComplete{
		GNBCUUEF1APID: cuUeId,
		GNBDUUEF1APID: duUeId,
	}

	f1apBytes, err := f1ap.F1apEncode(msg)
	if err != nil {
		return fmt.Errorf("encode UE Context Release Complete: %w", err)
	}

	// Send only if f1Client is available (for testing)
	if du.f1Client != nil {
		return du.f1Client.Send(f1apBytes)
	}

	du.Info("F1 client not available, skipping send (test mode)")
	return nil
}

// sendMeasurementReport sends RRC Measurement Report (wrapped in F1AP UL RRC Message Transfer)
func (du *DU) sendMeasurementReport(measurementReport []byte) error {
	du.Info("Forwarding Measurement Report to CU-CP")
	// Use existing sendULRRCMessageTransfer function
	return du.sendULRRCMessageTransfer(measurementReport)
}
