package du

import (
	"encoding/binary"
	"fmt"
	"net"

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
	// Heuristic: If GNBDUUEF1APID is absent, it's a new UE setup (Handover at Target DU)
	// If it's present, it's a following setup for an existing UE (Normal procedure)
	if msg.GNBDUUEF1APID == nil {
		du.Info("[TARGET DU] Handling UE Context Setup Request (Handover/New)")
		return du.handleTargetHandoverSetup(msg)
	}

	// Normal UE Context Setup
	du.Info("Handling UE Context Setup Request")

	// Find or Allocate Context
	var ctx *DuUeContext
	if msg.GNBDUUEF1APID != nil {
		ctx = du.ueMgr.GetContextByDuId(*msg.GNBDUUEF1APID)
	}

	// VALIDATION: SpCellID (NRCGI)
	// Ensure the UE is attaching to *this* DU's cell
	// Extract PLMN and CellID from SpCellID
	plmnBytes := msg.SpCellID.PLMNIdentity
	cellIdBytes := msg.SpCellID.NRCellIdentity.Bytes
	// Reconstruct CellID (assuming 36 bits, so mostly checking the bytes)
	// For now, we do a basic bytes check against our config
	// NOTE: This is a simplified check. A full check would parse bits.
	// We just log it for now as "Validated".
	du.Info("[UE Setup] Validating SpCellID: PLMN %x, CellID %x", plmnBytes, cellIdBytes)

	// In a real scenario, we would return Failure if this doesn't match du.Config.
	if len(plmnBytes) == 3 && len(cellIdBytes) > 0 {
		du.Info("[UE Setup] SpCellID format valid.")
	} else {
		du.Error("[UE Setup] SpCellID format invalid (PLMN len=%d, CellID len=%d). Rejecting.", len(plmnBytes), len(cellIdBytes))

		// Attempt to construct the RequestedTargetCellGlobalID for the failure message
		var nrcgi *ies.NRCGI
		if msg.SpCellID.PLMNIdentity != nil {
			nrcgi = &msg.SpCellID
		}

		_ = du.sendUeContextSetupFailure(msg.GNBCUUEF1APID, nil, ies.CauseRadioNetworkCellnotavailable, nrcgi)
		return fmt.Errorf("spcellid validation failed")
	}

	if ctx == nil {
		// New UE context
		duUeId := du.allocateDuUeF1apId()
		// In a real scenario, we'd also have the CRNTI from RACH
		// For now, let's assume we create a NEW context if not found

		// Allocate C-RNTI
		crnti, err := du.resourceMgr.AllocateCRNTI()
		if err != nil {
			du.Error("Failed to allocate C-RNTI: %v", err)

			_ = du.sendUeContextSetupFailure(msg.GNBCUUEF1APID, &duUeId, ies.CauseRadioNetworkNoradioresourcesavailable, &msg.SpCellID)
			return err
		}

		if err := du.InitUE(duUeId, msg.GNBCUUEF1APID, crnti); err != nil {
			return err
		}
		ctx = du.ueMgr.GetContextByDuId(duUeId)
	} else {
		// Update existing context with CU ID
		ctx.CuUeF1apId = msg.GNBCUUEF1APID
	}

	// STORE: ServCellIndex
	ctx.ServCellIndex = msg.ServCellIndex
	du.Info("[UE %d] Stored ServCellIndex: %d", ctx.DuUeF1apId, ctx.ServCellIndex)

	// STORE: SpCellID (String representation)
	ctx.SpCellID = fmt.Sprintf("PLMN:%x-CellID:%x", plmnBytes, cellIdBytes)

	// PROCESS: SRBsToBeSetupList
	if len(msg.SRBsToBeSetupList) > 0 {
		if ctx.SrbPriorities == nil {
			ctx.SrbPriorities = make(map[int64]int)
		}
		for _, srb := range msg.SRBsToBeSetupList {
			srbId := srb.SRBID
			du.Info("[UE %d] Processing SRB Setup: ID %d", ctx.DuUeF1apId, srbId)
			if srbId == 1 {
				ctx.Srb1Active = true
				ctx.SrbPriorities[srbId] = 1 // Highest Priority
				du.Info("[UE %d] SRB1 Activated (Scheduling Priority: 1)", ctx.DuUeF1apId)
			} else if srbId == 2 {
				ctx.Srb2Active = true
				// Note: Priority 2 is typically reserved for SRB3 in NSA/Dual Connectivity.
				// SRB2 (NAS traffic) receives Priority 3 as per standard 3GPP logical channel prioritization.
				ctx.SrbPriorities[srbId] = 3 // Standard Priority
				du.Info("[UE %d] SRB2 Activated (Scheduling Priority: 3)", ctx.DuUeF1apId)
			}
		}
	} else {
		// If list is empty/missing (optional), we might assume defaults or wait for Modification.
		// For Initial Setup, SRB1 is often implicit or explicit.
		du.Info("[UE %d] No SRBsToBeSetupList provided.", ctx.DuUeF1apId)
	}

	// PROCESS: Must-Have Fields
	if msg.CUtoDURRCInformation != nil {
		du.Info("[UE %d] Received CUtoDURRCInformation", ctx.DuUeF1apId)
	}

	// PROCESS: DRBsToBeSetupList
	var setupDrbs []int64
	if len(msg.DRBsToBeSetupList) > 0 {
		du.Info("[UE %d] Processing DRBsToBeSetupList (Count: %d)", ctx.DuUeF1apId, len(msg.DRBsToBeSetupList))
		for _, drb := range msg.DRBsToBeSetupList {
			drbId := drb.DRBID
			du.Info("[UE %d] Processing DRB Setup: ID %d", ctx.DuUeF1apId, drbId)

			// Extract RLC Mode
			rlcModeVal := int64(drb.RLCMode.Value)
			du.Info("[UE %d] DRB %d requested RLC Mode: %d", ctx.DuUeF1apId, drbId, rlcModeVal)

			// Extract QoS / 5QI
			// [WORKAROUND] Problem: f1-gen ASN.1 compiler missed the 5G Standalone QoS choice-extension
			// which contains the true 5G QoSFlowLevelQoSParameters.
			// Workaround: We extract the 4G LTE E-UTRAN QCI value instead, which CUs (like OAI)
			// populate for backward compatibility. 4G QCIs map 1:1 to 5G 5QIs for basic profiles.
			var fiveQi int64 = 9 // Default best-effort
			if drb.QoSInformation.EUTRANQoS != nil {
				fiveQi = drb.QoSInformation.EUTRANQoS.QCI
				du.Info("[UE %d] DRB %d QoS Information - 5QI/QCI: %d", ctx.DuUeF1apId, drbId, fiveQi)
			} else {
				du.Info("[UE %d] DRB %d QoS Information - EUTRANQoS missing, using default 5QI: %d", ctx.DuUeF1apId, drbId, fiveQi)
			}

			// Validate UL UP TNL Information
			if len(drb.ULUPTNLInformationToBeSetupList) == 0 {
				du.Error("[UE %d] No UL UP TNL Info provided for DRB %d. Rejecting setup.", ctx.DuUeF1apId, drbId)
				// Real DU would add this to DRBsFailedToBeSetupList and continue.
				continue
			}

			ulInfo := drb.ULUPTNLInformationToBeSetupList[0].ULUPTNLInformation
			if ulInfo.GTPTunnel == nil {
				du.Error("[UE %d] Missing GTP Tunnel info for DRB %d.", ctx.DuUeF1apId, drbId)
				continue
			}

			upfIp := net.IP(ulInfo.GTPTunnel.TransportLayerAddress.Bytes).String()
			ulTeid := binary.BigEndian.Uint32(ulInfo.GTPTunnel.GTPTEID)

			du.Info("[UE %d] Target UPF IP: %s, UL TEID: 0x%08x", ctx.DuUeF1apId, upfIp, ulTeid)

			// Allocate local Downlink TEID
			dlTeid, err := du.resourceMgr.AllocateTEID()
			if err != nil {
				du.Error("Failed to allocate TEID for DRB %d: %v", drbId, err)
				continue
			}
			du.Info("[UE %d] Allocated DL TEID: 0x%08x for DRB %d", ctx.DuUeF1apId, dlTeid, drbId)

			// Deriving S-NSSAI (SST/SD) from 5QI as a mock mechanism because f1-gen lacks DRBInformation
			sst := "1"     // Default eMBB
			sd := "000000" // Default SD
			if fiveQi >= 82 && fiveQi <= 85 {
				sst = "2" // URLLC
			} else if fiveQi >= 10 && fiveQi <= 19 {
				sst = "3" // mMTC
			}
			du.Info("[UE %d] Mock S-NSSAI derived from 5QI %d -> SST: %s, SD: %s", ctx.DuUeF1apId, fiveQi, sst, sd)

			// Create PDU Session (RESERVED state)
			pduSession := NewGnbPDUSession(drbId, upfIp, sst, sd, 0, 0, 0, fiveQi, rlcModeVal, ulTeid, dlTeid)
			pduSession.State = PduSessionStateReserved

			if ctx.PduSessions == nil {
				ctx.PduSessions = make(map[int64]*GnbPDUSession)
			}
			ctx.PduSessions[drbId] = pduSession

			setupDrbs = append(setupDrbs, drbId)
			du.Info("[UE %d] Reserved DRB %d (UPF TEID: %d, UPF IP: %s). Local DL TEID: %d", ctx.DuUeF1apId, drbId, ulTeid, upfIp, dlTeid)
		}
	}

	// Extract RRC container if present (RRCReconfiguration)
	if len(msg.RRCContainer) > 0 {
		du.Info("[UE %d] UE Context Setup Request contains RRC container, forwarding to UE", ctx.DuUeF1apId)
		if ctx.UeChannel != nil && ctx.UeChannel.SendToUeChannel != nil {
			ctx.UeChannel.SendToUeChannel <- msg.RRCContainer
		}
	}

	return du.sendUeContextSetupResponse(msg.GNBCUUEF1APID, ctx.DuUeF1apId, setupDrbs)
}

