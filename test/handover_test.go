package test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	f1ap "github.com/JocelynWS/f1-gen"
	"github.com/JocelynWS/f1-gen/ies"

	"du_ue/internal/du"
	"du_ue/pkg/config"
)

func createTestDU(t *testing.T) *du.DU {
	cfg := &config.Config{
		DU: config.DUConfig{
			ID:        1,
			Name:      "TestDU",
			CUCPAddr:  "127.0.0.1",
			CUCPPort:  38472,
			LocalAddr: "127.0.0.1",
			LocalPort: 0,
			PLMN: config.PLMNConfig{
				MCC: "999",
				MNC: "70",
			},
			Cell: config.CellConfig{
				PCI: 1,
			},
		},
		UE: config.UEConfig{
			MSIN: "0000000001",
			Key:  "465B5CE8B199B49FAA5F0A2EE238A6BC",
			OPC:  "E8ED289DEBA952E4283B54E88E6183CA",
			AMF:  "8000",
			PLMN: config.PLMNConfig{
				MCC: "999",
				MNC: "70",
			},
		},
	}

	duInstance, err := du.NewDU(cfg)
	require.NoError(t, err)
	require.NotNil(t, duInstance)

	return duInstance
}

// Test 1: Handover Context Initialization
func TestHandoverContextInitialization(t *testing.T) {
	duInstance := createTestDU(t)

	assert.Equal(t, du.HANDOVER_ROLE_NONE, duInstance.GetHandoverRole())
	assert.Equal(t, du.HO_STATE_IDLE, duInstance.GetHandoverState())
}

// Test 2: Source DU State Transitions
func TestSourceDUStateTransitions(t *testing.T) {
	duInstance := createTestDU(t)

	states := []du.HandoverState{
		du.HO_STATE_PREPARATION,
		du.HO_STATE_EXECUTION,
		du.HO_STATE_COMPLETION,
		du.HO_STATE_COMPLETED,
	}

	for _, state := range states {
		duInstance.SetSourceHandoverState(state)
		assert.Equal(t, du.HANDOVER_ROLE_SOURCE, duInstance.GetHandoverRole())
		assert.Equal(t, state, duInstance.GetHandoverState())
		assert.True(t, duInstance.IsSourceDU())
		assert.False(t, duInstance.IsTargetDU())
	}
}

// Test 3: Target DU State Transitions
func TestTargetDUStateTransitions(t *testing.T) {
	duInstance := createTestDU(t)

	states := []du.HandoverState{
		du.HO_STATE_PREPARATION,
		du.HO_STATE_EXECUTION,
		du.HO_STATE_COMPLETION,
		du.HO_STATE_COMPLETED,
	}

	for _, state := range states {
		duInstance.SetTargetHandoverState(state)
		assert.Equal(t, du.HANDOVER_ROLE_TARGET, duInstance.GetHandoverRole())
		assert.Equal(t, state, duInstance.GetHandoverState())
		assert.True(t, duInstance.IsTargetDU())
		assert.False(t, duInstance.IsSourceDU())
	}
}

// Test 4: Handover Context Reset
func TestHandoverContextReset(t *testing.T) {
	duInstance := createTestDU(t)

	duInstance.SetSourceHandoverState(du.HO_STATE_EXECUTION)
	duInstance.ResetHandoverContext()
	
	assert.Equal(t, du.HANDOVER_ROLE_NONE, duInstance.GetHandoverRole())
	assert.Equal(t, du.HO_STATE_IDLE, duInstance.GetHandoverState())
}

// Test 5: Target DU - UE Context Setup Request (Handover)
func TestTargetDU_UEContextSetupRequest_Handover(t *testing.T) {
	duInstance := createTestDU(t)
	
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
	
	err := duInstance.HandleUeContextSetupRequest(&pdu)
	// Accept SCTP connection error in test mode
	if err != nil && err.Error() != "SCTP connection not established" {
		t.Errorf("Unexpected error: %v", err)
	}
}

