package du

import (
	"github.com/lvdund/rrc"
	rrcies "github.com/lvdund/rrc/ies"
)

// handleRrcFromUE listens for RRC messages from the UE channel
func (du *DU) HandleRrcFromUE() {
	du.Info("==== Started listening for RRC messages from UE ===")

	var isInitialMessage bool = true // First message is Initial UL RRC Message Transfer

	for {
		select {
		case rrcBytes, ok := <-du.ue.ReceiveFromUeChannel:
			if !ok {
				du.Warn("ReceiveFromUeChannel closed, stopping RRC handler")
				return
			}

			// Intercept and handle specific RRC messages
			du.dispatchRrcMessage(rrcBytes)

			if isInitialMessage {
				// First RRC message (RRCSetupRequest) -> Initial UL RRC Message Transfer
				if err := du.sendInitialULRRCMessageTransfer(rrcBytes); err != nil {
					du.Error("Failed to send Initial UL RRC Message Transfer: %v", err)
				}
				isInitialMessage = false
			} else {
				// Subsequent RRC messages -> UL RRC Message Transfer
				if err := du.sendULRRCMessageTransfer(rrcBytes); err != nil {
					du.Error("Failed to send UL RRC Message Transfer: %v", err)
				}
			}
		}
	}
}

// dispatchRrcMessage peeks into RRC messages to trigger DU logic (like Handover)
func (du *DU) dispatchRrcMessage(rrcBytes []byte) {
	// Attempt to decode as UL-DCCH (most common for signaling after setup)
	var ulDcchMsg rrcies.UL_DCCH_Message
	if err := rrc.Decode(rrcBytes, &ulDcchMsg); err != nil {
		// Not a DCCH message (likely CCCH/RRCSetupRequest), skip interception
		return
	}

	// Check message type
	c1 := ulDcchMsg.Message.C1
	if c1 == nil {
		return
	}

	switch c1.Choice {
	case rrcies.UL_DCCH_MessageType_C1_Choice_MeasurementReport:
		du.Info("Intercepted MeasurementReport")
		if c1.MeasurementReport != nil {
			du.handleMeasurementReport(c1.MeasurementReport)
		}
	case rrcies.UL_DCCH_MessageType_C1_Choice_RrcReconfigurationComplete:
		du.Info("Intercepted RRCReconfigurationComplete")
		// Signal that Reconfiguration (Handover) is complete
		du.handleRrcReconfigurationComplete()
	}
}

// handleRrcReconfigurationComplete handles completion of HO or setup
// Structure: RRCReconfigurationComplete -> CriticalExtensions -> RrcReconfigurationComplete_IEs
func (du *DU) handleRrcReconfigurationComplete() {
	// If we are Target DU, this means Handover is finished
	if du.IsTargetDU() {
		du.Info("[TARGET DU] RRC Reconfiguration Complete (Transaction ID verified implicit) -> Handover SUCCESS")
		du.SetTargetHandoverState(HO_STATE_COMPLETED)
		// We could notify CU here, but we already forward the RRC message which the CU expects
		// The CU will receive this same RRC message via the F1AP UL RRC Message Transfer
	}
}

// handleMeasurementReport analyzes signal strength for Handover
func (du *DU) handleMeasurementReport(report *rrcies.MeasurementReport) {
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

	du.Info("Measurement Report Analysis: Serving Cell RSRP = %d dBm", servingRSRP)

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

		du.Info("  - Neighbor Cell (PCI: %d): RSRP = %d dBm", pci, targetRSRP)

		// 3. Handover Decision (A3 Event Logic)
		// If Target > Serving + Offset (3dB)
		offset := int64(3)
		if targetRSRP > servingRSRP+offset {
			du.Info("  >>> Handover Condition Met! (Target %d > Serving %d + %d)", targetRSRP, servingRSRP, offset)

			// Trigger Handover if we are not already in it
			if du.hoCtx == nil || du.hoCtx.state == HO_STATE_IDLE {
				du.TriggerHandover(pci)
			}
		}
	}
}

// TriggerHandover initiates the sending of UEContextModificationRequired
func (du *DU) TriggerHandover(targetPci int64) {
	du.Info("[SOURCE DU] Triggering Handover to Cell PCI %d", targetPci)
	du.SetSourceHandoverState(HO_STATE_PREPARATION)

	// Send F1AP message to CU
	// Note: We need to implement sendUeContextModificationRequired in f1ap_handover.go
	// Trigger Handover (send UE Context Modification Required to CU)
	if err := du.sendUeContextModificationRequired(targetPci); err != nil {
		du.Error("Failed to send UE Context Modification Required: %v", err)
		du.SetSourceHandoverState(HO_STATE_FAILED)
	}
}
