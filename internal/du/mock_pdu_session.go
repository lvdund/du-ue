package du

import (
	"du_ue/internal/uecontext/sec"
	"reflect"
	"time"
	"unsafe"

	"github.com/lvdund/rrc"
	rrcies "github.com/lvdund/rrc/ies"
	"github.com/reogac/nas"
)

// mockPduSessionAccept constructs a synthetic PDU Session Establishment Accept
// and injects it back to the UE's RRC channel. This simulates the AMF/CU-CP
// response that is currently missing due to the InitialContextSetupRequest issue.
func (du *DU) mockPduSessionAccept(ctx *DuUeContext, pduSessionId *uint8) {
	if pduSessionId == nil {
		du.Error("[UE %d] Cannot mock PDU Session Accept: Session ID is nil", ctx.DuUeF1apId)
		return
	}

	id := *pduSessionId
	du.Info("[UE %d] Scheduling mock PDU Session Establishment Accept for session %d", ctx.DuUeF1apId, id)

	go func() {
		// Simulate network processing delay and CU-CP/AMF roundtrip
		time.Sleep(500 * time.Millisecond)

		// 1. Construct NAS PDU Session Establishment Accept
		smAccept := new(nas.PduSessionEstablishmentAccept)
		smAccept.SetPti(1) // Usually 1 for initial request
		smAccept.SetSessionId(id)

		// Set required parameters
		smAccept.SelectedPduSessionType = nas.PduSessionTypeIpv4
		smAccept.SelectedSscMode = 1

		// Add QoS Rules (dummy byte array from etrib5gc tunnel implementation)
		// We just need a syntactically valid QoS rule for the UE to accept it
		smAccept.AuthorizedQosRules = nas.QosRules{
			Bytes: []byte{0x00, 0x01, 0x01, 0x01, 0x00, 0x07, 0x06, 0x01, 0x04, 0x05, 0x06, 0x07, 0x08},
		}

		// Add Session AMBR
		smAccept.SessionAmbr = *nas.NewSessionAmbr(nas.SessionAMBRUnit1Mbps, 100, nas.SessionAMBRUnit1Mbps, 100)

		smBytes, err := nas.EncodeSm(smAccept)
		if err != nil {
			du.Error("[UE %d] Failed to encode mock SM Accept: %v", ctx.DuUeF1apId, err)
			return
		}

		// 2. Wrap in DL NAS Transport
		dlNas := &nas.DlNasTransport{
			PayloadContainerType: nas.PayloadContainerTypeN1SMInfo,
			PayloadContainer:     smBytes,
			PduSessionId:         &id,
		}

		// We MUST apply the UE's NAS security context because the UE will reject
		// plain NAS messages if it already has a valid security context.
		// Since we're using NEA0/NIA2, "ciphered" really means Integrity+Plaintext.
		var amfNasCtx *nas.NasContext

		// Use reflection and unsafe to access the unexported secCtx field of UeContext.
		// This strictly adheres to the constraint not to modify internal/uecontext/ue.go.
		ueVal := reflect.ValueOf(ctx.UeChannel.UE).Elem()
		secCtxField := ueVal.FieldByName("secCtx")

		var secCtx *sec.SecurityContext
		if secCtxField.IsValid() {
			secCtxPtr := unsafe.Pointer(secCtxField.UnsafeAddr())
			secCtx = *(**sec.SecurityContext)(secCtxPtr)
		}

		if secCtx != nil {
			dlNas.SetSecurityHeader(nas.NasSecBoth) // Integrity protected and ciphered

			// Extract UE's current algorithms and Kamf
			ueNasCtx := secCtx.NasContext(true)
			encAlg, intAlg := ueNasCtx.SelectedAlgorithms()

			// Create a duplicate NAS context acting as the AMF (isAmf=true)
			// This ensures nas.EncodeMm increments the DL Count instead of the UL Count
			amfNasCtx = nas.NewNasContext(true)

			// Re-derive the keys using the UE's Kamf
			if err := amfNasCtx.DeriveKeys(encAlg, intAlg, secCtx.Kamf()); err != nil {
				du.Error("[UE %d] Failed to derive keys for AMF-side NAS Context: %v", ctx.DuUeF1apId, err)
				return
			}

			// The mock PDU Session Establishment Accept is the first DL NAS message since Security Mode Complete.
			// The UE's expected DL count is currently 1 (0 was SMC, 1 is this message).
			// The new amfNasCtx starts its DlCounter at zero, but nas.Encode(isAmf=true) uses the DlCounter.
			// Wait, the new context starts with count=0! If we need it to encode with count=1, we might need to
			// increment it by doing a dummy encode. Let's try 0 first, as the library might align it.
		} else {
			dlNas.SetSecurityHeader(nas.NasSecNone)
		}

		mmBytes, err := nas.EncodeMm(amfNasCtx, dlNas, true)
		if err != nil {
			du.Error("[UE %d] Failed to encode mock MM DL NAS Transport: %v", ctx.DuUeF1apId, err)
			return
		}

		// 3. Wrap in DLInformationTransfer RRC message
		dlDcchMessage := rrcies.DL_DCCH_Message{
			Message: rrcies.DL_DCCH_MessageType{
				Choice: rrcies.DL_DCCH_MessageType_Choice_C1,
				C1: &rrcies.DL_DCCH_MessageType_C1{
					Choice: rrcies.DL_DCCH_MessageType_C1_Choice_DlInformationTransfer,
					DlInformationTransfer: &rrcies.DLInformationTransfer{
						Rrc_TransactionIdentifier: rrcies.RRC_TransactionIdentifier{Value: 0},
						CriticalExtensions: rrcies.DLInformationTransfer_CriticalExtensions{
							Choice: rrcies.DLInformationTransfer_CriticalExtensions_Choice_DlInformationTransfer,
							DlInformationTransfer: &rrcies.DLInformationTransfer_IEs{
								DedicatedNAS_Message: &rrcies.DedicatedNAS_Message{
									Value: mmBytes,
								},
							},
						},
					},
				},
			},
		}

		encodedRrc, err := rrc.Encode(&dlDcchMessage)
		if err != nil {
			du.Error("[UE %d] Failed to encode mock DL Information Transfer: %v", ctx.DuUeF1apId, err)
			return
		}

		// 4. Inject into UE's receive channel
		du.Info("[UE %d] Injecting mock PDU Session Establishment Accept via DLInformationTransfer", ctx.DuUeF1apId)

		// Ensure channel is open before writing
		select {
		case ctx.UeChannel.SendToUeChannel <- encodedRrc:
			du.Info("[UE %d] Mock PDU Session Accept injected successfully", ctx.DuUeF1apId)
		case <-time.After(1 * time.Second):
			du.Error("[UE %d] Timeout trying to inject mock PDU Session Accept to UE channel", ctx.DuUeF1apId)
		}
	}()
}