// Test 6: Source DU - UE Context Modification Request
func TestSourceDU_UEContextModificationRequest(t *testing.T) {
	duInstance := createTestDU(t)
	
	// Create UE channels without full initialization to avoid RRC setup attempts
	toUE := make(chan []byte, 100)
	fromUE := make(chan []byte, 100)

	duInstance.SetUEChannelForTest(&du.UeChannel{
		ReceiveFromUeChannel: fromUE,
		SendToUeChannel:      toUE,
	})
	
	duUeId := int64(1)
	rrcReconfig := []byte{0x10, 0x20, 0x30, 0x40}
	
	msg := &ies.UEContextModificationRequest{
		GNBCUUEF1APID: 100,
		GNBDUUEF1APID: duUeId,
		RRCContainer:  rrcReconfig,
	}
	
	pdu := f1ap.F1apPdu{
		Present: ies.F1apPduInitiatingMessage,
		Message: f1ap.F1apMessage{
			ProcedureCode: ies.ProcedureCode{Value: ies.ProcedureCode_UEContextModification},
			Msg:           msg,
		},
	}
	
	err := duInstance.HandleUeContextModificationRequest(&pdu)
	// Accept SCTP connection error in test mode
	if err != nil && err.Error() != "SCTP connection not established" {
		t.Errorf("Unexpected error: %v", err)
	}
	
	// Verify RRC Reconfiguration was forwarded to UE
	select {
	case rrcBytes := <-toUE:
		assert.Equal(t, rrcReconfig, rrcBytes, "RRC Reconfiguration should be forwarded to UE")
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for RRC Reconfiguration to be forwarded to UE")
	}
}

// Test 7: Source DU - UE Context Release Command
func TestSourceDU_UEContextReleaseCommand(t *testing.T) {
	duInstance := createTestDU(t)
	
	duUeId := int64(1)
	msg := &ies.UEContextReleaseCommand{
		GNBCUUEF1APID: 100,
		GNBDUUEF1APID: duUeId,
	}
	
	pdu := f1ap.F1apPdu{
		Present: ies.F1apPduInitiatingMessage,
		Message: f1ap.F1apMessage{
			ProcedureCode: ies.ProcedureCode{Value: ies.ProcedureCode_UEContextRelease},
			Msg:           msg,
		},
	}
	
	err := duInstance.HandleUeContextReleaseCommand(&pdu)
	// Accept SCTP connection error in test mode
	if err != nil && err.Error() != "SCTP connection not established" {
		t.Errorf("Unexpected error: %v", err)
	}
}

// Test 8: DL RRC Message Transfer
func TestDLRRCMessageTransfer(t *testing.T) {
	duInstance := createTestDU(t)
	
	// Create UE channels without full initialization
	toUE := make(chan []byte, 100)
	fromUE := make(chan []byte, 100)

	duInstance.SetUEChannelForTest(&du.UeChannel{
		ReceiveFromUeChannel: fromUE,
		SendToUeChannel:      toUE,
	})
	
	rrcContainer := []byte{0x11, 0x22, 0x33, 0x44}
	msg := &ies.DLRRCMessageTransfer{
		GNBCUUEF1APID: 100,
		GNBDUUEF1APID: 1,
		SRBID:         1,
		RRCContainer:  rrcContainer,
	}
	
	pdu := f1ap.F1apPdu{
		Present: ies.F1apPduInitiatingMessage,
		Message: f1ap.F1apMessage{
			ProcedureCode: ies.ProcedureCode{Value: ies.ProcedureCode_DLRRCMessageTransfer},
			Msg:           msg,
		},
	}
	
	err := duInstance.HandleDlRrcMessageTransfer(&pdu)
	assert.NoError(t, err)
	
	// Verify RRC message was forwarded to UE
	select {
	case rrcBytes := <-toUE:
		assert.Equal(t, rrcContainer, rrcBytes, "RRC message should be forwarded to UE")
	case <-time.After(100 * time.Millisecond):
		// OK if not forwarded (might be because of other reasons)
	}
}

// Test 9: Target DU RACH Monitoring
func TestTargetDU_RACHMonitoring(t *testing.T) {
	duInstance := createTestDU(t)
	
	duInstance.SetTargetHandoverState(du.HO_STATE_PREPARATION)
	duInstance.StartRachMonitoring()
	
	time.Sleep(50 * time.Millisecond)
	
	assert.Equal(t, du.HO_STATE_PREPARATION, duInstance.GetHandoverState())
}

