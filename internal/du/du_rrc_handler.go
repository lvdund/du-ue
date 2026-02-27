package du

import (
	"github.com/lvdund/rrc"
	rrcies "github.com/lvdund/rrc/ies"
)

// HandleRrcFromUE listens for RRC messages from a specific UE channel
func (du *DU) HandleRrcFromUE(ctx *DuUeContext) {
	du.Info("[UE %d] Started listening for RRC messages", ctx.DuUeF1apId)

	var isInitialMessage bool = true // First message is Initial UL RRC Message Transfer

	for {
		select {
		case rrcBytes, ok := <-ctx.UeChannel.ReceiveFromUeChannel:
			if !ok {
				du.Warn("[UE %d] ReceiveFromUeChannel closed, stopping RRC handler", ctx.DuUeF1apId)
				return
			}

			// Intercept and handle specific RRC messages, and determine SRB ID
			srbID := du.processAndGetSrbID(ctx, rrcBytes)

			if isInitialMessage {
				// First RRC message (RRCSetupRequest) -> Initial UL RRC Message Transfer
				if err := du.sendInitialULRRCMessageTransfer(rrcBytes, ctx.DuUeF1apId, ctx.CRnti); err != nil {
					du.Error("[UE %d] Failed to send Initial UL RRC Message Transfer: %v", ctx.DuUeF1apId, err)
				}
				isInitialMessage = false
			} else {
				// Subsequent RRC messages -> UL RRC Message Transfer
				if err := du.sendULRRCMessageTransfer(rrcBytes, ctx.CuUeF1apId, ctx.DuUeF1apId, srbID); err != nil {
					du.Error("[UE %d] Failed to send UL RRC Message Transfer: %v", ctx.DuUeF1apId, err)
				}
			}
		}
	}
}

// processAndGetSrbID peeks into RRC messages to trigger DU logic and returns the appropriate SRB ID
func (du *DU) processAndGetSrbID(ctx *DuUeContext, rrcBytes []byte) int64 {
	// Default to SRB1 (High Priority DCCH) for most signaling
	srbID := int64(1)

	// Attempt to decode as UL-DCCH (most common for signaling after setup)
	var ulDcchMsg rrcies.UL_DCCH_Message
	if err := rrc.Decode(rrcBytes, &ulDcchMsg); err != nil {
		// Not a DCCH message (likely CCCH/RRCSetupRequest), retain default or handle if needed
		return srbID
	}

	// Check message type
	c1 := ulDcchMsg.Message.C1
	if c1 == nil {
		return srbID
	}

	switch c1.Choice {
	case rrcies.UL_DCCH_MessageType_C1_Choice_MeasurementReport:
		du.Info("[UE %d] Intercepted MeasurementReport", ctx.DuUeF1apId)
		if c1.MeasurementReport != nil {
			du.handleMeasurementReport(ctx, c1.MeasurementReport)
		}
		// Measurement Reports go on SRB1
	case rrcies.UL_DCCH_MessageType_C1_Choice_RrcReconfigurationComplete:
		du.Info("[UE %d] Intercepted RRCReconfigurationComplete", ctx.DuUeF1apId)
		// Signal that Reconfiguration (Handover) is complete
		du.HandleRrcReconfigurationComplete(ctx)
		// Reconfig Complete goes on SRB1
	case rrcies.UL_DCCH_MessageType_C1_Choice_UlInformationTransfer:
		// piggybacked NAS message -> SRB2 (if AS security is active, which it typically is for this message)
		// For now, we assume if we see this, we use SRB2
		srbID = 2
	}

	return srbID
}

// HandleRrcReconfigurationComplete handles completion of HO or setup
func (du *DU) HandleRrcReconfigurationComplete(ctx *DuUeContext) {
	// 1. PDU Session Activation (Initial Setup)
	if ctx.PduSessions != nil {
		for id, session := range ctx.PduSessions {
			if session.State == PduSessionStateReserved {
				session.State = PduSessionStateActive
				du.Info("[UE %d] PDU Session %d Activated (RRC Reconfiguration Complete)", ctx.DuUeF1apId, id)
			}
		}
	}

	// 2. Handover Completion (Target DU)
	// If we are Target DU for this UE, this means Handover is finished
	if du.IsTargetDU(ctx) {
		du.Info("[TARGET DU][UE %d] RRC Reconfiguration Complete -> Handover SUCCESS", ctx.DuUeF1apId)
		du.SetTargetHandoverState(ctx, HO_STATE_COMPLETED)
		// We could notify CU here, but we already forward the RRC message which the CU expects
		// The CU will receive this same RRC message via the F1AP UL RRC Message Transfer
	}
}