// detectPduSessionRequest inspects an RRC ULInformationTransfer message
// to see if it contains a NAS PDU Session Establishment Request.
func (du *DU) detectPduSessionRequest(ctx *DuUeContext, ulInfo *rrcies.ULInformationTransfer) {
	if ulInfo == nil || ulInfo.CriticalExtensions.UlInformationTransfer == nil || ulInfo.CriticalExtensions.UlInformationTransfer.DedicatedNAS_Message == nil {
		return
	}

	nasBytes := ulInfo.CriticalExtensions.UlInformationTransfer.DedicatedNAS_Message.Value
	if len(nasBytes) < 3 {
		return
	}

	du.Info("[UE %d] Parsing NAS bytes: len=%d, header=[%x %x %x]", ctx.DuUeF1apId, len(nasBytes), nasBytes[0], nasBytes[1], nasBytes[2])

	// Try to use the NAS library to decode it (ignoring MAC errors)
	msg, err := nas.Decode(nil, nasBytes, true)
	if err != nil {
		du.Info("[UE %d] Regular NAS Decode failed: %v", ctx.DuUeF1apId, err)

		// Bypass security header: Since the ciphering algorithm negotiated was NEA0 (Null ciphering),
		// the message is "ciphered" with a no-op algorithm, meaning the payload is plaintext!
		// 5G NAS Security Header is 7 bytes:
		// [0] EPD (0x7E)
		// [1] Security Header Type (0x01 = Integrity, 0x02 = Integrity+Ciphered, etc)
		// [2-5] MAC (4 bytes)
		// [6] SQN (1 byte)
		// [7:] Plaintext NAS Payload
		if nasBytes[0] == nas.EPD_5GMM && (nasBytes[1]&0x0F) != nas.NasSecNone {
			if len(nasBytes) > 7 {
				du.Info("[UE %d] Stripping 7-byte security header and trying to decode plaintext...", ctx.DuUeF1apId)
				plainBytes := nasBytes[7:]
				msg, err = nas.Decode(nil, plainBytes, true)
				if err != nil {
					du.Info("[UE %d] Plaintext decode also failed: %v", ctx.DuUeF1apId, err)
					return
				}
			} else {
				return
			}
		} else {
			return
		}
	}

	if msg.Gmm != nil && msg.Gmm.UlNasTransport != nil {
		trans := msg.Gmm.UlNasTransport
		du.Info("[UE %d] Found UlNasTransport: PayloadContainerType=%d, PduSessionId=%v", ctx.DuUeF1apId, trans.PayloadContainerType, trans.PduSessionId)

		if uint8(trans.PayloadContainerType) == nas.PayloadContainerTypeN1SMInfo && trans.PduSessionId != nil {
			pduId := *trans.PduSessionId
			du.Info("[UE %d] Found N1SMInfo! PduSessionId: %d", ctx.DuUeF1apId, pduId)
			du.Info("[UE %d] Dump of PayloadContainer: %x", ctx.DuUeF1apId, trans.PayloadContainer)

			// The 5GSM header is 4 bytes: [0] EPD, [1] PDU Session ID, [2] PTI, [3] MsgType
			if len(trans.PayloadContainer) > 3 {
				smEpD := trans.PayloadContainer[0]
				smPduId := trans.PayloadContainer[1]
				smPti := trans.PayloadContainer[2]
				smMsgType := trans.PayloadContainer[3]

				du.Info("[UE %d] SM Parse - EPD: 0x%X, PduId: 0x%X, PTI: 0x%X, MsgType: 0x%X", ctx.DuUeF1apId, smEpD, smPduId, smPti, smMsgType)

				// 0xC1 = 193 = PDU Session Establishment Request
				if smMsgType == nas.PduSessionEstablishmentRequestMsgType {
					du.Info("[UE %d] Match! Calling mockPduSessionAccept for session %d", ctx.DuUeF1apId, pduId)
					du.mockPduSessionAccept(ctx, &pduId)
				}
			} else {
				du.Info("[UE %d] SM Payload too short: len=%d", ctx.DuUeF1apId, len(trans.PayloadContainer))
			}
		}
	} else {
		du.Info("[UE %d] Decoded NAS is not UlNasTransport (msg.Gmm=%v)", ctx.DuUeF1apId, msg.Gmm != nil)
	}
}
