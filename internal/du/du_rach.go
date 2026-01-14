package du

import (
	"fmt"
	"time"
)

// RACHContext manages the Random Access state for a specific UE
type RACHContext struct {
	preambleId int
	raRnti     int
	tempCrnti  int64
	state      string
	startTime  time.Time

	// Channels to communicate with the specific UE simulation
	ueChannel *UeChannel
}

// RACH Message Legend:
// Msg1 = Random Access Preamble (UE -> DU)
// Msg2 = Random Access Response / RAR (DU -> UE)
// Msg3 = RRC Connection Request / Reconfig Complete (UE -> DU)
// Msg4 = RRC Setup / Contention Resolution (DU -> UE)

// StartRachMonitoring starts monitoring for RACH preambles
// In a real DU, this would listen to PHY. Here, we wait for the trigger
// from the Handover logic or the Test Harness.
func (du *DU) StartRachMonitoring() {
	du.Info("[TARGET DU] Started RACH Monitoring Service")

	// 1. Validate we are the Target DU
	if !du.IsTargetDU() {
		du.Error("[TARGET DU] Cannot start RACH monitoring: not acting as Target DU")
		return
	}

	// 2. Validate State (Expect to be in PREPARATION)
	state := du.GetHandoverState()
	if state != HO_STATE_PREPARATION {
		du.Warn("[TARGET DU] StartRachMonitoring called in unexpected state: %s", handoverStateToString(state))
	}

	// 3. Transition to EXECUTION (Listening Mode)
	du.SetTargetHandoverState(HO_STATE_EXECUTION)
	du.Info("[TARGET DU] RACH Window Open - Waiting for Preamble...")

	// In this simulator, we don't polled a real PHY.
	// The "Trigger" comes from `SimulateRachReception` being called
	// by the Target Handover Flow in `ue_context_setup.go`.
}

// SimulateRachReception is the entry point when the DU "detects" a preamble
// This is called by `du.handleTargetHandoverSetup`
func (du *DU) SimulateRachReception() error {
	du.Info("[TARGET DU] PHY Layer detected Random Access Preamble!")

	// 1. Create RACH Context
	rachCtx := &RACHContext{
		preambleId: 63, // Dedicated preamble for handover
		raRnti:     100,
		tempCrnti:  int64(C_RNTI), // Using the global constant for now
		state:      "MSG1_RECEIVED",
		startTime:  time.Now(),
		ueChannel:  du.ue, // Link to the UE channel
	}

	du.Info("  - Preamble ID: %d", rachCtx.preambleId)
	du.Info("  - Calculated RA-RNTI: %d", rachCtx.raRnti)

	// 2. Send Random Access Response (RAR)
	if err := du.sendRandomAccessResponse(rachCtx); err != nil {
		return fmt.Errorf("failed to send RAR: %w", err)
	}

	// 3. Wait for RRC Connection Request (Msg3 - RRCSetupRequest / ReconfigurationComplete)
	// In our flow, the UE sends this to `uplink_downlink.go` which forwards it to CU.
	// For RRCSetup (Initial Access), the DU would normally intercept Msg3 here.
	// But since we are focusing on Handover, Msg3 is "RRCReconfigurationComplete".

	du.Info("[TARGET DU] RACH Procedure: Msg1 & Msg2 completed. Waiting for Msg3...")
	return nil
}

// sendRandomAccessResponse (Msg2 - RAR)
// In a real stack, this is a MAC PDU. Here we simulate it by sending a signal
// or by directly triggering the next step if we are shortcutting.
func (du *DU) sendRandomAccessResponse(ctx *RACHContext) error {
	du.Info("[TARGET DU] Sending Msg2 (Random Access Response)")
	du.Info("  - Timing Advance: 0")
	du.Info("  - UL Grant: Allocated")
	du.Info("  - TC-RNTI: %d", ctx.tempCrnti)

	// 1. Construct simulated RAR Payload
	// Structure (Simplified MAC PDU): [Subheader | Timing Advance | UL Grant | TC-RNTI]
	// We use a 7-byte mock payload
	rarPayload := make([]byte, 7)
	rarPayload[0] = 0x40 // E/T/R/R/BI Header (Mac Subheader)
	rarPayload[1] = 0x00 // TA Command (11 bits - 1st byte)
	rarPayload[2] = 0x00 // TA Command (11 bits - 2nd byte) + UL Grant
	rarPayload[3] = 0x00 // UL Grant
	rarPayload[4] = 0x00 // UL Grant
	// TC-RNTI (16 bits)
	rarPayload[5] = byte(ctx.tempCrnti >> 8)
	rarPayload[6] = byte(ctx.tempCrnti)

	// 2. Send to UE Channel
	if ctx.ueChannel != nil && ctx.ueChannel.SendToUeChannel != nil {
		ctx.ueChannel.SendToUeChannel <- rarPayload
		du.Info("[TARGET DU] Transmitted RAR PDU (%d bytes)", len(rarPayload))
	} else {
		du.Warn("[TARGET DU] UE Channel not available, cannot send RAR bytes")
	}

	return nil
}