// handleMeasurementReport analyzes signal strength for Handover
func (du *DU) handleMeasurementReport(ctx *DuUeContext, report *rrcies.MeasurementReport) {
	// Navigate the Deep Struct Hierarchy
	// MeasurementReport -> CriticalExtensions -> MeasResults -> MeasResultNeighCells

	ext := report.CriticalExtensions.MeasurementReport
	if ext == nil {
		du.Debug("MeasurementReport extension is nil")
		return
	}

	// 1. Get Serving Cell RSRP
	measResults := ext.MeasResults
	if len(measResults.MeasResultServingMOList.Value) == 0 {
		du.Debug("No serving cell results")
		return
	}
	servingCell := measResults.MeasResultServingMOList.Value[0]
	servingRSRP := int64(servingCell.MeasResultServingCell.MeasResult.CellResults.ResultsSSB_Cell.Rsrp.Value) - 156

	du.Info("[UE %d] Measurement Report: Serving Cell RSRP = %d dBm", ctx.DuUeF1apId, servingRSRP)

	// 2. Check Neighbor Cells
	if measResults.MeasResultNeighCells == nil {
		du.Debug("No neighbor cells")
		return
	}

	// We only support MeasResultListNR for now
	if measResults.MeasResultNeighCells.Choice != rrcies.MeasResults_measResultNeighCells_Choice_MeasResultListNR ||
		measResults.MeasResultNeighCells.MeasResultListNR == nil {
		du.Debug("Neighbors not ListNR format")
		return
	}

	neighbors := measResults.MeasResultNeighCells.MeasResultListNR.Value
	for _, neighbor := range neighbors {
		if neighbor.MeasResult.CellResults.ResultsSSB_Cell.Rsrp == nil {
			continue
		}

		targetRSRP := int64(neighbor.MeasResult.CellResults.ResultsSSB_Cell.Rsrp.Value) - 156
		pci := int64(neighbor.PhysCellId.Value)

		du.Info("[UE %d] Neighbor Cell (PCI: %d): RSRP = %d dBm", ctx.DuUeF1apId, pci, targetRSRP)

		if ctx.HoCtx == nil || ctx.HoCtx.state == HO_STATE_IDLE {
			// RRM: PCI Validation
			if !du.isValidNeighbor(pci) {
				du.Warn("[UE %d] Handover Rejected: Unknown Neighbor PCI %d", ctx.DuUeF1apId, pci)
				continue
			}

			// RRM: Admission Control (Simulation)
			if !du.checkAdmissionControl(pci) {
				du.Warn("[UE %d] Handover Rejected: Target Cell PCI %d Overloaded", ctx.DuUeF1apId, pci)
				continue
			}

			du.Info("[UE %d] Triggering Handover to Target PCI %d", ctx.DuUeF1apId, pci)
			du.TriggerHandover(ctx, pci)
		}
	}
}

// isValidNeighbor checks if PCI is in the configured neighbor list
func (du *DU) isValidNeighbor(pci int64) bool {
	return du.resourceMgr.IsValidNeighbor(pci)
}

// checkAdmissionControl simulates load checking on the target cell
func (du *DU) checkAdmissionControl(pci int64) bool {
	// Simulate overloaded cell for PCI 999
	if pci == 999 {
		return false
	}
	return true
}

// TriggerHandover initiates the sending of UEContextModificationRequired
func (du *DU) TriggerHandover(ctx *DuUeContext, targetPci int64) {
	du.Info("[SOURCE DU][UE %d] Triggering Handover to Cell PCI %d", ctx.DuUeF1apId, targetPci)
	du.SetSourceHandoverState(ctx, HO_STATE_PREPARATION)

	// Trigger Handover (send UE Context Modification Required to CU)
	if err := du.sendUeContextModificationRequired(ctx, targetPci); err != nil {
		du.Error("[UE %d] Failed to send UE Context Modification Required: %v", ctx.DuUeF1apId, err)
		du.SetSourceHandoverState(ctx, HO_STATE_FAILED)
	}
}
