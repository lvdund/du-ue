package uecontext

import (
	"github.com/lvdund/rrc"
	rrcies "github.com/lvdund/rrc/ies"
)

// HandleRrcMsg decodes RRC messages from DU and extracts NAS messages
func (ue *UeContext) HandleRrcMsg(rrcMessageBytes []byte) error {
	ue.Info("Handle RRC message, length: %d bytes, %v", len(rrcMessageBytes), rrcMessageBytes)
	if len(rrcMessageBytes) == 0 {
		ue.Error("RRC message is empty")
		return nil
	}

	msg := rrcies.DL_DCCH_Message{}
	err := rrc.Decode(rrcMessageBytes, &msg)
	if err != nil {
		ue.Error("Failed to decode RRC message: %v", err)
		return err
	}

	return ue.handleDLDCCHMessage(&msg)
}

// handleDLDCCHMessage processes DL-DCCH messages and extracts NAS
func (ue *UeContext) handleDLDCCHMessage(msg *rrcies.DL_DCCH_Message) error {
	if msg.Message.Choice != rrcies.DL_DCCH_MessageType_Choice_C1 {
		ue.Warn("DL-DCCH message has unsupported choice type")
		return nil
	}

	c1 := msg.Message.C1
	if c1 == nil {
		ue.Warn("DL-DCCH C1 is nil")
		return nil
	}

	switch c1.Choice {
	case rrcies.DL_DCCH_MessageType_C1_Choice_DlInformationTransfer:
		// Extract NAS from DLInformationTransfer
		if c1.DlInformationTransfer != nil {
			return ue.handleDLInformationTransfer(c1.DlInformationTransfer)
		}

	case rrcies.DL_DCCH_MessageType_C1_Choice_RrcReconfiguration:
		// Extract NAS from RRCReconfiguration
		if c1.RrcReconfiguration != nil {
			return ue.HandleRrcReconfiguration(c1.RrcReconfiguration)
		}

	case rrcies.DL_DCCH_MessageType_C1_Choice_SecurityModeCommand:
		// Handle SecurityModeCommand (AS security, not NAS)
		ue.Info("Received SecurityModeCommand (AS security)")
		// TODO: Handle AS security mode command if needed
		return nil

	default:
		ue.Warn("Unhandled DL-DCCH message type: %v", c1.Choice)
	}

	return nil
}

// handleDLInformationTransfer extracts NAS message from DLInformationTransfer
func (ue *UeContext) handleDLInformationTransfer(msg *rrcies.DLInformationTransfer) error {
	ue.Info("Received DLInformationTransfer")

	// Extract NAS from CriticalExtensions -> DlInformationTransfer -> DedicatedNAS_Message
	if msg.CriticalExtensions.Choice != rrcies.DLInformationTransfer_CriticalExtensions_Choice_DlInformationTransfer {
		ue.Warn("DLInformationTransfer has unsupported CriticalExtensions choice")
		return nil
	}

	ies := msg.CriticalExtensions.DlInformationTransfer
	if ies.DedicatedNAS_Message == nil || len(ies.DedicatedNAS_Message.Value) == 0 {
		ue.Warn("DLInformationTransfer has no NAS message")
		return nil
	}

	nasBytes := ies.DedicatedNAS_Message.Value
	ue.Info("Extracted NAS message from DLInformationTransfer, length: %d", len(nasBytes))

	// Forward to NAS handler
	ue.HandleNasMsg(nasBytes)
	return nil
}

// sendUlInformationTransfer wraps NAS PDU in RRC UL Information Transfer
func (ue *UeContext) sendUlInformationTransfer(nasPdu []byte) error {
	ue.Info("Wrapping NAS PDU in UL Information Transfer, NAS length: %d", len(nasPdu))
	
	// Create UL Information Transfer message
	ulInfoTransfer := &rrcies.ULInformationTransfer{
		CriticalExtensions: rrcies.ULInformationTransfer_CriticalExtensions{
			Choice: rrcies.ULInformationTransfer_CriticalExtensions_Choice_UlInformationTransfer,
			UlInformationTransfer: &rrcies.ULInformationTransfer_IEs{
				DedicatedNAS_Message: &rrcies.DedicatedNAS_Message{
					Value: nasPdu,
				},
			},
		},
	}
	
	// Wrap in UL-DCCH-Message
	ulDcchMsg := rrcies.UL_DCCH_Message{
		Message: rrcies.UL_DCCH_MessageType{
			Choice: rrcies.UL_DCCH_MessageType_Choice_C1,
			C1: &rrcies.UL_DCCH_MessageType_C1{
				Choice:                 rrcies.UL_DCCH_MessageType_C1_Choice_UlInformationTransfer,
				UlInformationTransfer: ulInfoTransfer,
			},
		},
	}
	
	// Encode RRC message
	encoded, err := rrc.Encode(&ulDcchMsg)
	if err != nil {
		ue.Error("Failed to encode UL Information Transfer: %v", err)
		return err
	}
	
	// Send to DU
	if err := ue.sendToActiveDU(encoded); err != nil {
    ue.Error("Failed to send UL Information Transfer: %v", err)
    return err
}
	ue.Info("UL Information Transfer sent successfully, RRC length: %d", len(encoded))
	return nil
}