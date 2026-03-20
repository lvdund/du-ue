package du

import (
	"encoding/binary"
	"fmt"

	f1ap "github.com/JocelynWS/f1-gen"
	"github.com/JocelynWS/f1-gen/ies"
	"github.com/lvdund/ngap/aper"
	"github.com/lvdund/rrc"
	rrcies "github.com/lvdund/rrc/ies"
)

// sendInitialULRRCMessageTransfer sends Initial UL RRC Message Transfer to CU-CP
func (du *DU) sendInitialULRRCMessageTransfer(rrcBytes []byte, duUeF1apId int64, cRnti int64) error {
	du.Info("Sending Initial UL RRC Message Transfer (DU-UE-ID=%d, C-RNTI=%d)", duUeF1apId, cRnti)

	// Convert MCC/MNC to PLMN bytes
	plmnBytes := ConvertMccMncToPlmn(du.Config.PLMN.MCC, du.Config.PLMN.MNC)

	// Create NRCGI
	// Convert NRCellIdentity uint64 to 5-byte/36-bit BitString
	cellIdBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(cellIdBytes, du.Config.Cell.NRCellIdentity)
	
	nrcgi := ies.NRCGI{
		PLMNIdentity: plmnBytes,
		NRCellIdentity: aper.BitString{
			Bytes:   cellIdBytes[3:], // Last 5 bytes for 36-40 bits
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
		GNBDUUEF1APID:      duUeF1apId,
		NRCGI:              nrcgi,
		CRNTI:              cRnti,
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
func (du *DU) sendULRRCMessageTransfer(rrcBytes []byte, cuUeF1apId, duUeF1apId, srbID int64) error {
	du.Info("Sending UL RRC Message Transfer (CU-UE-ID=%d, DU-UE-ID=%d, SRB=%d)", cuUeF1apId, duUeF1apId, srbID)

	// Create UL RRC Message Transfer
	msg := ies.ULRRCMessageTransfer{
		GNBCUUEF1APID: cuUeF1apId,
		GNBDUUEF1APID: duUeF1apId,
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
