package uecontext

import (
	"context"
	"crypto/rand"
	"fmt"

	"du_ue/pkg/config"

	"github.com/lvdund/asn1go/aper"
	"github.com/lvdund/rrc"
	rrcies "github.com/lvdund/rrc/ies"
)

// InitUE initializes a UE context with channels and executes initial RRC setup.
// It blocks until RRCSetup is received and RRCSetupComplete is sent.
// DU target calls mgr.HandoverUEToDU(msin, targetDUID) when it receives
// UE Context Setup Request from CU-CP, which injects the new channel into UE.
func InitUE(toUE, fromUE chan []byte, ue_config config.UEConfig) *UeContext {
	ue := CreateUe(ue_config, context.Background())

	// Wire the provided channels into a DUConnection so the rest of the
	// code uses the unified DUConnection path instead of raw channel aliases.
	conn := &DUConnection{
		duID:          ue_config.DUID,
		ReceiveFromDu: toUE,
		SendToDu:      fromUE,
		IsReady:       make(chan bool, 1),
	}
	conn.ctx, conn.cancel = context.WithCancel(context.Background())

	ue.connMu.Lock()
	ue.setActiveConn(conn)
	ue.connMu.Unlock()

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
	if err := ue.handleRRCSetup(rrcSetupBytes, ue_config); err != nil {
		ue.Error("Failed to handle RRCSetup: %v", err)
		return nil
	}

	return ue
}

func generateRandomValue(numBits uint) ([]byte, error) {
	numBytes := (numBits + 7) / 8
	randomBytes := make([]byte, numBytes)

	if _, err := rand.Read(randomBytes); err != nil {
		return nil, fmt.Errorf("failed to generate random value: %w", err)
	}

	if extraBits := numBits % 8; extraBits != 0 {
		mask := byte(0xFF << (8 - extraBits))
		randomBytes[numBytes-1] &= mask
	}

	return randomBytes, nil
}

// InitRRCConn sends RRCSetupRequest with random UE identity.
func (ue *UeContext) InitRRCConn() error {
	ue.Info("Initializing RRC connection")

	randomBytes, err := generateRandomValue(39)
	if err != nil {
		return fmt.Errorf("failed to generate UE identity: %w", err)
	}

	rrcSetupRequest := rrcies.RRCSetupRequest{
		RrcSetupRequest: rrcies.RRCSetupRequest_IEs{
			Ue_Identity: rrcies.InitialUE_Identity{
				Choice: rrcies.InitialUE_Identity_Choice_RandomValue,
				RandomValue: aper.BitString{
					Bytes:   randomBytes,
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
		return fmt.Errorf("failed to encode RRCSetupRequest: %w", err)
	}

	ue.Info("Sending RRCSetupRequest to DU (UE identity: %x)", randomBytes)
	if err := ue.sendToActiveDU(encoded); err != nil {
		return fmt.Errorf("failed to send RRCSetupRequest: %w", err)
	}

	return nil
}

func extractTransactionId(rrcSetup *rrcies.RRCSetup) uint64 {
	if rrcSetup != nil {
		return rrcSetup.Rrc_TransactionIdentifier.Value
	}
	return 0
}

// handleRRCSetup handles RRCSetup message received from DU.
func (ue *UeContext) handleRRCSetup(rrcSetupBytes []byte, ue_config config.UEConfig) error {
	ue.Info("Handling RRCSetup message, length: %d bytes", len(rrcSetupBytes))

	rrcMsg, err := rrc.DecodeAny(rrcSetupBytes)
	if err != nil {
		return fmt.Errorf("failed to decode RRC message: %w", err)
	}

	if rrcMsg.Type != rrc.MessageContainerTypeDL_CCCH {
		return fmt.Errorf("expected DL-CCCH message, got: %v", rrcMsg.Type)
	}

	msg := rrcMsg.Message.(*rrcies.DL_CCCH_Message)

	if msg.Message.Choice != rrcies.DL_CCCH_MessageType_Choice_C1 {
		return fmt.Errorf("unsupported DL-CCCH choice type: %v", msg.Message.Choice)
	}

	c1 := msg.Message.C1
	if c1 == nil {
		return fmt.Errorf("DL-CCCH C1 is nil")
	}

	if c1.Choice != rrcies.DL_CCCH_MessageType_C1_Choice_RrcSetup {
		return fmt.Errorf("expected RRCSetup message, got choice: %v", c1.Choice)
	}

	if c1.RrcSetup == nil {
		return fmt.Errorf("RRCSetup is nil")
	}

	transactionId := extractTransactionId(c1.RrcSetup)
	ue.Info("Received RRCSetup from DU (transaction ID: %d)", transactionId)

	ue.auth.snn = []byte(deriveSNN(ue.mcc, ue.mnc))

	if err := ue.TriggerInitRegistration(); err != nil {
		return fmt.Errorf("failed to trigger registration: %w", err)
	}
	ue.Info("Created NAS Registration Request, length: %d bytes", len(ue.nasPdu))

	rrcSetupCompleteIEs := &rrcies.RRCSetupComplete_IEs{
		SelectedPLMN_Identity: 1,
		DedicatedNAS_Message: rrcies.DedicatedNAS_Message{
			Value: ue.nasPdu,
		},
	}

	if ue.guti != nil {
		ue.Info("Including S-TMSI in RRCSetupComplete (resume/re-registration)")
		stmsiPart2 := aper.BitString{
			Bytes:   []byte{0x00, 0x00},
			NumBits: 9,
		}
		rrcSetupCompleteIEs.Ng_5G_S_TMSI_Value = &rrcies.RRCSetupComplete_IEs_ng_5G_S_TMSI_Value{
			Choice:             rrcies.RRCSetupComplete_IEs_ng_5G_S_TMSI_Value_Choice_Ng_5G_S_TMSI_Part2,
			Ng_5G_S_TMSI_Part2: stmsiPart2,
		}
	} else {
		ue.Info("Initial registration - no S-TMSI included")
	}

	rrcSetupComplete := rrcies.RRCSetupComplete{
		Rrc_TransactionIdentifier: rrcies.RRC_TransactionIdentifier{Value: transactionId},
		CriticalExtensions: rrcies.RRCSetupComplete_CriticalExtensions{
			Choice:           rrcies.RRCSetupComplete_CriticalExtensions_Choice_RrcSetupComplete,
			RrcSetupComplete: rrcSetupCompleteIEs,
		},
	}

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
		return fmt.Errorf("failed to encode RRCSetupComplete: %w", err)
	}

	ue.Info("Sending RRCSetupComplete to DU (with NAS Registration Request embedded)")
	if err := ue.sendToActiveDU(encoded); err != nil {
		return fmt.Errorf("failed to send RRCSetupComplete: %w", err)
	}

	ue.Info("==== RRC connection Initialized ====")

	ue.connMu.RLock()
	conn := ue.activeDUConn
	ue.connMu.RUnlock()
	go ue.runRrcReceiver(conn)

	return nil
}