package uecontext

import (
	"context"
	"du_ue/pkg/config"
	"fmt"

	"github.com/lvdund/asn1go/aper"
	"github.com/lvdund/rrc"
	rrcies "github.com/lvdund/rrc/ies"
)

func InitUE(toUE, fromUE chan []byte, ue_config config.UEConfig) *UeContext {
	// Channel mapping:
	// toUE = DU -> UE (DU sends to UE, UE receives from DU)
	// fromUE = UE -> DU (UE sends to DU, DU receives from UE)

	ue := CreateUe(ue_config, context.Background())

	ue.ReceiveFromDuChannel = toUE // UE receives RRC messages from DU
	ue.SendToDuChannel = fromUE    // UE sends RRC messages to DU

	// Send RRCSetupRequest to DU
	if err := ue.InitRRCConn(); err != nil {
		ue.Error("Failed to initialize RRC connection: %v", err)
		return nil
	}

	// Block waiting for RRCSetup from DU
	ue.Info("Waiting for RRCSetup from DU...")
	rrcSetupBytes, ok := <-ue.ReceiveFromDuChannel
	if !ok {
		ue.Error("ReceiveFromDuChannel closed while waiting for RRCSetup")
		return nil
	}

	// Decode and handle RRCSetup
	if err := ue.handleRRCSetup(rrcSetupBytes); err != nil {
		ue.Error("Failed to handle RRCSetup: %v", err)
		return nil
	}

	return ue
}

// listenForRrcMessages runs in a goroutine to continuously listen for RRC messages from DU
func (ue *UeContext) listenForRrcMessages() {
	ue.Info("Started listening for RRC messages from DU")
	for {
		select {
		case rrcMessageBytes, ok := <-ue.ReceiveFromDuChannel:
			if !ok {
				ue.Info("ReceiveFromDuChannel closed, stopping RRC listener")
				return
			}
			if err := ue.HandleRrcMsg(rrcMessageBytes); err != nil {
				ue.Error("Failed to handle RRC message: %v", err)
			}
		case <-ue.ctx.Done():
			ue.Info("Context cancelled, stopping RRC listener")
			return
		}
	}
}

func (ue *UeContext) InitRRCConn() error {
	ue.Info("Initializing RRC connection")

	rrcSetupRequest := rrcies.RRCSetupRequest{
		RrcSetupRequest: rrcies.RRCSetupRequest_IEs{
			Ue_Identity: rrcies.InitialUE_Identity{
				Choice: rrcies.InitialUE_Identity_Choice_RandomValue,
				RandomValue: aper.BitString{
					Bytes:   []byte{0x1A, 0x2B, 0x3C, 0x4D, 0x5E},
					NumBits: 39,
				},
			},
			EstablishmentCause: rrcies.EstablishmentCause{
				Value: rrcies.EstablishmentCause_Enum_mo_Signalling,
			},
			Spare: aper.BitString{
				Bytes:   []byte{0x00},
				NumBits: 1,
			},
		},
	}

	ulccchMessage := rrcies.UL_CCCH_Message{
		Message: rrcies.UL_CCCH_MessageType{
			Choice: rrcies.UL_CCCH_MessageType_Choice_C1,
			C1: &rrcies.UL_CCCH_MessageType_C1{
				Choice:          rrcies.UL_CCCH_MessageType_C1_Choice_RrcSetupRequest,
				RrcSetupRequest: &rrcSetupRequest,
			},
		},
	}

	encoded, err := rrc.Encode(&ulccchMessage)
	if err != nil {
		return err
	}

	ue.Info("Sending RRCSetupRequest to DU")
	ue.SendToDuChannel <- encoded

	return nil
}

