package du

import (
	"fmt"

	f1ap "github.com/JocelynWS/f1-gen"
	"github.com/JocelynWS/f1-gen/ies"
	"github.com/lvdund/ngap/aper"
	"github.com/lvdund/rrc"
	rrcies "github.com/lvdund/rrc/ies"
)

var (
	//FIX: now UE IDs (for single UE simulation)
	DU_UE_F1AP_ID int64 = 0
	CU_UE_F1AP_ID int64 = 0
	C_RNTI        int64 = 1
)

// sendInitialULRRCMessageTransfer sends Initial UL RRC Message Transfer to CU-CP
func (du *DU) sendInitialULRRCMessageTransfer(rrcBytes []byte) error {
	du.Info("Sending Initial UL RRC Message Transfer")

	// Convert MCC/MNC to PLMN bytes
	plmnBytes := convertMccMncToPlmn(du.Config.PLMN.MCC, du.Config.PLMN.MNC)

	// Create NRCGI
	nrcgi := ies.NRCGI{
		PLMNIdentity: plmnBytes,
		NRCellIdentity: aper.BitString{
			Bytes:   []byte{0x0F, 0xFF, 0xFF, 0xFF, 0xFF},
			NumBits: 36,
		},
	}

	// DUtoCURRCContainer <- cellGroupConfig
	cellGroupConfig := rrcies.CellGroupConfig{
		CellGroupId: rrcies.CellGroupId{Value: 0},
	}

	encodedCellGroupConfig, err := rrc.Encode(&cellGroupConfig)
	if err != nil {
		return fmt.Errorf("encode CellGroupConfig: %w", err)
	}

	// Create Initial UL RRC Message Transfer
	msg := ies.InitialULRRCMessageTransfer{
		GNBDUUEF1APID:      DU_UE_F1AP_ID,
		NRCGI:              nrcgi,
		CRNTI:              C_RNTI,
		RRCContainer:       rrcBytes,
		TransactionID:      0,
		DUtoCURRCContainer: encodedCellGroupConfig,
	}

	// Encode F1AP message directly (F1apEncode takes the message struct)
	f1apBytes, err := f1ap.F1apEncode(&msg)
	if err != nil {
		return fmt.Errorf("encode Initial UL RRC Message Transfer: %w", err)
	}

	// Send via SCTP (PPID=62 is already set in Send method)
	if err := du.f1Client.Send(f1apBytes); err != nil {
		du.Error("Failed to send Initial UL RRC Message Transfer: %v", err)
		return err
	}

	du.Info("Successfully sent Initial UL RRC Message Transfer to CU-CP")
	return nil
}

// sendULRRCMessageTransfer sends UL RRC Message Transfer to CU-CP
func (du *DU) sendULRRCMessageTransfer(rrcBytes []byte) error {
	du.Info("Sending UL RRC Message Transfer")

	// Determine SRB ID based on RRC message type
	// SRB0 = 0 (CCCH), SRB1 = 1 (DCCH), SRB2 = 2 (DCCH)
	srbID := int64(1) // Default to SRB1 for DCCH messages

	// Create UL RRC Message Transfer
	msg := ies.ULRRCMessageTransfer{
		GNBCUUEF1APID: CU_UE_F1AP_ID,
		GNBDUUEF1APID: DU_UE_F1AP_ID,
		SRBID:         srbID,
		RRCContainer:  rrcBytes,
	}

	// Encode F1AP message directly (F1apEncode takes the message struct)
	f1apBytes, err := f1ap.F1apEncode(&msg)
	if err != nil {
		return fmt.Errorf("encode UL RRC Message Transfer: %w", err)
	}

	// Send via SCTP (PPID=62 is already set in Send method)
	return du.f1Client.Send(f1apBytes)
}
