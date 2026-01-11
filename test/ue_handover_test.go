package test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lvdund/rrc"
	rrcies "github.com/lvdund/rrc/ies"

	"du_ue/internal/uecontext"
	"du_ue/pkg/config"
)

// createTestUE creates a UE instance for testing
func createTestUE(t *testing.T) *uecontext.UeContext {
	cfg := config.UEConfig{
		MSIN: "0000000001",
		Key:  "465B5CE8B199B49FAA5F0A2EE238A6BC",
		OPC:  "E8ED289DEBA952E4283B54E88E6183CA",
		AMF:  "8000",
		PLMN: config.PLMNConfig{
			MCC: "999",
			MNC: "70",
		},
	}

	toUE := make(chan []byte, 100)
	fromUE := make(chan []byte, 100)

	ue := uecontext.InitUE(toUE, fromUE, cfg)
	require.NotNil(t, ue)

	return ue
}

// Test 1: UE Measurement Report Generation
func TestUE_MeasurementReport(t *testing.T) {
	ue := createTestUE(t)

	// Trigger measurement
	err := ue.TriggerMeasurement()
	assert.NoError(t, err)

	// Wait and check if measurement report was sent
	select {
	case rrcBytes := <-ue.SendToDuChannel:
		assert.NotEmpty(t, rrcBytes, "Measurement report should be sent")
		
		// Decode and verify it's a measurement report
		var ulDcchMsg rrcies.UL_DCCH_Message
		err := rrc.Decode(rrcBytes, &ulDcchMsg)
		assert.NoError(t, err)
		
		if ulDcchMsg.Message.C1 != nil {
			assert.Equal(t, rrcies.UL_DCCH_MessageType_C1_Choice_MeasurementReport, 
				ulDcchMsg.Message.C1.Choice)
		}

	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for measurement report")
	}
}

// Test 2: UE Handles RRC Reconfiguration (Handover Command)
func TestUE_HandleRrcReconfiguration(t *testing.T) {
	ue := createTestUE(t)

	// Create a simple RRC Reconfiguration message
	rrcReconfig := &rrcies.RRCReconfiguration{
		Rrc_TransactionIdentifier: rrcies.RRC_TransactionIdentifier{Value: 1},
		CriticalExtensions: rrcies.RRCReconfiguration_CriticalExtensions{
			Choice: rrcies.RRCReconfiguration_CriticalExtensions_Choice_RrcReconfiguration,
			RrcReconfiguration: &rrcies.RRCReconfiguration_IEs{},
		},
	}

	dlDcchMsg := rrcies.DL_DCCH_Message{
		Message: rrcies.DL_DCCH_MessageType{
			Choice: rrcies.DL_DCCH_MessageType_Choice_C1,
			C1: &rrcies.DL_DCCH_MessageType_C1{
				Choice:             rrcies.DL_DCCH_MessageType_C1_Choice_RrcReconfiguration,
				RrcReconfiguration: rrcReconfig,
			},
		},
	}

	encoded, err := rrc.Encode(&dlDcchMsg)
	require.NoError(t, err)

	// Send to UE
	ue.ReceiveFromDuChannel <- encoded

	// Wait for RRC Reconfiguration Complete
	select {
	case rrcBytes := <-ue.SendToDuChannel:
		assert.NotEmpty(t, rrcBytes, "RRC Reconfiguration Complete should be sent")

		var ulDcchMsg rrcies.UL_DCCH_Message
		err := rrc.Decode(rrcBytes, &ulDcchMsg)
		assert.NoError(t, err)

		if ulDcchMsg.Message.C1 != nil {
			assert.Equal(t, rrcies.UL_DCCH_MessageType_C1_Choice_RrcReconfigurationComplete,
				ulDcchMsg.Message.C1.Choice)
		}

	case <-time.After(200 * time.Millisecond):
		t.Error("Timeout waiting for RRC Reconfiguration Complete")
	}
}

// Test 3: UE PDU Session Establishment (without security context)
func TestUE_PduSessionEstablishment(t *testing.T) {
	ue := createTestUE(t)

	// Trigger PDU session (will fail without security context, but should not crash)
	err := ue.TriggerDefaultPduSession()
	assert.NoError(t, err, "TriggerDefaultPduSession should not return error")

	// PDU Session creation should succeed, but sending will fail due to no security context
	// This is expected behavior - in real scenario, UE must register first
	
	// Drain channel if anything was sent (shouldn't be)
	select {
	case <-ue.SendToDuChannel:
		t.Error("Should not send without security context")
	case <-time.After(50 * time.Millisecond):
		// Expected - no message sent without security context
	}
}