// handleTargetHandoverSetup handles handover preparation at Target DU
func (du *DU) handleTargetHandoverSetup(msg *ies.UEContextSetupRequest) error {
	du.Info("[TARGET DU] UE Context Setup Request: CU-UE-ID=%d", msg.GNBCUUEF1APID)

	// Store UE IDs and create context
	duUeF1apId := du.allocateDuUeF1apId()
	if msg.GNBDUUEF1APID != nil {
		duUeF1apId = *msg.GNBDUUEF1APID
	}

	// Create UE context and initialize handover state
	// Allocate C-RNTI
	crnti, err := du.resourceMgr.AllocateCRNTI()
	if err != nil {
		du.Error("Failed to allocate C-RNTI: %v", err)
		return err
	}

	if err := du.InitUE(duUeF1apId, msg.GNBCUUEF1APID, crnti); err != nil {
		return err
	}
	ctx := du.ueMgr.GetContextByDuId(duUeF1apId)
	du.SetTargetHandoverState(ctx, HO_STATE_IDLE) // Initially IDLE, transitioned to PREPARATION below

	// Target cell ID is ourselves (simulated)
	ctx.HoCtx.targetCellId = int64(du.Config.Cell.PCI)
	du.SetTargetHandoverState(ctx, HO_STATE_PREPARATION)

	// Allocate resources
	du.Info("[TARGET DU] Allocating resources for handover UE")
	if err := du.allocateHandoverResources(); err != nil {
		du.Error("Failed to allocate resources: %v", err)

		return du.sendUeContextSetupFailure(msg.GNBCUUEF1APID, &duUeF1apId, ies.CauseRadioNetworkNoradioresourcesavailable, &msg.SpCellID)
	}

	// Start RACH monitoring (context-aware logic should be updated in du_rach.go)
	du.StartRachMonitoring(ctx)

	// Send response
	return du.sendUeContextSetupResponse(msg.GNBCUUEF1APID, duUeF1apId, nil)
}

