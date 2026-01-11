package uecontext

import (
	"fmt"
	"time"

	"github.com/lvdund/rrc"
	rrcies "github.com/lvdund/rrc/ies"
)

// Measurement state
const (
	MEASUREMENT_IDLE = iota
	MEASUREMENT_ACTIVE
)

type MeasurementContext struct {
	state           int
	servingCellRSRP int32
	neighborCells   map[int32]*NeighborCellMeasurement
}

type NeighborCellMeasurement struct {
	cellId int32
	rsrp   int32
	rsrq   int32
}

func (ue *UeContext) initMeasurement() {
	ue.measurement = &MeasurementContext{
		state:         MEASUREMENT_IDLE,
		neighborCells: make(map[int32]*NeighborCellMeasurement),
	}
}

// TriggerMeasurement simulates UE starting measurement and sending report
func (ue *UeContext) TriggerMeasurement() error {
	ue.Info("Starting RRC Measurement")

	// Simulate measurement values
	servingRSRP := int32(-80) // dBm
	targetRSRP := int32(-75)  // dBm (better than serving)
	targetRSRQ := int32(-10)  // dB

	ue.Info("Measurement: Serving RSRP=%d dBm, Target RSRP=%d dBm", servingRSRP, targetRSRP)

	// Check A3 event: Target better than Serving + offset
	offset := int32(3) // 3 dB
	if targetRSRP > servingRSRP+offset {
		ue.Info("A3 Event triggered: Target cell is better")
		return ue.sendMeasurementReport(servingRSRP, targetRSRP, targetRSRQ)
	}

	return nil
}

// sendMeasurementReport creates and sends RRC Measurement Report
func (ue *UeContext) sendMeasurementReport(servingRSRP, targetRSRP, targetRSRQ int32) error {
	ue.Info("Sending RRC Measurement Report")

	// Convert values to proper types
	rsrpServing := rrcies.RSRP_Range{Value: uint64(servingRSRP + 156)} // RSRP_Range: 0..127, mapping from -156..-29 dBm
	rsrpTarget := rrcies.RSRP_Range{Value: uint64(targetRSRP + 156)}
	rsrqTarget := rrcies.RSRQ_Range{Value: uint64(targetRSRQ + 87)} // RSRQ_Range: 0..127, mapping from -87..-30 dB

	measId := rrcies.MeasId{Value: 1}
	servCellId := rrcies.ServCellIndex{Value: 0}
	physCellId := rrcies.PhysCellId{Value: 2} // Target cell PCI

	// Create MeasResult for serving cell
	servingMeasResult := &rrcies.MeasResultNR_measResult{
		CellResults: &rrcies.MeasResultNR_measResult_cellResults{
			ResultsSSB_Cell: &rrcies.MeasQuantityResults{
				Rsrp: &rsrpServing,
				Rsrq: &rsrqTarget,
			},
		},
	}

	// Create MeasResult for neighbor/target cell
	targetMeasResult := &rrcies.MeasResultNR_measResult{
		CellResults: &rrcies.MeasResultNR_measResult_cellResults{
			ResultsSSB_Cell: &rrcies.MeasQuantityResults{
				Rsrp: &rsrpTarget,
				Rsrq: &rsrqTarget,
			},
		},
	}

	// Create Measurement Report message
	measReport := &rrcies.MeasurementReport{
		CriticalExtensions: rrcies.MeasurementReport_CriticalExtensions{
			Choice: rrcies.MeasurementReport_CriticalExtensions_Choice_MeasurementReport,
			MeasurementReport: &rrcies.MeasurementReport_IEs{
				MeasResults: rrcies.MeasResults{
					MeasId: measId,
					MeasResultServingMOList: rrcies.MeasResultServMOList{
						Value: []rrcies.MeasResultServMO{
							{
								ServCellId: servCellId,
								MeasResultServingCell: rrcies.MeasResultNR{
									MeasResult: servingMeasResult,
								},
							},
						},
					},
					MeasResultNeighCells: &rrcies.MeasResults_measResultNeighCells{
						Choice: rrcies.MeasResults_measResultNeighCells_Choice_MeasResultListNR,
						MeasResultListNR: &rrcies.MeasResultListNR{
							Value: []rrcies.MeasResultNR{
								{
									PhysCellId: &physCellId,
									MeasResult: targetMeasResult,
								},
							},
						},
					},
				},
			},
		},
	}

	// Wrap in UL-DCCH-Message
	ulDcchMsg := rrcies.UL_DCCH_Message{
		Message: rrcies.UL_DCCH_MessageType{
			Choice: rrcies.UL_DCCH_MessageType_Choice_C1,
			C1: &rrcies.UL_DCCH_MessageType_C1{
				Choice:            rrcies.UL_DCCH_MessageType_C1_Choice_MeasurementReport,
				MeasurementReport: measReport,
			},
		},
	}

	encoded, err := rrc.Encode(&ulDcchMsg)
	if err != nil {
		ue.Error("Failed to encode Measurement Report: %v", err)
		return err
	}

	// Send to DU
	ue.SendToDuChannel <- encoded
	ue.Info("Measurement Report sent successfully")
	return nil
}

