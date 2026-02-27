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

	// Find UE Context
	ctx := du.ueMgr.GetContextByDuId(msg.GNBDUUEF1APID)
	if ctx == nil {
		du.Error("UE context not found (DU-UE-ID=%d)", msg.GNBDUUEF1APID)
		return fmt.Errorf("UE context not found")
	}

	// Update CU UE ID if not already set (e.g., from first DL message like RRCSetup)
	if ctx.CuUeF1apId == 0 && msg.GNBCUUEF1APID > 0 {
		ctx.CuUeF1apId = msg.GNBCUUEF1APID
		du.ueMgr.AddContext(ctx) // to update secondary indexes safely
		du.Info("[UE %d] Updated CU_UE_ID to %d from DLRRCMessageTransfer", ctx.DuUeF1apId, ctx.CuUeF1apId)
	}

	// Extract RRC container and forward to UE
	if len(msg.RRCContainer) == 0 {
		du.Warn("DL RRC Message Transfer has empty RRC container")
		return nil
	}

	// Forward RRC message to UE via channel
	if ctx.UeChannel != nil && ctx.UeChannel.SendToUeChannel != nil {
		du.Info("[UE %d] Forwarding RRC message to UE, length: %d", ctx.DuUeF1apId, len(msg.RRCContainer))
		ctx.UeChannel.SendToUeChannel <- msg.RRCContainer
	} else {
		du.Error("[UE %d] UE channel not initialized", ctx.DuUeF1apId)
		return fmt.Errorf("UE channel not initialized")
	}

	return nil
}