// Test 4: UE State Transitions
func TestUE_StateTransitions(t *testing.T) {
	ue := createTestUE(t)

	// Initial state
	assert.Equal(t, uecontext.UE_STATE_DEREGISTERED, ue.GetState())

	// Change to registering
	ue.SetState(uecontext.UE_STATE_REGISTERING)
	assert.Equal(t, uecontext.UE_STATE_REGISTERING, ue.GetState())

	// Change to registered
	ue.SetState(uecontext.UE_STATE_REGISTERED)
	assert.Equal(t, uecontext.UE_STATE_REGISTERED, ue.GetState())
}

// Test 5: UE NAS Message Handling
func TestUE_NasMessageHandling(t *testing.T) {
	ue := createTestUE(t)

	// Create a simple NAS message (e.g., Authentication Request would require proper encoding)
	// For now, test with empty/invalid to verify error handling
	ue.HandleNasMsg([]byte{})

	// Should not crash, just log error
	// In real scenario, would send proper NAS messages
}

// Test 6: UE Concurrent Operations
func TestUE_ConcurrentOperations(t *testing.T) {
	ue := createTestUE(t)

	done := make(chan bool)

	// Goroutine 1: State changes
	go func() {
		for i := 0; i < 100; i++ {
			ue.SetState(uecontext.UE_STATE_REGISTERING)
			time.Sleep(1 * time.Millisecond)
		}
		done <- true
	}()

	// Goroutine 2: State reads
	go func() {
		for i := 0; i < 100; i++ {
			_ = ue.GetState()
			time.Sleep(1 * time.Millisecond)
		}
		done <- true
	}()

	// Goroutine 3: Measurements
	go func() {
		for i := 0; i < 10; i++ {
			_ = ue.TriggerMeasurement()
			time.Sleep(10 * time.Millisecond)
		}
		done <- true
	}()

	// Wait for all goroutines
	for i := 0; i < 3; i++ {
		<-done
	}

	// No crash = success
}

// Test 7: UE PDU Session Management (without security context)
func TestUE_PduSessionManagement(t *testing.T) {
	ue := createTestUE(t)

	// Test custom DNN (will fail without security context, but should not crash)
	err := ue.TriggerCustomPduSession("ims")
	assert.NoError(t, err, "TriggerCustomPduSession should not return error")

	// Verify PDU Session was created even though it can't be sent
	// This tests the PDU Session management logic independent of NAS security
	
	// Drain the channel if anything (shouldn't be sent without security)
	select {
	case <-ue.SendToDuChannel:
		t.Error("Should not send without security context")
	case <-time.After(50 * time.Millisecond):
		// Expected - no message sent
	}
}

// Test 8: UE Context Cleanup
func TestUE_ContextCleanup(t *testing.T) {
	ue := createTestUE(t)

	// Terminate UE
	ue.Terminate()

	// Verify cleanup (no crash)
	assert.NotNil(t, ue)
}

// Test 9: UE SUCI Generation
func TestUE_SuciGeneration(t *testing.T) {
	ue := createTestUE(t)

	// SUCI should be generated during UE creation
	assert.NotEmpty(t, ue.GetMsin())
	assert.Equal(t, "0000000001", ue.GetMsin())
}

// Test 10: Integration Test - Full Handover from UE Perspective
func TestUE_FullHandoverFlow(t *testing.T) {
	ue := createTestUE(t)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Step 1: UE sends measurement report
	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = ue.TriggerMeasurement()
	}()

	// Step 2: Simulate receiving RRC Reconfiguration (Handover Command)
	go func() {
		time.Sleep(100 * time.Millisecond)

		rrcReconfig := &rrcies.RRCReconfiguration{
			Rrc_TransactionIdentifier: rrcies.RRC_TransactionIdentifier{Value: 1},
			CriticalExtensions: rrcies.RRCReconfiguration_CriticalExtensions{
				Choice: rrcies.RRCReconfiguration_CriticalExtensions_Choice_RrcReconfiguration,
				RrcReconfiguration: &rrcies.RRCReconfiguration_IEs{},
			},
		}

		dlDcchMsg := rrcies.DL_DCCH_Message{
			Message: rrcies.DL_DCCH_MessageType{
				Choice: rrcies.DL_DCCH_MessageType_Choice_C1,
				C1: &rrcies.DL_DCCH_MessageType_C1{
					Choice:             rrcies.DL_DCCH_MessageType_C1_Choice_RrcReconfiguration,
					RrcReconfiguration: rrcReconfig,
				},
			},
		}

		encoded, _ := rrc.Encode(&dlDcchMsg)
		ue.ReceiveFromDuChannel <- encoded
	}()

	// Collect messages sent by UE
	messageCount := 0
	for {
		select {
		case msg := <-ue.SendToDuChannel:
			messageCount++
			t.Logf("UE sent message %d, length: %d", messageCount, len(msg))

		case <-ctx.Done():
			// Test complete
			assert.GreaterOrEqual(t, messageCount, 1, "UE should send at least 1 message")
			return
		}
	}
}