// SendRRCSetup (Msg4)
// This is used for Initial Access (not Handover), but fulfills the user's request
// for "Message Creation on DU Side".
//
// Commented out as requested - not part of Handover (Changelog) scope.
/*
func (du *DU) SendRRCSetup(transactionId int64) error {
	du.Info("[DU] Constructing RRCSetup (Msg4)")

	// --- 1. Construct RRC Transaction Identifier ---
	rrcTransId := rrcies.RRC_TransactionIdentifier{
		Value: uint64(transactionId),
	}

	// --- 2. Construct Master Cell Group (The Configuration) ---
	// This is where we configure RLC/MAC/PHY. We use a minimal valid config.
	cellGroupConfig := rrcies.CellGroupConfig{
		CellGroupId: rrcies.CellGroupId{Value: 0},
		// RLC/MAC config would be added here in a full implementation
		// For now, we leave them nil (Optional) as per the "Minimal" strategy
		SpCellConfig: &rrcies.SpCellConfig{
			ReconfigurationWithSync: nil, // Not a handover
		},
	}

	// Encode the CellGroupConfig (it's an OCTET STRING inside RRCSetup)
	cellGroupBytes, err := rrc.Encode(&cellGroupConfig)
	if err != nil {
		return fmt.Errorf("failed to encode MasterCellGroup: %w", err)
	}

	// --- 3. Construct the RRCSetup Message ---
	rrcSetup := &rrcies.RRCSetup{
		Rrc_TransactionIdentifier: rrcTransId,
		CriticalExtensions: rrcies.RRCSetup_CriticalExtensions{
			Choice: rrcies.RRCSetup_CriticalExtensions_Choice_RrcSetup,
			RrcSetup: &rrcies.RRCSetup_IEs{
				RadioBearerConfig: rrcies.RadioBearerConfig{
					Srb_ToAddModList: &rrcies.SRB_ToAddModList{
						Value: []rrcies.SRB_ToAddMod{
							{
								Srb_Identity: rrcies.SRB_Identity{Value: 1}, // SRB1
								// Default configuration implicit
							},
						},
					},
				},
				MasterCellGroup: aper.OctetString(cellGroupBytes),
			},
		},
	}

	// --- 4. Wrap in DL-CCCH Message ---
	// RRCSetup is sent on CCCH (Common Control Channel)
	dlCcchMsg := rrcies.DL_CCCH_Message{
		Message: rrcies.DL_CCCH_MessageType{
			Choice: rrcies.DL_CCCH_MessageType_Choice_C1,
			C1: &rrcies.DL_CCCH_MessageType_C1{
				Choice:   rrcies.DL_CCCH_MessageType_C1_Choice_RrcSetup,
				RrcSetup: rrcSetup,
			},
		},
	}

	// --- 5. Encode ---
	encodedBytes, err := rrc.Encode(&dlCcchMsg)
	if err != nil {
		return fmt.Errorf("failed to encode DL-CCCH (RRCSetup): %w", err)
	}

	// --- 6. Send to UE ---
	if du.ue != nil && du.ue.SendToUeChannel != nil {
		du.Info("[DU] Sending RRCSetup to UE (%d bytes)", len(encodedBytes))
		du.ue.SendToUeChannel <- encodedBytes
	} else {
		du.Warn("[DU] UE Channel not ready, cannot send RRCSetup")
	}

	return nil
}
*/
