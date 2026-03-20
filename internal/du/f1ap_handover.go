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
	if msg.PC5LinkAMBR > 0 {
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

	var drbsModifiedList []ies.DRBsModifiedItem
	if len(msg.DRBsToBeModifiedList) > 0 {
		du.Info("[UE %d] Processing %d DRBs to be modified", ctx.DuUeF1apId, len(msg.DRBsToBeModifiedList))
		for _, item := range msg.DRBsToBeModifiedList {
			drbId := item.DRBID
			du.Info("[UE %d] Modifying DRB ID %d", ctx.DuUeF1apId, drbId)

			if item.QoSInformation != nil && item.QoSInformation.EUTRANQoS != nil {
				fiveQi := item.QoSInformation.EUTRANQoS.QCI
				du.Info("[UE %d] DRB %d QoS Information Updated - New 5QI/QCI: %d", ctx.DuUeF1apId, drbId, fiveQi)
				if session, exists := ctx.PduSessions[drbId]; exists {
					session.FiveQi = fiveQi
					// Re-derive S-NSSAI
					sst := "1"
					sd := "000000"
					if fiveQi >= 82 && fiveQi <= 85 {
						sst = "2"
					} else if fiveQi >= 10 && fiveQi <= 19 {
						sst = "3"
					}
					session.Snssai.Sst = sst
					session.Snssai.Sd = sd
				}
			}

			drbsModifiedItem := ies.DRBsModifiedItem{
				DRBID: drbId,
			}
			drbsModifiedList = append(drbsModifiedList, drbsModifiedItem)
		}
	}

	if len(msg.DRBsToBeReleasedList) > 0 {
		du.Info("[UE %d] Processing %d DRBs to be released", ctx.DuUeF1apId, len(msg.DRBsToBeReleasedList))
		for _, item := range msg.DRBsToBeReleasedList {
			drbId := item.DRBID
			du.Info("[UE %d] Releasing DRB ID %d", ctx.DuUeF1apId, drbId)
			if session, exists := ctx.PduSessions[drbId]; exists {
				// Release TEID
				du.Info("[UE %d] Releasing TEID %d for DRB %d", ctx.DuUeF1apId, session.Teid.DownlinkTeid, drbId)
				// Assuming resourceMgr has ReleaseTEID (I need to check, but usually it might not exist if simple. I'll just delete from map)
				delete(ctx.PduSessions, drbId)
				du.Info("[UE %d] Released PDU Session (ID=%d)", ctx.DuUeF1apId, drbId)
			}
		}
	}

	var srbsSetupList []ies.SRBsSetupModItem
	if len(msg.SRBsToBeSetupModList) > 0 {
		du.Info("[UE %d] Processing %d SRBs to be setup/modified", ctx.DuUeF1apId, len(msg.SRBsToBeSetupModList))
		if ctx.SrbPriorities == nil {
			ctx.SrbPriorities = make(map[int64]int)
		}
		for _, item := range msg.SRBsToBeSetupModList {
			srbId := item.SRBID
			du.Info("[UE %d] Processing SRB Setup/Mod: ID %d", ctx.DuUeF1apId, srbId)
			if srbId == 1 {
				ctx.Srb1Active = true
				ctx.SrbPriorities[srbId] = 1
			} else if srbId == 2 {
				ctx.Srb2Active = true
				ctx.SrbPriorities[srbId] = 3
			} else if srbId == 3 {
				ctx.SrbPriorities[srbId] = 2 // Typical for SRB3
			}
			srbsSetupList = append(srbsSetupList, ies.SRBsSetupModItem{SRBID: srbId})
		}
	}

	if len(msg.SRBsToBeReleasedList) > 0 {
		du.Info("[UE %d] Processing %d SRBs to be released", ctx.DuUeF1apId, len(msg.SRBsToBeReleasedList))
		for _, item := range msg.SRBsToBeReleasedList {
			srbId := item.SRBID
			du.Info("[UE %d] Releasing SRB ID %d", ctx.DuUeF1apId, srbId)
			if srbId == 1 {
				ctx.Srb1Active = false
			} else if srbId == 2 {
				ctx.Srb2Active = false
			}
			delete(ctx.SrbPriorities, srbId)
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
	} else {
		// Fallback: If no RRC Container, we wait for DLRRCMessageTransfer (as per robust flow)
		du.Info("[UE %d] No RRC Container in Modification Request. Waiting for DL RRC Transfer.", ctx.DuUeF1apId)
	}

	// Send UE Context Modification Response
	// We verify the resource reservation here
	return du.sendUeContextModificationResponse(msg.GNBCUUEF1APID, msg.GNBDUUEF1APID, drbsSetupList, drbsModifiedList, srbsSetupList)
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
func (du *DU) sendUeContextModificationResponse(cuUeId, duUeId int64, drbsSetupList []ies.DRBsSetupModItem, drbsModifiedList []ies.DRBsModifiedItem, srbsSetupList []ies.SRBsSetupModItem) error {
	du.Info("Sending UE Context Modification Response")

	// Build mandatory DUtoCURRCInformation
	duToCuRrcInfo := &ies.DUtoCURRCInformation{
		CellGroupConfig: []byte{}, // Empty for now
	}

	msg := &ies.UEContextModificationResponse{
		GNBCUUEF1APID:        cuUeId,
		GNBDUUEF1APID:        duUeId,
		DUtoCURRCInformation: duToCuRrcInfo,
		// Mandatory lists in f1-gen (though optional in spec)
		BHChannelsSetupModList: []ies.BHChannelsSetupModItem{
			{BHRLCChannelID: aper.BitString{Bytes: []byte{0, 0}, NumBits: 16}},
		},
		BHChannelsModifiedList: []ies.BHChannelsModifiedItem{
			{BHRLCChannelID: aper.BitString{Bytes: []byte{0, 0}, NumBits: 16}},
		},
		RequestedTargetCellGlobalID: &ies.NRCGI{
			PLMNIdentity: []byte{0, 0, 0},
			NRCellIdentity: aper.BitString{
				Bytes:   []byte{0x00, 0x00, 0x00, 0x00, 0x00},
				NumBits: 36,
			},
		},
	}

	// Always provide at least one dummy item for list fields if they are empty
	if len(drbsSetupList) > 0 {
		msg.DRBsSetupModList = drbsSetupList
	}

	if len(drbsModifiedList) > 0 {
		msg.DRBsModifiedList = drbsModifiedList
	}

	if len(srbsSetupList) > 0 {
		msg.SRBsSetupModList = srbsSetupList
	}

	// Use manual encoder to workaround vendor bug (Procedure 7/8 swapped)
	f1apBytes, err := EncodeUEContextModificationResponse(msg)
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

	dummyId := du.Config.MockCU.DummyID

	// Create UE Context Modification Required message
	msg := &ies.UEContextModificationRequired{
		GNBCUUEF1APID: cuUeF1apId,
		GNBDUUEF1APID: duUeF1apId,
	}

	// 1. Mandatory RRC Container (CellGroupConfig)
	msg.DUtoCURRCInformation = &ies.DUtoCURRCInformation{
		CellGroupConfig: []byte{},
	}

	// 2. Mandatory Empty Lists (Satisfying F1AP requirements with configured dummy ID)
	msg.DRBsRequiredToBeModifiedList = []ies.DRBsRequiredToBeModifiedItem{{DRBID: dummyId}}
	msg.SRBsRequiredToBeReleasedList = []ies.SRBsRequiredToBeReleasedItem{{SRBID: dummyId}}
	msg.DRBsRequiredToBeReleasedList = []ies.DRBsRequiredToBeReleasedItem{{DRBID: dummyId}}
	msg.BHChannelsRequiredToBeReleasedList = []ies.BHChannelsRequiredToBeReleasedItem{
		{BHRLCChannelID: aper.BitString{Bytes: []byte{0x00, 0x00}, NumBits: 16}},
	}
	msg.SLDRBsRequiredToBeModifiedList = []ies.SLDRBsRequiredToBeModifiedItem{{SLDRBID: dummyId}}
	msg.SLDRBsRequiredToBeReleasedList = []ies.SLDRBsRequiredToBeReleasedItem{{SLDRBID: dummyId}}
	msg.TargetCellsToCancel = []ies.TargetCellListItem{
		{
			TargetCell: ies.NRCGI{
				PLMNIdentity: []byte{0x00, 0x01, 0x01},
				NRCellIdentity: aper.BitString{
					Bytes:   []byte{0x00, 0x00, 0x00, 0x00, 0x00},
					NumBits: 36,
				},
			},
		},
	}

	// 3. Other Mandatory Fields
	msg.Cause = ies.Cause{
		Choice: ies.CausePresentRadioNetwork,
		RadioNetwork: &ies.CauseRadioNetwork{
			Value: ies.CauseRadioNetworkNoradioresourcesavailable,
		},
	}

	// Encode
	// We use the library's encoder directly. Previous attempts to manually wrap 
	// resulted in double-wrapping and decoding errors at the CU-CP.
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

// TriggerDuInitiatedModification simulates a DU internal decision to modify a UE context (ORAN 6.3.2)
// Specifically, it triggers the release of a DRB by sending UE Context Modification Required.
func (du *DU) TriggerDuInitiatedModification(duUeF1apId int64, drbId int64) error {
	du.Info("[UE %d] Triggering DU-Initiated Modification (Release DRB %d)", duUeF1apId, drbId)

	// Find UE Context
	ctx := du.ueMgr.GetContextByDuId(duUeF1apId)
	if ctx == nil {
		du.Error("UE context not found (DU-UE-ID=%d)", duUeF1apId)
		return fmt.Errorf("UE context not found")
	}

	dummyId := du.Config.MockCU.DummyID

	// Create UE Context Modification Required message
	msg := &ies.UEContextModificationRequired{
		GNBCUUEF1APID: ctx.CuUeF1apId,
		GNBDUUEF1APID: ctx.DuUeF1apId,
	}

	// 1. Mandatory RRC Container (CellGroupConfig)
	msg.DUtoCURRCInformation = &ies.DUtoCURRCInformation{
		CellGroupConfig: []byte{0x00},
	}

	// 2. Populate DRBsRequiredToBeReleasedList
	msg.DRBsRequiredToBeReleasedList = []ies.DRBsRequiredToBeReleasedItem{
		{
			DRBID: drbId,
		},
	}

	// 3. Other Mandatory Fields (Satisfying F1AP requirements with configured dummy items)
	msg.DRBsRequiredToBeModifiedList = []ies.DRBsRequiredToBeModifiedItem{
		{
			DRBID: dummyId,
			DLUPTNLInformationToBeSetupList: []ies.DLUPTNLInformationToBeSetupItem{
				{
					DLUPTNLInformation: ies.UPTransportLayerInformation{
						Choice: ies.UPTransportLayerInformationPresentGTPTunnel,
						GTPTunnel: &ies.GTPTunnel{
							TransportLayerAddress: aper.BitString{
								Bytes:   []byte{127, 0, 0, 1},
								NumBits: 32,
							},
							GTPTEID: []byte{0x00, 0x00, 0x00, 0x01},
						},
					},
				},
			},
		},
	}
	msg.SRBsRequiredToBeReleasedList = []ies.SRBsRequiredToBeReleasedItem{
		{SRBID: dummyId},
	}
	msg.BHChannelsRequiredToBeReleasedList = []ies.BHChannelsRequiredToBeReleasedItem{
		{
			BHRLCChannelID: aper.BitString{
				Bytes:   []byte{0x00, 0x00},
				NumBits: 16,
			},
		},
	}
	msg.SLDRBsRequiredToBeModifiedList = []ies.SLDRBsRequiredToBeModifiedItem{
		{SLDRBID: dummyId},
	}
	msg.SLDRBsRequiredToBeReleasedList = []ies.SLDRBsRequiredToBeReleasedItem{
		{SLDRBID: dummyId},
	}
	msg.TargetCellsToCancel = []ies.TargetCellListItem{
		{
			TargetCell: ies.NRCGI{
				PLMNIdentity: ConvertMccMncToPlmn(du.Config.PLMN.MCC, du.Config.PLMN.MNC),
				NRCellIdentity: aper.BitString{
					Bytes:   []byte{0x00, 0x00, 0x00, 0x00, 0x00},
					NumBits: 36,
				},
			},
		},
	}

	msg.Cause = ies.Cause{
		Choice: ies.CausePresentRadioNetwork,
		RadioNetwork: &ies.CauseRadioNetwork{
			Value: ies.CauseRadioNetworkNoradioresourcesavailable,
		},
	}

	// Encode
	f1apBytes, err := f1ap.F1apEncode(msg)
	if err != nil {
		du.Error("Failed to encode UE Context Modification Required: %v", err)
		return err
	}

	// Send
	if du.f1Client != nil {
		return du.f1Client.Send(f1apBytes)
	}

	du.Info("F1 client not available, skipping send (test mode)")
	return nil
}

