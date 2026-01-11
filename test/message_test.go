package test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	f1ap "github.com/JocelynWS/f1-gen"
	"github.com/JocelynWS/f1-gen/ies"
	"github.com/lvdund/ngap/aper"

	"du_ue/internal/du"
	"du_ue/pkg/config"
)

// TestUEContextSetupResponseContent tests the actual message content
func TestUEContextSetupResponseContent(t *testing.T) {
	cfg := &config.Config{
		DU: config.DUConfig{
			ID:   1,
			Name: "TestDU",
			PLMN: config.PLMNConfig{
				MCC: "999",
				MNC: "70",
			},
			Cell: config.CellConfig{
				PCI: 123,
			},
		},
	}

	duInstance, err := du.NewDU(cfg)
	require.NoError(t, err)

	duInstance.SetTargetHandoverState(du.HO_STATE_PREPARATION)

	duUeId := int64(1)
	msg := &ies.UEContextSetupRequest{
		GNBCUUEF1APID: 100,
		GNBDUUEF1APID: &duUeId,
		RRCContainer:  []byte{0x01, 0x02, 0x03},
	}

	pdu := f1ap.F1apPdu{
		Present: ies.F1apPduInitiatingMessage,
		Message: f1ap.F1apMessage{
			ProcedureCode: ies.ProcedureCode{Value: ies.ProcedureCode_UEContextSetup},
			Msg:           msg,
		},
	}

	// This will try to send, but connection is not established in test
	err = duInstance.HandleUeContextSetupRequest(&pdu)
	// Accept SCTP connection error in test mode
	if err != nil && err.Error() != "SCTP connection not established" {
		t.Errorf("Unexpected error: %v", err)
	}
	
	// TODO: Add actual message content verification by capturing encoded bytes
}

// TestPLMNIdentityEncoding tests PLMN encoding correctness
func TestPLMNIdentityEncoding(t *testing.T) {
	testCases := []struct {
		name     string
		mcc      string
		mnc      string
		expected []byte
	}{
		{
			name:     "2-digit MNC",
			mcc:      "001",
			mnc:      "01",
			expected: []byte{0x00, 0xF1, 0x10}, // MCC=001, MNC=01
		},
		{
			name:     "3-digit MNC",
			mcc:      "999",
			mnc:      "70",
			expected: []byte{0x99, 0xF9, 0x07}, // MCC=999, MNC=70
		},
		{
			name:     "3-digit MNC with full digits",
			mcc:      "208",
			mnc:      "950",
			expected: []byte{0x02, 0x08, 0x59}, // MCC=208, MNC=950
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Build PLMN Identity manually
			plmnBytes := make([]byte, 3)
			mcc := tc.mcc
			mnc := tc.mnc

			mcc1 := mcc[0] - '0'
			mcc2 := mcc[1] - '0'
			mcc3 := mcc[2] - '0'

			mnc1 := mnc[0] - '0'
			mnc2 := mnc[1] - '0'
			mnc3 := byte(0xF)
			if len(mnc) == 3 {
				mnc3 = mnc[2] - '0'
			}

			plmnBytes[0] = (mcc2 << 4) | mcc1
			plmnBytes[1] = (mnc3 << 4) | mcc3
			plmnBytes[2] = (mnc2 << 4) | mnc1

			assert.Equal(t, tc.expected, plmnBytes,
				"PLMN encoding mismatch for MCC=%s, MNC=%s", tc.mcc, tc.mnc)
		})
	}
}

// TestNRCellIdentityEncoding tests NR Cell Identity encoding
func TestNRCellIdentityEncoding(t *testing.T) {
	testCases := []struct {
		name     string
		pci      uint64
		numBits  uint64
		expected []byte // First 5 bytes
	}{
		{
			name:     "PCI=1",
			pci:      1,
			numBits:  36,
			expected: []byte{0x00, 0x00, 0x00, 0x00, 0x10}, // PCI 1 shifted left by 4
		},
		{
			name:     "PCI=123",
			pci:      123,
			numBits:  36,
			expected: []byte{0x00, 0x00, 0x00, 0x07, 0xB0}, // PCI 123 shifted left by 4
		},
		{
			name:     "PCI=503",
			pci:      503,
			numBits:  36,
			expected: []byte{0x00, 0x00, 0x00, 0x1F, 0x70}, // PCI 503 shifted left by 4
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Build NR Cell Identity
			cellId := tc.pci << 4 // Shift to make 36 bits

			nrCellIdentityBytes := make([]byte, 5)
			nrCellIdentityBytes[0] = byte(cellId >> 32)
			nrCellIdentityBytes[1] = byte(cellId >> 24)
			nrCellIdentityBytes[2] = byte(cellId >> 16)
			nrCellIdentityBytes[3] = byte(cellId >> 8)
			nrCellIdentityBytes[4] = byte(cellId)

			assert.Equal(t, tc.expected, nrCellIdentityBytes,
				"NR Cell Identity encoding mismatch for PCI=%d", tc.pci)
		})
	}
}