// allocateHandoverResources allocates resources for handover UE
func (du *DU) allocateHandoverResources() error {
	du.Info("[TARGET DU] Allocating C-RNTI, PRBs, RACH resources")
	// TODO: Actual resource allocation
	return nil
}

// sendUeContextSetupFailure sends failure response
func (du *DU) sendUeContextSetupFailure(cuUeId int64, duUeId *int64, causeValue aper.Enumerated, nrcgi *ies.NRCGI) error {
	du.Error("[DU] Sending UE Context Setup Failure (CU-UE-ID=%d)", cuUeId)

	cause := ies.Cause{
		Choice: ies.CausePresentRadioNetwork,
		RadioNetwork: &ies.CauseRadioNetwork{
			Value: causeValue,
		},
	}

	msg := &ies.UEContextSetupFailure{
		GNBCUUEF1APID:               cuUeId,
		GNBDUUEF1APID:               duUeId,
		Cause:                       cause,
		RequestedTargetCellGlobalID: nrcgi,
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
func (du *DU) sendUeContextSetupResponse(cuUeId, duUeId int64, setupDrbs []int64) error {
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
	// A real DU generates an ASN.1 CellGroupConfig here. For simulation, provide a minimalist dummy byte payload.
	duToCuRrcInfo := ies.DUtoCURRCInformation{
		CellGroupConfig: []byte{0x00, 0x01},
	}

	// Optional C-RNTI (as pointer)
	// For now, use the context's C-RNTI if available, otherwise 0 (should not happen for Setup Response)
	var crnti int64
	ctx := du.ueMgr.GetContextByDuId(duUeId)
	if ctx != nil {
		crnti = ctx.CRnti
		du.Info("[UE %d] State Transition: %s -> %s", ctx.DuUeF1apId, ctx.GetState(), UE_STATE_CONNECTED)
		ctx.SetState(UE_STATE_CONNECTED)
	}

	// Build DRBsSetupList
	var drbsSetupList []ies.DRBsSetupItem
	for _, drbId := range setupDrbs {
		pdu, ok := ctx.PduSessions[drbId]
		if ok {
			// Convert allocated DL TEID to bytes
			gtpTeid := make([]byte, 4)
			binary.BigEndian.PutUint32(gtpTeid, pdu.Teid.DownlinkTeid)

			// Get Local DU IP
			duIpStr := "127.0.0.1"
			if du.Config != nil {
				duIpStr = du.Config.LocalAddr
			}
			transportLayerAddress, err := IPToBitString(duIpStr)
			if err != nil {
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

			drbSetupItem := ies.DRBsSetupItem{
				DRBID:                           drbId,
				DLUPTNLInformationToBeSetupList: []ies.DLUPTNLInformationToBeSetupItem{dlUpTnlInfo},
			}
			drbsSetupList = append(drbsSetupList, drbSetupItem)
		}
	}

	// Build SRBsSetupList
	var srbsSetupList []ies.SRBsSetupItem
	// Add SRB1 if active
	if ctx.Srb1Active {
		srbsSetupList = append(srbsSetupList, ies.SRBsSetupItem{
			SRBID: 1,
			LCID:  1, // Standard LCID for SRB1
		})
	}
	// Add SRB2 if active
	if ctx.Srb2Active {
		srbsSetupList = append(srbsSetupList, ies.SRBsSetupItem{
			SRBID: 2,
			LCID:  2, // Standard LCID for SRB2
		})
	}

	msg := &ies.UEContextSetupResponse{
		GNBCUUEF1APID:               cuUeId,
		GNBDUUEF1APID:               duUeId,
		DUtoCURRCInformation:        duToCuRrcInfo,
		CRNTI:                       &crnti,
		RequestedTargetCellGlobalID: nrcgi,
		SRBsSetupList:               srbsSetupList,
		DRBsSetupList:               drbsSetupList,
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

// NOTE: RACH Logic Refactoring
// The following functions were previously located here but have been moved to `internal/du/du_rach.go`
// to support a state-based RACH handling implementation:
// 1. StartRachMonitoring()
// 2. SimulateRachReception() (Replaces HandleRandomAccessPreamble)
// 3. sendRandomAccessResponse()
