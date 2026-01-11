package uecontext

import (
	"fmt"

	"github.com/lvdund/rrc"
	rrcies "github.com/lvdund/rrc/ies"
)

// HandleRrcMessage handles RRC messages received from DU
func (ue *UeContext) HandleRrcMessage(rrcBytes []byte) error {
	if len(rrcBytes) == 0 {
		ue.Error("RRC message is empty")
		return fmt.Errorf("empty RRC message")
	}

	ue.Info("Handling RRC message, length: %d", len(rrcBytes))

	// Try to decode as DL-DCCH-Message first (most common after RRC Setup)
	var dlDcchMsg rrcies.DL_DCCH_Message
	if err := rrc.Decode(rrcBytes, &dlDcchMsg); err == nil {
		return ue.handleDlDcchMessage(&dlDcchMsg)
	}

	// Try to decode as DL-CCCH-Message (for RRC Setup)
	var dlCcchMsg rrcies.DL_CCCH_Message
	if err := rrc.Decode(rrcBytes, &dlCcchMsg); err == nil {
		return ue.handleDlCcchMessage(&dlCcchMsg)
	}

	ue.Error("Failed to decode RRC message")
	return fmt.Errorf("failed to decode RRC message")
}

// handleDlCcchMessage handles DL-CCCH messages (RRC Setup)
func (ue *UeContext) handleDlCcchMessage(msg *rrcies.DL_CCCH_Message) error {
	if msg.Message.C1 == nil {
		return fmt.Errorf("invalid DL-CCCH message structure")
	}

	switch msg.Message.C1.Choice {
	case rrcies.DL_CCCH_MessageType_C1_Choice_RrcSetup:
		ue.Info("Received RRC Setup")
		return ue.handleRrcSetup(msg.Message.C1.RrcSetup)
	case rrcies.DL_CCCH_MessageType_C1_Choice_RrcReject:
		ue.Error("Received RRC Reject")
		return fmt.Errorf("RRC connection rejected")
	default:
		ue.Warn("Received unknown DL-CCCH message type")
	}

	return nil
}

// handleDlDcchMessage handles DL-DCCH messages (after RRC connection established)
func (ue *UeContext) handleDlDcchMessage(msg *rrcies.DL_DCCH_Message) error {
	if msg.Message.C1 == nil {
		return fmt.Errorf("invalid DL-DCCH message structure")
	}

	switch msg.Message.C1.Choice {
	case rrcies.DL_DCCH_MessageType_C1_Choice_DlInformationTransfer:
		ue.Info("Received DL Information Transfer")
		return ue.handleDlInformationTransfer(msg.Message.C1.DlInformationTransfer)

	case rrcies.DL_DCCH_MessageType_C1_Choice_RrcReconfiguration:
		ue.Info("Received RRC Reconfiguration")
		return ue.handleRrcReconfigurationMessage(msg.Message.C1.RrcReconfiguration)

	case rrcies.DL_DCCH_MessageType_C1_Choice_RrcRelease:
		ue.Info("Received RRC Release")
		return ue.handleRrcRelease(msg.Message.C1.RrcRelease)

	default:
		ue.Warn("Received unknown DL-DCCH message type: %d", msg.Message.C1.Choice)
	}

	return nil
}

// handleRrcReconfigurationMessage handles RRC Reconfiguration (both normal and handover)
func (ue *UeContext) handleRrcReconfigurationMessage(msg *rrcies.RRCReconfiguration) error {
	if msg.CriticalExtensions.RrcReconfiguration == nil {
		return fmt.Errorf("invalid RRC Reconfiguration structure")
	}

	rrcReconfig := msg.CriticalExtensions.RrcReconfiguration

	// Check if this is a handover by checking SecondaryCellGroup
	isHandover := false
	if rrcReconfig.SecondaryCellGroup != nil {
		// Decode SecondaryCellGroup to check for ReconfigurationWithSync
		var cellGroupConfig rrcies.CellGroupConfig
		if err := rrc.Decode(*rrcReconfig.SecondaryCellGroup, &cellGroupConfig); err == nil {
			if cellGroupConfig.SpCellConfig != nil && 
				cellGroupConfig.SpCellConfig.ReconfigurationWithSync != nil {
				isHandover = true
				ue.Info("RRC Reconfiguration contains handover command")
				return ue.handleHandoverReconfiguration(&cellGroupConfig)
			}
		}
	}

	if !isHandover {
		// Normal RRC Reconfiguration (not handover)
		ue.Info("Processing normal RRC Reconfiguration")
		
		// Extract NAS PDU if present in NonCriticalExtension
		if rrcReconfig.NonCriticalExtension != nil &&
			rrcReconfig.NonCriticalExtension.DedicatedNAS_MessageList != nil &&
			len(rrcReconfig.NonCriticalExtension.DedicatedNAS_MessageList) > 0 {
			ue.Info("RRC Reconfiguration contains NAS message")
			nasMsg := rrcReconfig.NonCriticalExtension.DedicatedNAS_MessageList[0]
			// FIX: HandleNasMsg returns void, not error
			ue.HandleNasMsg(nasMsg.Value)
		}
	}

	// Send RRC Reconfiguration Complete
	return ue.sendRrcReconfigurationComplete()
}