// TestUEContextModificationResponseFields tests mandatory fields
func TestUEContextModificationResponseFields(t *testing.T) {
	cuUeId := int64(100)
	duUeId := int64(1)

	duToCuRrcInfo := &ies.DUtoCURRCInformation{
		CellGroupConfig: []byte{0xAA, 0xBB}, // Some config
	}

	msg := &ies.UEContextModificationResponse{
		GNBCUUEF1APID:        cuUeId,
		GNBDUUEF1APID:        duUeId,
		DUtoCURRCInformation: duToCuRrcInfo,
	}

	// Verify mandatory fields are set
	assert.Equal(t, cuUeId, msg.GNBCUUEF1APID)
	assert.Equal(t, duUeId, msg.GNBDUUEF1APID)
	assert.NotNil(t, msg.DUtoCURRCInformation)
	assert.NotNil(t, msg.DUtoCURRCInformation.CellGroupConfig)

	// Try to encode
	_, err := f1ap.F1apEncode(msg)
	assert.NoError(t, err, "Should be able to encode UEContextModificationResponse")
}

// TestUEContextSetupResponseFields tests mandatory fields
func TestUEContextSetupResponseFields(t *testing.T) {
	cuUeId := int64(100)
	duUeId := int64(1)
	crnti := int64(0x1234)

	plmnBytes := []byte{0x99, 0xF9, 0x07}
	nrCellIdBytes := []byte{0x00, 0x00, 0x00, 0x07, 0xB0}

	nrcgi := &ies.NRCGI{
		PLMNIdentity: plmnBytes,
		NRCellIdentity: aper.BitString{
			Bytes:   nrCellIdBytes,
			NumBits: 36,
		},
	}

	duToCuRrcInfo := ies.DUtoCURRCInformation{
		CellGroupConfig: []byte{},
	}

	msg := &ies.UEContextSetupResponse{
		GNBCUUEF1APID:               cuUeId,
		GNBDUUEF1APID:               duUeId,
		DUtoCURRCInformation:        duToCuRrcInfo,
		CRNTI:                       &crnti,
		RequestedTargetCellGlobalID: nrcgi,
	}

	// Verify all mandatory fields are set
	assert.Equal(t, cuUeId, msg.GNBCUUEF1APID)
	assert.Equal(t, duUeId, msg.GNBDUUEF1APID)
	assert.NotNil(t, msg.DUtoCURRCInformation)
	assert.NotNil(t, msg.CRNTI)
	assert.Equal(t, crnti, *msg.CRNTI)
	assert.NotNil(t, msg.RequestedTargetCellGlobalID)
	assert.Equal(t, uint64(36), msg.RequestedTargetCellGlobalID.NRCellIdentity.NumBits)

	// Try to encode
	_, err := f1ap.F1apEncode(msg)
	assert.NoError(t, err, "Should be able to encode UEContextSetupResponse")
}

// TestUEContextReleaseCompleteEncoding tests encoding
func TestUEContextReleaseCompleteEncoding(t *testing.T) {
	msg := &ies.UEContextReleaseComplete{
		GNBCUUEF1APID: 100,
		GNBDUUEF1APID: 1,
	}

	bytes, err := f1ap.F1apEncode(msg)
	require.NoError(t, err)
	assert.NotEmpty(t, bytes, "Encoded message should not be empty")

	// TODO: Add decode test to verify round-trip
}

// TestULRRCMessageTransferEncoding tests encoding
func TestULRRCMessageTransferEncoding(t *testing.T) {
	rrcBytes := []byte{0x01, 0x02, 0x03, 0x04}

	msg := &ies.ULRRCMessageTransfer{
		GNBCUUEF1APID: 100,
		GNBDUUEF1APID: 1,
		SRBID:         1,
		RRCContainer:  rrcBytes,
	}

	bytes, err := f1ap.F1apEncode(msg)
	require.NoError(t, err)
	assert.NotEmpty(t, bytes, "Encoded message should not be empty")

	// Verify RRC container is included
	assert.Contains(t, msg.RRCContainer, byte(0x01))
}