// HandleRrcReconfiguration handles RRC Reconfiguration message (for handover)
func (ue *UeContext) HandleRrcReconfiguration(rrcBytes []byte) error {
	ue.Info("Handling RRC Reconfiguration (Handover Command)")

	// Decode RRC Reconfiguration
	var dlDcchMsg rrcies.DL_DCCH_Message
	if err := rrc.Decode(rrcBytes, &dlDcchMsg); err != nil {
		ue.Error("Failed to decode RRC Reconfiguration: %v", err)
		return err
	}

	// Extract RRC Reconfiguration
	if dlDcchMsg.Message.C1 == nil ||
		dlDcchMsg.Message.C1.RrcReconfiguration == nil {
		ue.Error("Invalid RRC Reconfiguration message")
		return fmt.Errorf("invalid message structure")
	}

	rrcReconfig := dlDcchMsg.Message.C1.RrcReconfiguration
	ue.Info("RRC Reconfiguration received, TransactionId=%d", rrcReconfig.Rrc_TransactionIdentifier)

	// Check if this is a handover command by examining ReconfigurationWithSync
	isHandover := false
	
	if rrcReconfig.CriticalExtensions.RrcReconfiguration != nil &&
		rrcReconfig.CriticalExtensions.RrcReconfiguration.SecondaryCellGroup != nil {
		
		// Decode SecondaryCellGroup to check for ReconfigurationWithSync
		var cellGroupConfig rrcies.CellGroupConfig
		if err := rrc.Decode(*rrcReconfig.CriticalExtensions.RrcReconfiguration.SecondaryCellGroup, &cellGroupConfig); err != nil {
			ue.Error("Failed to decode SecondaryCellGroup: %v", err)
		} else {
			// Check if SpCellConfig contains ReconfigurationWithSync
			if cellGroupConfig.SpCellConfig != nil && 
				cellGroupConfig.SpCellConfig.ReconfigurationWithSync != nil {
				isHandover = true
				ue.Info("ReconfigurationWithSync detected - this is a handover command")
			}
		}
	}

	if isHandover {
		ue.Info("Handover to target cell, new C-RNTI will be assigned")
		// Simulate Random Access procedure to target cell
		go ue.performRandomAccess()
	} else {
		ue.Info("RRC Reconfiguration completed (not a handover)")
	}

	return nil
}

// performRandomAccess simulates Random Access procedure with target DU
func (ue *UeContext) performRandomAccess() {
	ue.Info("Performing Random Access to Target Cell")

	// Simulate Msg1 (RACH Preamble) transmission
	ue.Info("Sending Msg1 (RACH Preamble)")
	time.Sleep(10 * time.Millisecond)

	// Simulate Msg2 (RAR) reception
	ue.Info("Received Msg2 (Random Access Response)")
	time.Sleep(10 * time.Millisecond)

	// Send RRC Reconfiguration Complete (Msg3)
	if err := ue.sendRrcReconfigurationComplete(); err != nil {
		ue.Error("Failed to send RRC Reconfiguration Complete: %v", err)
		return
	}

	ue.Info("Handover completed successfully")
}

// sendRrcReconfigurationComplete sends RRC Reconfiguration Complete
func (ue *UeContext) sendRrcReconfigurationComplete() error {
	ue.Info("Sending RRC Reconfiguration Complete")

	transactionId := rrcies.RRC_TransactionIdentifier{Value: 0}

	// Create RRC Reconfiguration Complete
	rrcComplete := &rrcies.RRCReconfigurationComplete{
		Rrc_TransactionIdentifier: transactionId,
		CriticalExtensions: rrcies.RRCReconfigurationComplete_CriticalExtensions{
			Choice:                     rrcies.RRCReconfigurationComplete_CriticalExtensions_Choice_RrcReconfigurationComplete,
			RrcReconfigurationComplete: &rrcies.RRCReconfigurationComplete_IEs{},
		},
	}

	// Wrap in UL-DCCH-Message
	ulDcchMsg := rrcies.UL_DCCH_Message{
		Message: rrcies.UL_DCCH_MessageType{
			Choice: rrcies.UL_DCCH_MessageType_Choice_C1,
			C1: &rrcies.UL_DCCH_MessageType_C1{
				Choice:                     rrcies.UL_DCCH_MessageType_C1_Choice_RrcReconfigurationComplete,
				RrcReconfigurationComplete: rrcComplete,
			},
		},
	}

	encoded, err := rrc.Encode(&ulDcchMsg)
	if err != nil {
		ue.Error("Failed to encode RRC Reconfiguration Complete: %v", err)
		return err
	}

	// Send to DU
	ue.SendToDuChannel <- encoded
	ue.Info("RRC Reconfiguration Complete sent successfully")
	return nil
}