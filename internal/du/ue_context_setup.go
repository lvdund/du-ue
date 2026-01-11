package du

import (
	"fmt"

	f1ap "github.com/JocelynWS/f1-gen"
	"github.com/JocelynWS/f1-gen/ies"
	"github.com/lvdund/ngap/aper"
)

// HandleUeContextSetupRequest handles UE Context Setup Request from CU-CP
func (du *DU) HandleUeContextSetupRequest(f1apPdu *f1ap.F1apPdu) error {
	if f1apPdu.Present != ies.F1apPduInitiatingMessage {
		du.Error("Invalid F1AP PDU present type for UE Context Setup Request")
		return fmt.Errorf("invalid PDU type")
	}

	msg, ok := f1apPdu.Message.Msg.(*ies.UEContextSetupRequest)
	if !ok {
		du.Error("Failed to cast message to UEContextSetupRequest")
		return fmt.Errorf("invalid message type")
	}

	// Check if this is handover preparation (Target DU)
	if du.hoCtx != nil && du.GetHandoverState() == HO_STATE_PREPARATION {
		du.Info("[TARGET DU] Handling UE Context Setup Request (Handover)")
		return du.handleTargetHandoverSetup(msg)
	}

	// Normal UE Context Setup
	du.Info("Handling UE Context Setup Request")
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

// handleTargetHandoverSetup handles handover preparation at Target DU
func (du *DU) handleTargetHandoverSetup(msg *ies.UEContextSetupRequest) error {
	du.Info("[TARGET DU] UE Context Setup Request: CU-UE-ID=%d", msg.GNBCUUEF1APID)

	// Store UE IDs
	if msg.GNBDUUEF1APID != nil {
		DU_UE_F1AP_ID = *msg.GNBDUUEF1APID
	}
	CU_UE_F1AP_ID = msg.GNBCUUEF1APID

	// Allocate resources
	du.Info("[TARGET DU] Allocating resources for handover UE")
	if err := du.allocateHandoverResources(); err != nil {
		du.Error("Failed to allocate resources: %v", err)
		return du.sendUeContextSetupFailure(msg.GNBCUUEF1APID, DU_UE_F1AP_ID)
	}

	// Create UE context
	du.createTargetHandoverUeContext()

	// Start RACH monitoring
	du.StartRachMonitoring()

	// Send response
	return du.sendUeContextSetupResponse(msg.GNBCUUEF1APID, DU_UE_F1AP_ID)
}

// allocateHandoverResources allocates resources for handover UE
func (du *DU) allocateHandoverResources() error {
	du.Info("[TARGET DU] Allocating C-RNTI, PRBs, RACH resources")
	// TODO: Actual resource allocation
	return nil
}

// createTargetHandoverUeContext creates UE context for handover
func (du *DU) createTargetHandoverUeContext() {
	du.Info("[TARGET DU] Creating UE context for handover")

	if du.ue == nil {
		toUE := make(chan []byte, 100)
		fromUE := make(chan []byte, 100)

		du.ue = &UeChannel{
			ReceiveFromUeChannel: fromUE,
			SendToUeChannel:      toUE,
		}

		go du.handleRrcFromUE()
	}

	du.Info("[TARGET DU] Ready, waiting for UE RACH")
}

// sendUeContextSetupFailure sends failure response
func (du *DU) sendUeContextSetupFailure(cuUeId, duUeId int64) error {
	du.Error("[TARGET DU] Sending UE Context Setup Failure")

	msg := &ies.UEContextSetupFailure{
		GNBCUUEF1APID: cuUeId,
		GNBDUUEF1APID: &duUeId,
	}

	f1apBytes, err := f1ap.F1apEncode(msg)
	if err != nil {
		return fmt.Errorf("encode failure: %w", err)
	}

	// Send only if f1Client is available (for testing)
	if du.f1Client != nil {
		return du.f1Client.Send(f1apBytes)
	}

	du.Info("F1 client not available, skipping send (test mode)")
	return nil
}

// sendUeContextSetupResponse sends UE Context Setup Response to CU-CP
func (du *DU) sendUeContextSetupResponse(cuUeId, duUeId int64) error {
	du.Info("Sending UE Context Setup Response")

	// Build PLMN Identity (3 bytes)
	plmnBytes := make([]byte, 3)
	mcc := du.Config.PLMN.MCC
	mnc := du.Config.PLMN.MNC

	mcc1 := mcc[0] - '0'
	mcc2 := mcc[1] - '0'
	mcc3 := mcc[2] - '0'

	mnc1 := mnc[0] - '0'
	mnc2 := mnc[1] - '0'
	mnc3 := byte(0xF) // Default filler for 2-digit MNC
	if len(mnc) == 3 {
		mnc3 = mnc[2] - '0'
	}

	plmnBytes[0] = (mcc2 << 4) | mcc1
	plmnBytes[1] = (mnc3 << 4) | mcc3
	plmnBytes[2] = (mnc2 << 4) | mnc1

	// Build NR Cell Identity (36 bits from PCI)
	pci := uint64(du.Config.Cell.PCI)
	cellId := pci << 4 // Shift to make 36 bits with proper alignment

	nrCellIdentityBytes := make([]byte, 5) // 36 bits = 5 bytes
	nrCellIdentityBytes[0] = byte(cellId >> 32)
	nrCellIdentityBytes[1] = byte(cellId >> 24)
	nrCellIdentityBytes[2] = byte(cellId >> 16)
	nrCellIdentityBytes[3] = byte(cellId >> 8)
	nrCellIdentityBytes[4] = byte(cellId)

	// Create NRCGI using aper.BitString
	nrcgi := &ies.NRCGI{
		PLMNIdentity: plmnBytes,
		NRCellIdentity: aper.BitString{
			Bytes:   nrCellIdentityBytes,
			NumBits: 36,
		},
	}

	// Build mandatory DUtoCURRCInformation
	duToCuRrcInfo := ies.DUtoCURRCInformation{
		CellGroupConfig: []byte{}, // Empty for now
	}

	// Optional C-RNTI (as pointer)
	crnti := int64(C_RNTI)

	msg := &ies.UEContextSetupResponse{
		GNBCUUEF1APID:               cuUeId,
		GNBDUUEF1APID:               duUeId,
		DUtoCURRCInformation:        duToCuRrcInfo,
		CRNTI:                       &crnti,
		RequestedTargetCellGlobalID: nrcgi,
	}

	// Encode the message
	f1apBytes, err := f1ap.F1apEncode(msg)
	if err != nil {
		return fmt.Errorf("encode UE Context Setup Response: %w", err)
	}

	// Send only if f1Client is available (for testing)
	if du.f1Client != nil {
		return du.f1Client.Send(f1apBytes)
	}

	du.Info("F1 client not available, skipping send (test mode)")
	return nil
}

func (du *DU) HandleUeContextSetupResponse(f1apPdu *f1ap.F1apPdu) error {
	du.Info("Handling UE Context Setup Response")
	return nil
}

// StartRachMonitoring starts monitoring for RACH preambles
func (du *DU) StartRachMonitoring() {
	du.Info("[TARGET DU] Starting RACH monitoring for handover UE")

	go func() {
		du.Info("[TARGET DU] Waiting for UE to send RACH preamble...")
	}()
}

// HandleRandomAccessPreamble handles RACH Msg1 from handover UE
func (du *DU) HandleRandomAccessPreamble(preambleId int, ueId int64) error {
	du.Info("[TARGET DU] Received Random Access Preamble (Msg1)")
	du.Info("  - Preamble ID: %d", preambleId)
	du.Info("  - Detected timing advance")

	// Send Random Access Response (Msg2)
	return du.sendRandomAccessResponse(preambleId, ueId)
}

// sendRandomAccessResponse sends RAR (Msg2) to UE
func (du *DU) sendRandomAccessResponse(preambleId int, ueId int64) error {
	du.Info("[TARGET DU] Sending Random Access Response (Msg2)")
	du.Info("  - Assigning Timing Advance")
	du.Info("  - Granting UL resources for Msg3")
	du.Info("  - Assigning new C-RNTI: %d", C_RNTI)

	// TODO: Actual RAR PDU creation and transmission
	return nil
}

// HandleRrcReconfigurationComplete handles RRC Reconfiguration Complete from UE
func (du *DU) HandleRrcReconfigurationComplete(rrcBytes []byte) error {
	du.Info("[TARGET DU] Received RRC Reconfiguration Complete from UE")
	du.Info("  - Handover execution successful!")
	du.Info("  - UE is now active on target cell")

	// Forward to CU-CP via F1AP UL RRC Message Transfer
	return du.sendULRRCMessageTransfer(rrcBytes)
}

// SimulateRachReception simulates receiving RACH from handover UE (for testing)
func (du *DU) SimulateRachReception() error {
	du.Info("[TARGET DU] RACH procedure start")

	preambleId := 63

	if err := du.HandleRandomAccessPreamble(preambleId, DU_UE_F1AP_ID); err != nil {
		return fmt.Errorf("handle RACH preamble failed: %w", err)
	}

	du.Info("[TARGET DU] RACH procedure completed")
	return nil
}