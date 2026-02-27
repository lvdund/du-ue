package du

import (
	"encoding/binary"
	"fmt"

	f1ap "github.com/JocelynWS/f1-gen"
	"github.com/JocelynWS/f1-gen/ies"
	"github.com/lvdund/ngap/aper"
)

// HandleUeContextModificationRequest handles UE Context Modification Request (contains RRC Reconfiguration for handover)
func (du *DU) HandleUeContextModificationRequest(f1apPdu *f1ap.F1apPdu) error {
	du.Info("Handling UE Context Modification Request")

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

	// Find UE Context
	ctx := du.ueMgr.GetContextByDuId(msg.GNBDUUEF1APID)
	if ctx == nil {
		du.Error("UE context not found (DU-UE-ID=%d)", msg.GNBDUUEF1APID)

		var nrcgi *ies.NRCGI
		if msg.SpCellID != nil {
			nrcgi = msg.SpCellID
		} else {
			nrcgi = &ies.NRCGI{
				PLMNIdentity: []byte{0x00, 0x00, 0x00},
				NRCellIdentity: aper.BitString{
					Bytes:   []byte{0x00, 0x00, 0x00, 0x00, 0x00},
					NumBits: 36,
				},
			}
		}

		_ = du.sendUeContextModificationFailure(msg.GNBCUUEF1APID, msg.GNBDUUEF1APID, ies.CauseRadioNetworkUnknownoralreadyallocatedgnbduuef1Apid, nrcgi)
		return fmt.Errorf("UE context not found")
	}

	// -------------------------------------------------------------------------
	// [WORKAROUND] Handling misclassified "Mandatory" Release 16/17 IEs
	// The f1-gen library marks these as Mandatory, but 3GPP F1AP TS 38.473 states
	// they are strictly Optional. We extract and safely log them if forced to exist.
	// -------------------------------------------------------------------------

	// PC5 Link AMBR (Vehicular Sidelink Bit Rate)
	if msg.PC5LinkAMBR != nil && *msg.PC5LinkAMBR > 0 {
		du.Info("[UE %d] Received PC5LinkAMBR (Sidelink V2X): %d bps (Ignored by simulator)", ctx.DuUeF1apId, msg.PC5LinkAMBR)
	} else {
		du.Info("[UE %d] PC5LinkAMBR value is 0 or absent (Ignored)", ctx.DuUeF1apId)
	}

	// Conditional Intra-DU Mobility Information (Conditional Handover)
	if msg.ConditionalIntraDUMobilityInformation != nil {
		du.Info("[UE %d] Received ConditionalIntraDUMobilityInformation (Conditional Handover) - (Ignored by simulator)", ctx.DuUeF1apId)
	} else {
		du.Info("[UE %d] No ConditionalIntraDUMobilityInformation received", ctx.DuUeF1apId)
	}

	// Execute Duplication (URLLC Packet Duplication)
	if msg.ExecuteDuplication != nil {
		du.Info("[UE %d] Received ExecuteDuplication (URLLC): %d (Ignored by simulator)", ctx.DuUeF1apId, msg.ExecuteDuplication.Value)
	} else {
		du.Info("[UE %d] No ExecuteDuplication received", ctx.DuUeF1apId)
	}
	// -------------------------------------------------------------------------

	// 1. Handle PDU Session Resource Setup (DRBsToBeSetupModList)
	// We use DRB ID as a proxy for PDU Session ID due to library limitations.
	var drbsSetupList []ies.DRBsSetupModItem

	if len(msg.DRBsToBeSetupModList) > 0 {
		du.Info("[UE %d] Processing %d DRBs to be setup", ctx.DuUeF1apId, len(msg.DRBsToBeSetupModList))

		for _, item := range msg.DRBsToBeSetupModList {
			drbId := item.DRBID
			du.Info("[UE %d] Setting up DRB ID %d", ctx.DuUeF1apId, drbId)

			// Extract UL TEID and UPF IP from ULUPTNLInformation
			var ulTeid uint32
			var upfIp string

			if len(item.ULUPTNLInformationToBeSetupList) > 0 {
				ulInfo := item.ULUPTNLInformationToBeSetupList[0].ULUPTNLInformation
				if ulInfo.Choice == ies.UPTransportLayerInformationPresentGTPTunnel && ulInfo.GTPTunnel != nil {
					// Parse TEID (4 bytes)
					if len(ulInfo.GTPTunnel.GTPTEID) >= 4 {
						ulTeid = binary.BigEndian.Uint32(ulInfo.GTPTunnel.GTPTEID)
					}
					// Parse UPF IP (BitString)
					// TODO: Add proper BitString to IP string conversion if needed for debugging
					// For now, we assume standard byte layout
					if len(ulInfo.GTPTunnel.TransportLayerAddress.Bytes) >= 4 {
						// Simple debug string for now
						upfIp = fmt.Sprintf("%x", ulInfo.GTPTunnel.TransportLayerAddress.Bytes)
					}
				}
			}

			// Extract RLC Mode from the DRBsToBeSetupModItem
			rlcModeVal := int64(item.RLCMode.Value)
			du.Info("[UE %d] DRB %d requested RLC Mode: %d", ctx.DuUeF1apId, drbId, rlcModeVal)

			// Extract QoS / 5QI
			// [WORKAROUND] Problem: f1-gen ASN.1 compiler missed the 5G Standalone QoS choice-extension
			// which contains the true 5G QoSFlowLevelQoSParameters.
			// Workaround: We extract the 4G LTE E-UTRAN QCI value instead, which CUs (like OAI)
			// populate for backward compatibility. 4G QCIs map 1:1 to 5G 5QIs for basic profiles.
			var fiveQi int64 = 9 // Default best-effort
			if item.QoSInformation.EUTRANQoS != nil {
				fiveQi = item.QoSInformation.EUTRANQoS.QCI
				du.Info("[UE %d] DRB %d QoS Information - 5QI/QCI: %d", ctx.DuUeF1apId, drbId, fiveQi)
			} else {
				du.Info("[UE %d] DRB %d QoS Information - EUTRANQoS missing, using default 5QI: %d", ctx.DuUeF1apId, drbId, fiveQi)
			}

			// Allocate Local DL TEID
			dlTeid, err := du.resourceMgr.AllocateTEID()
			if err != nil {
				du.Error("Failed to allocate TEID for DRB %d: %v", drbId, err)
				continue // Skip this DRB
			}

			// Deriving S-NSSAI (SST/SD) from 5QI as a mock mechanism because f1-gen lacks DRBInformation
			sst := "1"     // Default eMBB
			sd := "000000" // Default SD
			if fiveQi >= 82 && fiveQi <= 85 {
				sst = "2" // URLLC
			} else if fiveQi >= 10 && fiveQi <= 19 {
				sst = "3" // mMTC
			}
			du.Info("[UE %d] Mock S-NSSAI derived from 5QI %d -> SST: %s, SD: %s", ctx.DuUeF1apId, fiveQi, sst, sd)

			// Create PDU Session
			// We use the dynamically extracted 5QI and RLC Mode
			pduSession := NewGnbPDUSession(
				drbId, // Use DRB ID as PDU Session ID
				upfIp,
				sst, sd, // Mock S-NSSAI based on 5QI
				0,          // IPv4 default
				0,          // QoS ID
				0,          // ARP
				fiveQi,     // 5QI dynamically parsed
				rlcModeVal, // RLC Mode dynamically parsed
				ulTeid,
				dlTeid,
			)
			// State: RESERVED (Waiting for UE confirmation)
			pduSession.State = PduSessionStateReserved

			// Store in UE Context
			if ctx.PduSessions == nil {
				ctx.PduSessions = make(map[int64]*GnbPDUSession)
			}
			ctx.PduSessions[drbId] = pduSession
			du.Info("[UE %d] Created PDU Session (ID=%d) - UL TEID: %d, DL TEID: %d", ctx.DuUeF1apId, drbId, ulTeid, dlTeid)

			// Prepare Response Item
			// Convert allocated DL TEID to bytes
			gtpTeid := make([]byte, 4)
			binary.BigEndian.PutUint32(gtpTeid, dlTeid)

			// Get Local DU IP (from config or utils)
			// Assuming du.Config.LocalAddr is available. If not, hardcode 127.0.0.1 for now
			// We use the helper IPToBitString
			duIpStr := "127.0.0.1"
			if du.Config != nil {
				duIpStr = du.Config.LocalAddr
			}
			transportLayerAddress, err := IPToBitString(duIpStr)
			if err != nil {
				du.Error("Failed to convert DU IP %s: %v", duIpStr, err)
				transportLayerAddress = aper.BitString{Bytes: []byte{127, 0, 0, 1}, NumBits: 32}
			}

			dlUpTnlInfo := ies.DLUPTNLInformationToBeSetupItem{
				DLUPTNLInformation: ies.UPTransportLayerInformation{
					Choice: ies.UPTransportLayerInformationPresentGTPTunnel,
					GTPTunnel: &ies.GTPTunnel{
						TransportLayerAddress: transportLayerAddress,
						GTPTEID:               gtpTeid,
					},
				},
			}

			drbsSetupItem := ies.DRBsSetupModItem{
				DRBID:                           drbId,
				DLUPTNLInformationToBeSetupList: []ies.DLUPTNLInformationToBeSetupItem{dlUpTnlInfo},
			}
			drbsSetupList = append(drbsSetupList, drbsSetupItem)
		}
	}

	// 2. Check if this is handover-related (contains RRC Reconfiguration)
	if len(msg.RRCContainer) > 0 {
		du.Info("[UE %d] Contains RRC Reconfiguration (Handover/PDU), forwarding to UE", ctx.DuUeF1apId)
		// Forward RRC Reconfiguration to UE
		if ctx.UeChannel != nil && ctx.UeChannel.SendToUeChannel != nil {
			ctx.UeChannel.SendToUeChannel <- msg.RRCContainer
		} else {
			return fmt.Errorf("UE channel not initialized")
		}

		// Stop scheduling UE on source cell logic removed for now as it's cleaner to handle in specific handover flows
		// if needed.
	} else {
		// Fallback: If no RRC Container, we wait for DLRRCMessageTransfer (as per robust flow)
		du.Info("[UE %d] No RRC Container in Modification Request. Waiting for DL RRC Transfer.", ctx.DuUeF1apId)
	}

	// Send UE Context Modification Response
	// We verify the resource reservation here
	return du.sendUeContextModificationResponse(msg.GNBCUUEF1APID, msg.GNBDUUEF1APID, drbsSetupList)
}

