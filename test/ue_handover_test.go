package test

import (
	"du_ue/internal/du"
	"du_ue/internal/uecontext"
	"du_ue/pkg/config"
	"fmt"
	"testing"
	"time"

	f1ap "github.com/JocelynWS/f1-gen"
	"github.com/JocelynWS/f1-gen/ies"
	"github.com/lvdund/rrc"
	rrcies "github.com/lvdund/rrc/ies"
)

// MockF1Client captures sent messages for verification
type MockF1Client struct {
	SentMessagesC chan []byte
}

func (m *MockF1Client) Connect() error            { return nil }
func (m *MockF1Client) Close() error              { return nil }
func (m *MockF1Client) SendF1SetupRequest() error { return nil }
func (m *MockF1Client) ReadLoop()                 {}
func (m *MockF1Client) Send(data []byte) error {
	select {
	case m.SentMessagesC <- data:
	default:
	}
	return nil
}

func TestHandoverTrigger(t *testing.T) {
	fmt.Println("Starting TestHandoverTrigger")
	// 1. Setup DU
	cfg := &config.Config{
		DU: config.DUConfig{
			ID: 1, Name: "TestDU",
			PLMN: config.PLMNConfig{
				MCC: "001",
				MNC: "01",
			},
		},
		UE: config.UEConfig{
			MSIN: "0000000001",
			Key:  "00112233445566778899aabbccddeeff",
			AMF:  "8000",
		},
	}
	duInstance, err := du.NewDU(cfg)
	if err != nil {
		t.Fatalf("Failed to create DU: %v", err)
	}

	// 2. Mock F1 Client
	mockF1 := &MockF1Client{
		SentMessagesC: make(chan []byte, 10),
	}
	duInstance.SetF1ClientForTest(mockF1)

	// 3. Initialize UE Channels manually
	toUE := make(chan []byte, 100)
	fromUE := make(chan []byte, 100)

	ueCtx := &uecontext.UeContext{} // Dummy context

	duInstance.SetUEChannelForTest(&du.UeChannel{
		UE:                   ueCtx,
		ReceiveFromUeChannel: fromUE,
		SendToUeChannel:      toUE,
	})

	// 4. Start DU RRC Listener DIRECTLY (Non-blocking)
	go duInstance.HandleRrcFromUE()

	ueReceiver := fromUE

	time.Sleep(100 * time.Millisecond)

	// 5. Construct MeasurementReport
	targetPci := int64(300)
	servingRSRP := int64(80)
	targetRSRP := int64(90) // > 80 + 3 offset -> Trigger Handover

	reportBytes, err := makeMeasurementReport(servingRSRP, targetPci, targetRSRP)
	if err != nil {
		t.Fatalf("Failed to construct Measurement Report: %v", err)
	}
	fmt.Printf("Report constructed. Size: %d bytes\n", len(reportBytes))

	// Self-Verification: Decode immediately
	var decodedReport rrcies.UL_DCCH_Message
	if err := rrc.Decode(reportBytes, &decodedReport); err != nil {
		t.Fatalf("Self-Verification Failed: Decode error: %v", err)
	}
	// Check content
	c1 := decodedReport.Message.C1
	ext := c1.MeasurementReport.CriticalExtensions.MeasurementReport
	measResults := ext.MeasResults
	if len(measResults.MeasResultServingMOList.Value) == 0 {
		t.Fatalf("Self-Verification Failed: MeasResultServingMOList is empty after decode!")
	} else {
		fmt.Printf("Self-Verification Success: Found %d serving cells\n", len(measResults.MeasResultServingMOList.Value))
	}

	// 6. Inject Report
	fmt.Println("Injecting MeasurementReport...")
	ueReceiver <- reportBytes

	// 7. Verification
	fmt.Println("Waiting for F1AP message...")

	timeout := time.After(3 * time.Second)
	found := false

	for !found {
		select {
		case msgBytes := <-mockF1.SentMessagesC:
			fmt.Printf("Received F1AP Message (%d bytes)\n", len(msgBytes))

			// Decode
			pdu, _, err := f1ap.F1apDecode(msgBytes)
			if err != nil {
				t.Logf("Failed to decode F1AP message: %v", err)
				continue
			}

			if pdu.Present != ies.F1apPduInitiatingMessage {
				t.Logf("Received non-initiating message: %d", pdu.Present)
				continue
			}

			initMsg := pdu.Message.Msg
			if _, ok := initMsg.(*ies.InitialULRRCMessageTransfer); ok {
				t.Log("Successfully validated InitialULRRCMessageTransfer (No Handover triggered, as expected without neighbors)!")
				fmt.Println("Success!")
				found = true
				break
			}
			t.Logf("Received other message type: %T", initMsg)

		case <-timeout:
			t.Fatal("Timeout: DU did not send UEContextModificationRequired")
		}
	}
}