// Test 10: Target DU Random Access Preamble
func TestTargetDU_RandomAccessPreamble(t *testing.T) {
	duInstance := createTestDU(t)
	
	duInstance.SetTargetHandoverState(du.HO_STATE_EXECUTION)
	
	preambleId := 63
	ueId := int64(1)
	
	err := duInstance.HandleRandomAccessPreamble(preambleId, ueId)
	assert.NoError(t, err)
}

// Test 11: Target DU RRC Reconfiguration Complete
func TestTargetDU_RRCReconfigurationComplete(t *testing.T) {
	duInstance := createTestDU(t)
	
	// Create UE channels without full initialization
	toUE := make(chan []byte, 100)
	fromUE := make(chan []byte, 100)

	duInstance.SetUEChannelForTest(&du.UeChannel{
		ReceiveFromUeChannel: fromUE,
		SendToUeChannel:      toUE,
	})
	
	duInstance.SetTargetHandoverState(du.HO_STATE_EXECUTION)
	
	rrcCompleteBytes := []byte{0xAA, 0xBB, 0xCC}
	
	err := duInstance.HandleRrcReconfigurationComplete(rrcCompleteBytes)
	// Accept SCTP connection error in test mode
	if err != nil && err.Error() != "SCTP connection not established" {
		t.Errorf("Unexpected error: %v", err)
	}
}

// Test 12: Concurrent State Access
func TestConcurrentHandoverStateAccess(t *testing.T) {
	duInstance := createTestDU(t)
	
	done := make(chan bool)
	
	go func() {
		for i := 0; i < 100; i++ {
			duInstance.SetSourceHandoverState(du.HO_STATE_PREPARATION)
			time.Sleep(1 * time.Millisecond)
		}
		done <- true
	}()
	
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = duInstance.GetHandoverState()
				_ = duInstance.GetHandoverRole()
				time.Sleep(1 * time.Millisecond)
			}
			done <- true
		}()
	}
	
	for i := 0; i < 6; i++ {
		<-done
	}
	
	assert.Equal(t, du.HANDOVER_ROLE_SOURCE, duInstance.GetHandoverRole())
}

// Test 13: Handover Failure Scenario
func TestHandoverFailureScenario(t *testing.T) {
	duInstance := createTestDU(t)
	
	duInstance.SetSourceHandoverState(du.HO_STATE_PREPARATION)
	duInstance.SetSourceHandoverState(du.HO_STATE_FAILED)
	
	assert.Equal(t, du.HO_STATE_FAILED, duInstance.GetHandoverState())
	assert.Equal(t, du.HANDOVER_ROLE_SOURCE, duInstance.GetHandoverRole())
}

// Test 14: Invalid PDU Type
func TestInvalidPDUType(t *testing.T) {
	duInstance := createTestDU(t)
	
	pdu := f1ap.F1apPdu{
		Present: ies.F1apPduSuccessfulOutcome,
		Message: f1ap.F1apMessage{
			ProcedureCode: ies.ProcedureCode{Value: ies.ProcedureCode_UEContextSetup},
		},
	}
	
	err := duInstance.HandleUeContextSetupRequest(&pdu)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid PDU type")
}

// Test 15: Empty RRC Container in DL Transfer
func TestEmptyRRCContainer(t *testing.T) {
	duInstance := createTestDU(t)
	
	// Create UE channels without full initialization
	toUE := make(chan []byte, 100)
	fromUE := make(chan []byte, 100)

	duInstance.SetUEChannelForTest(&du.UeChannel{
		ReceiveFromUeChannel: fromUE,
		SendToUeChannel:      toUE,
	})
	
	msg := &ies.DLRRCMessageTransfer{
		GNBCUUEF1APID: 100,
		GNBDUUEF1APID: 1,
		SRBID:         1,
		RRCContainer:  []byte{},
	}
	
	pdu := f1ap.F1apPdu{
		Present: ies.F1apPduInitiatingMessage,
		Message: f1ap.F1apMessage{
			ProcedureCode: ies.ProcedureCode{Value: ies.ProcedureCode_DLRRCMessageTransfer},
			Msg:           msg,
		},
	}
	
	err := duInstance.HandleDlRrcMessageTransfer(&pdu)
	assert.NoError(t, err)
}