// handleRRCSetup handles RRCSetup message received from DU
// This is called directly from InitUE after blocking on ReceiveFromDuChannel
func (ue *UeContext) handleRRCSetup(rrcSetupBytes []byte) error {
	ue.Info("Handling RRCSetup message, length: %d bytes", len(rrcSetupBytes))

	// Decode RRC message
	rrcMsg, err := rrc.DecodeAny(rrcSetupBytes)
	if err != nil {
		ue.Error("Failed to decode RRC message: %v", err)
		return err
	}

	// Check if it's DL-CCCH message
	if rrcMsg.Type != rrc.MessageContainerTypeDL_CCCH {
		ue.Error("Expected DL-CCCH message, got: %v", rrcMsg.Type)
		return fmt.Errorf("unexpected RRC message type")
	}

	msg := rrcMsg.Message.(*rrcies.DL_CCCH_Message)

	// Check message structure
	if msg.Message.Choice != rrcies.DL_CCCH_MessageType_Choice_C1 {
		ue.Error("DL-CCCH message has unsupported choice type: %v", msg.Message.Choice)
		return fmt.Errorf("unsupported DL-CCCH choice type")
	}

	c1 := msg.Message.C1
	if c1 == nil {
		ue.Error("DL-CCCH C1 is nil")
		return fmt.Errorf("DL-CCCH C1 is nil")
	}

	// Check if it's RRCSetup message
	if c1.Choice != rrcies.DL_CCCH_MessageType_C1_Choice_RrcSetup {
		ue.Error("DL-CCCH message is not RRCSetup, choice: %v", c1.Choice)
		return fmt.Errorf("expected RRCSetup message")
	}

	if c1.RrcSetup == nil {
		ue.Error("RRCSetup is nil")
		return fmt.Errorf("RRCSetup is nil")
	}

	ue.Info("Received RRCSetup from DU")

	// Handle RRCSetup: prepare for registration
	ue.auth.snn = []byte(deriveSNN(ue.mcc, ue.mnc))

	// Trigger registration to create NAS Registration Request
	if err := ue.TriggerInitRegistration(); err != nil {
		ue.Error("Failed to trigger registration: %v", err)
		return err
	}
	ue.Info("Created NAS Registration Request, length: %d bytes", len(ue.nasPdu))

	// Send RRCSetupComplete with NAS Registration Request embedded
	rrcSetupComplete := rrcies.RRCSetupComplete{
		Rrc_TransactionIdentifier: rrcies.RRC_TransactionIdentifier{Value: 0},
		CriticalExtensions: rrcies.RRCSetupComplete_CriticalExtensions{
			Choice: rrcies.RRCSetupComplete_CriticalExtensions_Choice_RrcSetupComplete,
			RrcSetupComplete: &rrcies.RRCSetupComplete_IEs{
				SelectedPLMN_Identity: 1,
				Ng_5G_S_TMSI_Value: &rrcies.RRCSetupComplete_IEs_ng_5G_S_TMSI_Value{
					Choice: rrcies.RRCSetupComplete_IEs_ng_5G_S_TMSI_Value_Choice_Ng_5G_S_TMSI_Part2,
					Ng_5G_S_TMSI_Part2: aper.BitString{
						Bytes:   []byte{0x00, 0x80},
						NumBits: 9,
					},
				},
				DedicatedNAS_Message: rrcies.DedicatedNAS_Message{
					Value: ue.nasPdu, // NAS Registration Request is embedded here
				},
			},
		},
	}

	// Encode RRCSetupComplete as UL-DCCH message
	uldccchMessage := rrcies.UL_DCCH_Message{
		Message: rrcies.UL_DCCH_MessageType{
			Choice: rrcies.UL_DCCH_MessageType_Choice_C1,
			C1: &rrcies.UL_DCCH_MessageType_C1{
				Choice:           rrcies.UL_DCCH_MessageType_C1_Choice_RrcSetupComplete,
				RrcSetupComplete: &rrcSetupComplete,
			},
		},
	}

	encoded, err := rrc.Encode(&uldccchMessage)
	if err != nil {
		ue.Error("Failed to encode RRCSetupComplete: %v", err)
		return err
	}

	ue.Info("Sending RRCSetupComplete to DU (with NAS Registration Request embedded)")
	ue.SendToDuChannel <- encoded

	// Signal that RRC connection is ready (this unblocks <-ue.IsReadyConn in InitUE)
	// ue.IsReadyConn <- true

	ue.Info("==== RRC connection Initialized ====")

	// Start goroutine to listen for subsequent RRC messages from DU (DL-DCCH messages)
	go ue.listenForRrcMessages()

	return nil
}