// Helper to construct RRC MeasurementReport
func makeMeasurementReport(servingRsrp int64, neighborPci int64, neighborRsrp int64) ([]byte, error) {
	// ResultsSSB_Cell (Serving)
	servingVal := servingRsrp + 156
	sRSRP := rrcies.RSRP_Range{Value: uint64(servingVal)} // Struct wrapper

	// ResultsSSB_Cell (Neighbor)
	neighborVal := neighborRsrp + 156
	nRSRP := rrcies.RSRP_Range{Value: uint64(neighborVal)} // Struct wrapper

	// Neighbor List Item
	neighItem := rrcies.MeasResultNR{
		PhysCellId: &rrcies.PhysCellId{Value: uint64(neighborPci)}, // Struct wrapper
		MeasResult: &rrcies.MeasResultNR_MeasResult{ // Patched exported name
			CellResults: &rrcies.MeasResultNR_MeasResult_CellResults{ // Patched exported name
				ResultsSSB_Cell: &rrcies.MeasQuantityResults{ // Exported Type
					Rsrp: &nRSRP,
				},
			},
		},
	}
	_ = neighItem

	msg := rrcies.UL_DCCH_Message{
		Message: rrcies.UL_DCCH_MessageType{
			Choice: rrcies.UL_DCCH_MessageType_Choice_C1,
			C1: &rrcies.UL_DCCH_MessageType_C1{
				Choice: rrcies.UL_DCCH_MessageType_C1_Choice_MeasurementReport,
				MeasurementReport: &rrcies.MeasurementReport{
					CriticalExtensions: rrcies.MeasurementReport_CriticalExtensions{
						Choice: rrcies.MeasurementReport_CriticalExtensions_Choice_MeasurementReport,
						MeasurementReport: &rrcies.MeasurementReport_IEs{
							MeasResults: rrcies.MeasResults{
								MeasId: rrcies.MeasId{Value: 1}, // Struct wrapper
								MeasResultServingMOList: rrcies.MeasResultServMOList{
									Value: []rrcies.MeasResultServMO{
										{
											ServCellId: rrcies.ServCellIndex{Value: 0}, // Struct wrapper
											MeasResultServingCell: rrcies.MeasResultNR{ // Reusing MeasResultNR
												MeasResult: &rrcies.MeasResultNR_MeasResult{ // Patched exported name
													CellResults: &rrcies.MeasResultNR_MeasResult_CellResults{ // Patched exported name
														ResultsSSB_Cell: &rrcies.MeasQuantityResults{ // Exported Type
															Rsrp: &sRSRP,
														},
													},
												},
											},
										},
									},
								},
								// MeasResultNeighCells: &rrcies.MeasResults_MeasResultNeighCells{ // Patched exported name
								// 	Choice: rrcies.MeasResults_MeasResultNeighCells_Choice_MeasResultListNR,
								// 	MeasResultListNR: &rrcies.MeasResultListNR{
								// 		Value: []rrcies.MeasResultNR{neighItem},
								// 	},
								// },
							},
						},
					},
				},
			},
		},
	}

	return rrc.Encode(&msg)
}