// handleHandoverReconfiguration handles RRC Reconfiguration for handover
func (ue *UeContext) handleHandoverReconfiguration(cellGroupConfig *rrcies.CellGroupConfig) error {
	ue.Info("Handling Handover Reconfiguration")

	syncReconfig := cellGroupConfig.SpCellConfig.ReconfigurationWithSync

	// Extract target cell information
	if syncReconfig.SpCellConfigCommon != nil {
		ue.Info("Target cell configuration received")
	}

	// FIX: NewUE_Identity is RNTI_Value (not pointer), check Value field directly
	// RNTI_Value is a struct with a Value field (int64)
	if syncReconfig.NewUE_Identity.Value != 0 {
		ue.Info("New C-RNTI assigned: %d", syncReconfig.NewUE_Identity.Value)
	}

	// Perform Random Access to target cell
	ue.Info("Initiating Random Access to target cell")
	go ue.performRandomAccess()

	return nil
}

// handleRrcSetup handles RRC Setup message
func (ue *UeContext) handleRrcSetup(msg *rrcies.RRCSetup) error {
	ue.Info("Processing RRC Setup")

	// Send RRC Setup Complete with Registration Request
	return ue.sendRrcSetupComplete()
}

// handleDlInformationTransfer handles DL Information Transfer (contains NAS message)
func (ue *UeContext) handleDlInformationTransfer(msg *rrcies.DLInformationTransfer) error {
	if msg.CriticalExtensions.DlInformationTransfer == nil {
		return fmt.Errorf("invalid DL Information Transfer structure")
	}

	dlInfo := msg.CriticalExtensions.DlInformationTransfer
	if dlInfo.DedicatedNAS_Message == nil {
		return fmt.Errorf("no NAS message in DL Information Transfer")
	}

	// Extract and handle NAS message
	nasBytes := dlInfo.DedicatedNAS_Message.Value
	ue.Info("Extracted NAS message from DL Information Transfer, length: %d", len(nasBytes))
	
	// FIX: HandleNasMsg returns void, not error
	ue.HandleNasMsg(nasBytes)
	return nil
}

// handleRrcRelease handles RRC Release message
func (ue *UeContext) handleRrcRelease(msg *rrcies.RRCRelease) error {
	ue.Info("Processing RRC Release")
	ue.SetState(UE_STATE_DEREGISTERED)
	return nil
}

// sendRrcSetupComplete sends RRC Setup Complete with Registration Request
func (ue *UeContext) sendRrcSetupComplete() error {
	ue.Info("Sending RRC Setup Complete")

	// Trigger Registration Request
	if err := ue.TriggerInitRegistration(); err != nil {
		return fmt.Errorf("failed to create registration request: %w", err)
	}

	// Get the NAS PDU
	nasPdu := ue.nasPdu

	// FIX: SelectedPLMN_Identity is int64, not struct
	// Create RRC Setup Complete message
	rrcSetupComplete := &rrcies.RRCSetupComplete{
		Rrc_TransactionIdentifier: rrcies.RRC_TransactionIdentifier{Value: 0},
		CriticalExtensions: rrcies.RRCSetupComplete_CriticalExtensions{
			Choice: rrcies.RRCSetupComplete_CriticalExtensions_Choice_RrcSetupComplete,
			RrcSetupComplete: &rrcies.RRCSetupComplete_IEs{
				SelectedPLMN_Identity: 1, // This is int64 with range 1..maxPLMN
				DedicatedNAS_Message: rrcies.DedicatedNAS_Message{
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
				Choice:           rrcies.UL_DCCH_MessageType_C1_Choice_RrcSetupComplete,
				RrcSetupComplete: rrcSetupComplete,
			},
		},
	}

	encoded, err := rrc.Encode(&ulDcchMsg)
	if err != nil {
		ue.Error("Failed to encode RRC Setup Complete: %v", err)
		return err
	}

	// Send to DU
	ue.SendToDuChannel <- encoded
	ue.Info("RRC Setup Complete sent successfully")
	return nil
}