// sendUeContextModificationFailure sends UE Context Modification Failure response
func (du *DU) sendUeContextModificationFailure(cuUeId int64, duUeId int64, causeValue aper.Enumerated, nrcgi *ies.NRCGI) error {
	du.Error("[DU] Sending UE Context Modification Failure (CU-UE-ID=%d, DU-UE-ID=%d)", cuUeId, duUeId)

	cause := ies.Cause{
		Choice: ies.CausePresentRadioNetwork,
		RadioNetwork: &ies.CauseRadioNetwork{
			Value: causeValue,
		},
	}

	msg := &ies.UEContextModificationFailure{
		GNBCUUEF1APID:               cuUeId,
		GNBDUUEF1APID:               duUeId,
		Cause:                       cause,
		RequestedTargetCellGlobalID: nrcgi,
	}

	outBytes, err := f1ap.F1apEncode(msg)
	if err != nil {
		return fmt.Errorf("encode UE Context Modification Failure: %w", err)
	}

	if du.f1Client != nil {
		return du.f1Client.Send(outBytes)
	}

	du.Info("F1 client not available, skipping send (test mode)")
	return nil
}

// sendUeContextModificationResponse sends response back to CU-CP
func (du *DU) sendUeContextModificationResponse(cuUeId, duUeId int64, drbsSetupList []ies.DRBsSetupModItem) error {
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

	// Add DRBs Setup Mod List if available
	if len(drbsSetupList) > 0 {
		msg.DRBsSetupModList = drbsSetupList
	}

	// WORKAROUND: Use manual IE construction and encoding to bypass f1-gen strictness
	// 1. Procedure Code must be 4 (UEContextModification), not 5 (Confirmation/Required)
	// 2. Mandatory fields like DRBsModifiedList must be omitted if empty
	iesList := BuildUEContextModificationResponseIEs(msg)
	f1apBytes, err := EncodeF1APPdu(ies.ProcedureCode_UEContextModification, ies.Criticality_PresentReject, iesList)
	if err != nil {
		return fmt.Errorf("encode UE Context Modification Response (manual): %w", err)
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

	// Find UE Context
	ctx := du.ueMgr.GetContextByDuId(msg.GNBDUUEF1APID)
	if ctx != nil {
		du.Info("[UE %d] State Transition: %s -> %s", ctx.DuUeF1apId, ctx.GetState(), UE_STATE_IDLE)
		ctx.SetState(UE_STATE_IDLE)

		// Release UE context and resources
		du.Info("[UE %d] Releasing UE context and resources", ctx.DuUeF1apId)
		// TODO: Actual resource release logic
		du.ueMgr.RemoveContext(ctx.DuUeF1apId)
	}

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
func (du *DU) sendMeasurementReport(ctx *DuUeContext, measurementReport []byte) error {
	du.Info("[UE %d] Forwarding Measurement Report to CU-CP", ctx.DuUeF1apId)
	return du.sendULRRCMessageTransfer(measurementReport, ctx.CuUeF1apId, ctx.DuUeF1apId, 1)
}

// sendUeContextModificationRequired sends UE Context Modification Required to CU-CP (Handover Trigger)
func (du *DU) sendUeContextModificationRequired(ctx *DuUeContext, targetPci int64) error {
	du.Info("[UE %d] Sending UE Context Modification Required (Target PCI %d)", ctx.DuUeF1apId, targetPci)

	if ctx.HoCtx == nil {
		ctx.InitHandoverContext()
	}

	// GNBCUUEF1APID and GNBDUUEF1APID must be valid
	ctx.HoCtx.mutex.RLock()
	cuUeF1apId := ctx.CuUeF1apId
	duUeF1apId := ctx.DuUeF1apId
	ctx.HoCtx.mutex.RUnlock()

	// Create UE Context Modification Required message
	msg := &ies.UEContextModificationRequired{
		GNBCUUEF1APID: cuUeF1apId,
		GNBDUUEF1APID: duUeF1apId,
	}

	// 1. Mandatory RRC Container (CellGroupConfig)
	msg.DUtoCURRCInformation = &ies.DUtoCURRCInformation{
		CellGroupConfig: []byte{},
	}

	// 2. Mandatory Empty Lists (Satisfying F1AP requirements)
	msg.DRBsRequiredToBeModifiedList = []ies.DRBsRequiredToBeModifiedItem{{}}
	msg.SRBsRequiredToBeReleasedList = []ies.SRBsRequiredToBeReleasedItem{{}}
	msg.DRBsRequiredToBeReleasedList = []ies.DRBsRequiredToBeReleasedItem{{}}
	msg.BHChannelsRequiredToBeReleasedList = []ies.BHChannelsRequiredToBeReleasedItem{{}}
	msg.SLDRBsRequiredToBeModifiedList = []ies.SLDRBsRequiredToBeModifiedItem{{}}
	msg.SLDRBsRequiredToBeReleasedList = []ies.SLDRBsRequiredToBeReleasedItem{{}}
	msg.TargetCellsToCancel = []ies.TargetCellListItem{{}}

	// 3. Other Mandatory Fields
	msg.Cause = ies.Cause{}

	// Note: CandidateSpCellList is currently missing in the generated library, skipping.

	// Encode
	f1apBytes, err := f1ap.F1apEncode(msg)
	if err != nil {
		du.Warn("Failed to encode UE Context Modification Required (simulated): %v", err)
		return nil
	}

	// Send
	if du.f1Client != nil {
		return du.f1Client.Send(f1apBytes)
	}

	du.Info("F1 client not available, skipping send (test mode)")
	return nil
}

// HandleUeContextModificationConfirm handles UE Context Modification Confirm (Source DU side)
// This message is sent by the CU in response to UEContextModificationRequired (Handover Trigger)
// It typically contains the Target->Source->UE RRC Reconfiguration (Handover Command).
func (du *DU) HandleUeContextModificationConfirm(f1apPdu *f1ap.F1apPdu) error {
	du.Info("Handling UE Context Modification Confirm")

	if f1apPdu.Present != ies.F1apPduSuccessfulOutcome {
		du.Error("Invalid F1AP PDU present type (expected SuccessfulOutcome)")
		return fmt.Errorf("invalid PDU type")
	}
	msg, ok := f1apPdu.Message.Msg.(*ies.UEContextModificationConfirm)
	if !ok {
		du.Error("Failed to cast message to UEContextModificationConfirm")
		return fmt.Errorf("invalid message type")
	}

	du.Info("UE Context Modification Confirm: CU-UE-ID=%d, DU-UE-ID=%d",
		msg.GNBCUUEF1APID, msg.GNBDUUEF1APID)

	// Find UE Context
	ctx := du.ueMgr.GetContextByDuId(msg.GNBDUUEF1APID)
	if ctx == nil {
		du.Error("UE context not found (DU-UE-ID=%d)", msg.GNBDUUEF1APID)
		return fmt.Errorf("UE context not found")
	}

	// Check for RRC Container (Handover Command)
	if len(msg.RRCContainer) > 0 {
		du.Info("[UE %d] Received RRC Container (Handover Command), forwarding to UE", ctx.DuUeF1apId)

		if ctx.UeChannel != nil && ctx.UeChannel.SendToUeChannel != nil {
			ctx.UeChannel.SendToUeChannel <- msg.RRCContainer
			du.Info("[UE %d] Handover Command forwarded to UE", ctx.DuUeF1apId)
		} else {
			du.Error("[UE %d] UE channel not available", ctx.DuUeF1apId)
			return fmt.Errorf("ue channel missing")
		}

		// Update State -> EXECUTION
		du.SetSourceHandoverState(ctx, HO_STATE_EXECUTION)
	} else {
		du.Warn("[UE %d] UE Context Modification Confirm received without RRC Container", ctx.DuUeF1apId)
	}

	return nil
}
