package du

import (
	"fmt"

	f1ap "github.com/JocelynWS/f1-gen"
	"github.com/JocelynWS/f1-gen/ies"
)

func (du *DU) HandleUlRrcMessageTransfer(f1apPdu *f1ap.F1apPdu) error {
	du.Info("Handling UL RRC Message Transfer")
	// This is handled by the UE sending RRC messages, not received from CU-CP
	return nil
}

// HandleDlRrcMessageTransfer handles DL RRC Message Transfer from CU-CP
func (du *DU) HandleDlRrcMessageTransfer(f1apPdu *f1ap.F1apPdu) error {
	if f1apPdu.Present != ies.F1apPduInitiatingMessage {
		du.Error("Invalid F1AP PDU present type for DL RRC Message Transfer")
		return fmt.Errorf("invalid PDU type")
	}

	msg, ok := f1apPdu.Message.Msg.(*ies.DLRRCMessageTransfer)
	if !ok {
		du.Error("Failed to cast message to DLRRCMessageTransfer")
		return fmt.Errorf("invalid message type")
	}

	du.Info("DL RRC Message Transfer: CU-UE-ID=%d, DU-UE-ID=%d, SRB-ID=%d",
		msg.GNBCUUEF1APID, msg.GNBDUUEF1APID, msg.SRBID)

	// Extract RRC container and forward to UE
	if len(msg.RRCContainer) == 0 {
		du.Warn("DL RRC Message Transfer has empty RRC container")
		return nil
	}

	// Forward RRC message to UE via channel
	if du.ue != nil && du.ue.SendToUeChannel != nil {
		du.Info("Forwarding RRC message to UE, length: %d", len(msg.RRCContainer))
		du.ue.SendToUeChannel <- msg.RRCContainer
	} else {
		du.Error("UE channel not initialized")
		return fmt.Errorf("UE channel not initialized")
	}

	return nil
}
