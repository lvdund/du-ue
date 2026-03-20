package du

import (
	"du_ue/internal/uecontext/sec"
	"encoding/binary"
	"reflect"
	"time"
	"unsafe"

	f1ap "github.com/JocelynWS/f1-gen"
	"github.com/JocelynWS/f1-gen/ies"
	"github.com/lvdund/ngap/aper"
	"github.com/lvdund/rrc"
	rrcies "github.com/lvdund/rrc/ies"
	"github.com/reogac/nas"
)

// interceptIncomingForMockCu intercepts incoming F1AP messages from DU (destined for CU) or synthetic ones.
func (c *F1APClient) interceptIncomingForMockCu(data []byte) {
	pdu, err, _ := f1ap.F1apDecode(data)
	if err != nil {
		c.Debug("[MOCK-CU] Failed to decode incoming (to CU) F1AP message: %v", err)
		return
	}

	if pdu.Message.Msg == nil {
		return
	}

	switch pdu.Message.ProcedureCode.Value {
	case ies.ProcedureCode_InitialULRRCMessageTransfer:
		// We avoid proactive mock setup if a real CU-CP is active to prevent ID conflicts.
		// standalone_mock mode could be added to config to enable this.
		c.Debug("[MOCK-CU] Intercepted InitialULRRCMessageTransfer, skipping proactive setup to favor real CU")
		/*
			if msg, ok := pdu.Message.Msg.(*ies.InitialULRRCMessageTransfer); ok {
				c.mockUeContextSetupRequest(msg)
			}
		*/

	case ies.ProcedureCode_UEContextModificationRequired:
		if msg, ok := pdu.Message.Msg.(*ies.UEContextModificationRequired); ok {
			// Trigger mock response for ALL instances of Procedure 8 to bypass incomplete external CU (Problem 2)
			c.Info("[MOCK-CU] Intercepted UEContextModificationRequired, triggering mock UEContextModificationRequest internally")
			
			// Mark UE as "Captured" by Mock CU to enable follow-up interception (Problem 1)
			if ctx := c.du.ueMgr.GetContextByDuId(msg.GNBDUUEF1APID); ctx != nil {
				ctx.MockCuActive = true
				c.Debug("[MOCK-CU] UE %d is now handled by Mock CU (sticky)", ctx.DuUeF1apId)
			}
			go c.mockUeContextModificationRequest(msg)
		}

	case ies.ProcedureCode_ULRRCMessageTransfer:
		if msg, ok := pdu.Message.Msg.(*ies.ULRRCMessageTransfer); ok {
			// Find context to check if it's captured
			ctx := c.du.ueMgr.GetContextByDuId(msg.GNBDUUEF1APID)

			// Intercept if captured (sticky) or if it's a mock session (ID >= 1000)
			if (ctx != nil && ctx.MockCuActive) || (msg.GNBCUUEF1APID >= 1000) {
				c.Info("[MOCK-CU] Intercepted ULRRCMessageTransfer (sticky/mock), checking for NAS SM requests")
				c.handleMockCuUlRrcMessageTransfer(msg)
			} else {
				c.Debug("[MOCK-CU] Allowing ULRRCMessageTransfer for real CU session (ID: %d)", msg.GNBCUUEF1APID)
			}
		}
	}
}

// interceptOutgoingForMockCu intercepts outgoing F1AP messages from DU (destined for real CU-CP).
// Returns true if the message should be blocked.
func (c *F1APClient) interceptOutgoingForMockCu(data []byte) bool {
	// First, run incoming interception (handles triggers for Mock CU)
	c.interceptIncomingForMockCu(data)

	pdu, err, _ := f1ap.F1apDecode(data)
	if err != nil {
		return false
	}

	// Block outgoing messages if they belong to a Mock CU session (ID >= 1000)
	// This prevents sending mock responses to the real CU-CP.
	if pdu.Present == ies.F1apPduSuccessfulOutcome {
		var cuUeF1apId int64
		switch pdu.Message.ProcedureCode.Value {
		case ies.ProcedureCode_UEContextSetup:
			if resp, ok := pdu.Message.Msg.(*ies.UEContextSetupResponse); ok {
				cuUeF1apId = resp.GNBCUUEF1APID
			}
		case ies.ProcedureCode_UEContextModification:
			if resp, ok := pdu.Message.Msg.(*ies.UEContextModificationConfirm); ok {
				cuUeF1apId = resp.GNBCUUEF1APID
			}
		}

		if cuUeF1apId >= 1000 {
			c.Info("[MOCK-CU] Blocking mock response (CU-UE-ID: %d) from reaching real CU-CP", cuUeF1apId)
			return true
		}
	}

	// Procedure 8 (Modification Required) is a trigger. 
	// Always block and handle internally to resolve "unknown procedure code {8}" on external CU.
	if pdu.Present == ies.F1apPduInitiatingMessage && pdu.Message.ProcedureCode.Value == ies.ProcedureCode_UEContextModificationRequired {
		c.Info("[MOCK-CU] Blocking Procedure 8 from reaching real CU; will handle internally")
		return true
	}

	// For Success Outcomes (like UE Context Modification Response) or subsequent UL RRC,
	// block if the UE is in "Mock CU Sticky" mode.
	var duUeF1apId int64
	if pdu.Present == ies.F1apPduSuccessfulOutcome {
		switch pdu.Message.ProcedureCode.Value {
		case ies.ProcedureCode_UEContextSetup:
			if resp, ok := pdu.Message.Msg.(*ies.UEContextSetupResponse); ok {
				duUeF1apId = resp.GNBDUUEF1APID
			}
		case ies.ProcedureCode_UEContextModification:
			if resp, ok := pdu.Message.Msg.(*ies.UEContextModificationResponse); ok {
				duUeF1apId = resp.GNBDUUEF1APID
			}
		}
	} else if pdu.Present == ies.F1apPduInitiatingMessage && pdu.Message.ProcedureCode.Value == ies.ProcedureCode_ULRRCMessageTransfer {
		if msg, ok := pdu.Message.Msg.(*ies.ULRRCMessageTransfer); ok {
			duUeF1apId = msg.GNBDUUEF1APID
		}
	}

	if duUeF1apId > 0 {
		if ctx := c.du.ueMgr.GetContextByDuId(duUeF1apId); ctx != nil && ctx.MockCuActive {
			c.Info("[MOCK-CU] Blocking message for sticky UE %d (handled by Mock CU)", duUeF1apId)
			return true
		}
	}

	return false
}

// getMockSnssaiFor5QI derives S-NSSAI from 5QI based on configuration mappings.
func (c *F1APClient) getMockSnssaiFor5QI(fiveQi int64) (sst, sd string) {
	mockCfg := c.du.Config.MockCU
	for _, mapping := range mockCfg.SnssaiMap {
		if fiveQi >= mapping.Min5QI && fiveQi <= mapping.Max5QI {
			return mapping.SST, mapping.SD
		}
	}
	return "1", "000000" // Final fallback
}

// mockUeContextSetupRequest generates a synthetic UE Context Setup Request.
func (c *F1APClient) mockUeContextSetupRequest(initialUlRrcMsg *ies.InitialULRRCMessageTransfer) {
	duUeF1apId := initialUlRrcMsg.GNBDUUEF1APID
	mockCfg := c.du.Config.MockCU

	// Simulate CU processing delay
	time.Sleep(200 * time.Millisecond)

	c.Info("[MOCK-CU] Generating mock UEContextSetupRequest for DU-UE-ID: %d", duUeF1apId)

	ctx := c.du.ueMgr.GetContextByDuId(duUeF1apId)
	if ctx == nil {
		c.Error("[MOCK-CU] UE Context not found for DU-UE-ID: %d", duUeF1apId)
		return
	}

	// Use existing CU-UE-ID if available, otherwise generate synthetic one
	cuUeF1apId := ctx.CuUeF1apId
	if cuUeF1apId == 0 {
		cuUeF1apId = duUeF1apId + mockCfg.UeIdStart
	}

	// 1. Placeholder RRC message (empty). No longer send premature PDU session accept.
	rrcMsg := []byte{}

	// Allocate TEID (Dynamic)
	teid, _ := c.du.resourceMgr.AllocateTEID()
	gtpTeid := make([]byte, 4)
	binary.BigEndian.PutUint32(gtpTeid, teid)

	tunnelAddress, _ := IPToBitString(mockCfg.TunnelIP)

	// Build the F1AP message
	msg := ies.UEContextSetupRequest{
		GNBCUUEF1APID: cuUeF1apId,
		GNBDUUEF1APID: &duUeF1apId,
		ServCellIndex: 0,
		SpCellID: ies.NRCGI{
			PLMNIdentity: ConvertMccMncToPlmn(c.du.Config.PLMN.MCC, c.du.Config.PLMN.MNC),
			NRCellIdentity: aper.BitString{
				Bytes:   []byte{0x0F, 0xFF, 0xFF, 0xFF, 0xFF},
				NumBits: 36,
			},
		},
		SRBsToBeSetupList: []ies.SRBsToBeSetupItem{
			{SRBID: 1},
			{SRBID: 2},
		},
		DRBsToBeSetupList: []ies.DRBsToBeSetupItem{
			{
				DRBID: 1,
				QoSInformation: ies.QoSInformation{
					Choice: ies.QoSInformationPresentEUTRANQoS,
					EUTRANQoS: &ies.EUTRANQoS{
						QCI: mockCfg.Default5QI,
						AllocationAndRetentionPriority: ies.AllocationAndRetentionPriority{
							PriorityLevel:           1,
							PreEmptionCapability:    ies.PreEmptionCapability{Value: ies.PreEmptionCapabilityShallnottriggerpreemption},
							PreEmptionVulnerability: ies.PreEmptionVulnerability{Value: ies.PreEmptionVulnerabilityNotpreemptable},
						},
					},
				},
				ULUPTNLInformationToBeSetupList: []ies.ULUPTNLInformationToBeSetupItem{
					{
						ULUPTNLInformation: ies.UPTransportLayerInformation{
							Choice: ies.UPTransportLayerInformationPresentGTPTunnel,
							GTPTunnel: &ies.GTPTunnel{
								TransportLayerAddress: tunnelAddress,
								GTPTEID:               gtpTeid,
							},
						},
					},
				},
				RLCMode: ies.RLCMode{Value: aper.Enumerated(mockCfg.DefaultRLC)},
			},
		},
		CUtoDURRCInformation: &ies.CUtoDURRCInformation{
			CGConfigInfo: []byte{0x00},
		},
		RRCContainer: rrcMsg,
		PC5LinkAMBR:  1000000000, // 1 Gbps
		ConditionalInterDUMobilityInformation: &ies.ConditionalInterDUMobilityInformation{
			CHOTrigger: ies.CHOTriggerInterDU{Value: ies.CHOtriggerInterDUChoinitiation},
		},
	}

	// Encode and dispatch
	data, err := f1ap.F1apEncode(&msg)
	if err != nil {
		c.Error("[MOCK-CU] Failed to encode mock UEContextSetupRequest: %v", err)
		return
	}

	c.DispatchMockCuPdu(data)
}

// mockUeContextModificationRequest generates a synthetic UE Context Modification Request.
func (c *F1APClient) mockUeContextModificationRequest(msgRequired *ies.UEContextModificationRequired) {
	cuUeF1apId := msgRequired.GNBCUUEF1APID
	duUeF1apId := msgRequired.GNBDUUEF1APID

	// Consolidate context lookup and ID synchronization
	ctx := c.du.ueMgr.GetContextByDuId(duUeF1apId)
	if ctx == nil {
		c.Error("[MOCK-CU] UE Context not found for DU-UE-ID: %d", duUeF1apId)
		return
	}

	// Sync CU-UE-ID: Use existing if available (especially if from real CU-CP), otherwise dummy
	if ctx.CuUeF1apId != 0 && (cuUeF1apId == 0 || cuUeF1apId >= 1000) {
		cuUeF1apId = ctx.CuUeF1apId
	}
	
	mockCfg := c.du.Config.MockCU

	// Simulate CU processing delay
	time.Sleep(200 * time.Millisecond)

	c.Info("[MOCK-CU] Generating mock UEContextModificationRequest for DU-UE-ID: %d", duUeF1apId)

	// 1. Construct NAS PDU Session Release Command (Session ID 1)
	pduId := uint8(1)
	smRelease := new(nas.PduSessionReleaseCommand)
	smRelease.SetPti(1)
	smRelease.SetSessionId(pduId)
	smRelease.GsmCause = 0x24 // Regular deactivation

	smBytes, err := nas.EncodeSm(smRelease)
	if err != nil {
		c.Error("[MOCK-CU] Failed to encode NAS SM Release: %v", err)
		return
	}

	dlNas := &nas.DlNasTransport{
		PayloadContainerType: nas.PayloadContainerTypeN1SMInfo,
		PayloadContainer:     smBytes,
		PduSessionId:         &pduId,
	}

	mmBytes := c.applyNasSecurity(ctx, dlNas)

	// 3. Wrap in DLInformationTransfer RRC message
	rrcMsg := c.wrapInDlInformationTransfer(mmBytes)

	// Build the F1AP message
	msg := ies.UEContextModificationRequest{
		GNBCUUEF1APID: cuUeF1apId,
		GNBDUUEF1APID: duUeF1apId,
		RRCContainer:  rrcMsg,
		// Mirror DRB release requests from the DU
		DRBsToBeReleasedList: make([]ies.DRBsToBeReleasedItem, 0),
		// Mandatory placeholder fields in the f1-gen library (using config dummy id)
		ExecuteDuplication: &ies.ExecuteDuplication{Value: ies.ExecuteDuplicationTrue},
		PC5LinkAMBR:        1000000000,
		ConditionalIntraDUMobilityInformation: &ies.ConditionalIntraDUMobilityInformation{
			ChoTriggerIntraDU: ies.CHOtriggerIntraDU{Value: ies.CHOtriggerIntraDUChoinitiation},
		},
	}

	// Mirror DRBs to be released if any
	if len(msgRequired.DRBsRequiredToBeReleasedList) > 0 {
		for _, item := range msgRequired.DRBsRequiredToBeReleasedList {
			// Skip dummy items (using config dummy id)
			if item.DRBID == mockCfg.DummyID {
				continue
			}
			msg.DRBsToBeReleasedList = append(msg.DRBsToBeReleasedList, ies.DRBsToBeReleasedItem{
				DRBID: item.DRBID,
			})
		}
	}

	// Mirror SRBs to be released if any
	if len(msgRequired.SRBsRequiredToBeReleasedList) > 0 {
		msg.SRBsToBeReleasedList = make([]ies.SRBsToBeReleasedItem, 0)
		for _, item := range msgRequired.SRBsRequiredToBeReleasedList {
			if item.SRBID == mockCfg.DummyID {
				continue
			}
			msg.SRBsToBeReleasedList = append(msg.SRBsToBeReleasedList, ies.SRBsToBeReleasedItem{
				SRBID: item.SRBID,
			})
		}
	}

	// [DEBUG] Log IDs
	c.Info("[MOCK-CU] Mocking UEContextModificationRequest: CU-ID=%d, DU-ID=%d", cuUeF1apId, duUeF1apId)

	// Encode and dispatch
	data, err := f1ap.F1apEncode(&msg)
	if err != nil {
		c.Error("[MOCK-CU] Failed to encode mock UEContextModificationRequest: %v", err)
		return
	}

	c.DispatchMockCuPdu(data)
}

func (c *F1APClient) handleMockCuUlRrcMessageTransfer(msg *ies.ULRRCMessageTransfer) {
	if msg.SRBID != 2 {
		return
	}

	duUeF1apId := msg.GNBDUUEF1APID
	cuUeF1apId := msg.GNBCUUEF1APID

	// 1. Decode RRC
	var ulDcchMsg rrcies.UL_DCCH_Message
	if err := rrc.Decode(msg.RRCContainer, &ulDcchMsg); err != nil {
		return
	}

	c1 := ulDcchMsg.Message.C1
	if c1 == nil || c1.Choice != rrcies.UL_DCCH_MessageType_C1_Choice_UlInformationTransfer || c1.UlInformationTransfer == nil {
		return
	}

	nasBytes := c1.UlInformationTransfer.CriticalExtensions.UlInformationTransfer.DedicatedNAS_Message.Value
	if len(nasBytes) < 3 {
		return
	}

	// 2. Decode NAS (MM/SM)
	nasMsg, err := nas.Decode(nil, nasBytes, true)
	if err != nil {
		// Handle ciphered plaintext for NEA0 (same as du.detectPduSessionRequest)
		if nasBytes[0] == nas.EPD_5GMM && (nasBytes[1]&0x0F) != nas.NasSecNone && len(nasBytes) > 7 {
			nasMsg, err = nas.Decode(nil, nasBytes[7:], true)
		}
		if err != nil {
			return
		}
	}

	if nasMsg.Gmm != nil && nasMsg.Gmm.UlNasTransport != nil {
		trans := nasMsg.Gmm.UlNasTransport
		if uint8(trans.PayloadContainerType) == nas.PayloadContainerTypeN1SMInfo && trans.PduSessionId != nil {
			pduId := *trans.PduSessionId
			payload := trans.PayloadContainer
			if len(payload) > 3 {
				msgType := payload[3]
				switch msgType {
				case nas.PduSessionEstablishmentRequestMsgType:
					c.Info("[MOCK-CU] UE-initiated PDU Session Establishment Request (Session %d)", pduId)
					go c.mockPduSessionEstablishmentAcceptF1(cuUeF1apId, duUeF1apId, pduId)
				case nas.PduSessionReleaseRequestMsgType:
					c.Info("[MOCK-CU] UE-initiated PDU Session Release Request (Session %d)", pduId)
					go c.mockPduSessionReleaseCommandF1(cuUeF1apId, duUeF1apId, pduId)
				}
			}
		}
	}
}

// mockPduSessionEstablishmentAcceptF1 sends a UE Context Modification Request with NAS Accept.
func (c *F1APClient) mockPduSessionEstablishmentAcceptF1(cuUeF1apId, duUeF1apId int64, pduId uint8) {
	time.Sleep(500 * time.Millisecond)

	ctx := c.du.ueMgr.GetContextByDuId(duUeF1apId)
	if ctx == nil {
		return
	}

	// Use existing CU-UE-ID if available to stay in sync with real CU-CP
	if ctx.CuUeF1apId != 0 {
		cuUeF1apId = ctx.CuUeF1apId
	}

	// 1. Construct NAS PDU Session Establishment Accept
	smAccept := new(nas.PduSessionEstablishmentAccept)
	smAccept.SetPti(1)
	smAccept.SetSessionId(pduId)
	smAccept.SelectedPduSessionType = nas.PduSessionTypeIpv4
	smAccept.SelectedSscMode = 1
	smAccept.AuthorizedQosRules = nas.QosRules{
		Bytes: []byte{0x00, 0x01, 0x01, 0x01, 0x00, 0x07, 0x06, 0x01, 0x04, 0x05, 0x06, 0x07, 0x08},
	}
	smAccept.SessionAmbr = *nas.NewSessionAmbr(nas.SessionAMBRUnit1Mbps, 100, nas.SessionAMBRUnit1Mbps, 100)

	smBytes, _ := nas.EncodeSm(smAccept)
	dlNas := &nas.DlNasTransport{
		PayloadContainerType: nas.PayloadContainerTypeN1SMInfo,
		PayloadContainer:     smBytes,
		PduSessionId:         &pduId,
	}

	mmBytes := c.applyNasSecurity(ctx, dlNas)
	rrcMsg := c.wrapInDlInformationTransfer(mmBytes)

	// 2. Build F1AP UE Context Modification Request
	// Note: We use Modification Request to add the DRB for the new session.
	teid, _ := c.du.resourceMgr.AllocateTEID()
	gtpTeid := make([]byte, 4)
	binary.BigEndian.PutUint32(gtpTeid, teid)

	tunnelAddress, _ := IPToBitString(c.du.Config.MockCU.TunnelIP)

	msg := ies.UEContextModificationRequest{
		GNBCUUEF1APID: cuUeF1apId,
		GNBDUUEF1APID: duUeF1apId,
		DRBsToBeSetupModList: []ies.DRBsToBeSetupModItem{
			{
				DRBID: int64(pduId), // Simple mapping
				QoSInformation: ies.QoSInformation{
					Choice: ies.QoSInformationPresentEUTRANQoS,
					EUTRANQoS: &ies.EUTRANQoS{
						QCI: c.du.Config.MockCU.Default5QI,
						AllocationAndRetentionPriority: ies.AllocationAndRetentionPriority{
							PriorityLevel: 1,
						},
					},
				},
				ULUPTNLInformationToBeSetupList: []ies.ULUPTNLInformationToBeSetupItem{
					{
						ULUPTNLInformation: ies.UPTransportLayerInformation{
							Choice: ies.UPTransportLayerInformationPresentGTPTunnel,
							GTPTunnel: &ies.GTPTunnel{
								TransportLayerAddress: tunnelAddress,
								GTPTEID:               gtpTeid,
							},
						},
					},
				},
				RLCMode: ies.RLCMode{Value: aper.Enumerated(c.du.Config.MockCU.DefaultRLC)},
			},
		},
		RRCContainer: rrcMsg,
		// Mandatory placeholder fields
		ExecuteDuplication: &ies.ExecuteDuplication{Value: ies.ExecuteDuplicationTrue},
		PC5LinkAMBR:        1000000000,
		ConditionalIntraDUMobilityInformation: &ies.ConditionalIntraDUMobilityInformation{
			ChoTriggerIntraDU: ies.CHOtriggerIntraDU{Value: ies.CHOtriggerIntraDUChoinitiation},
		},
	}

	data, err := f1ap.F1apEncode(&msg)
	if err != nil {
		c.Error("[SHIM] Failed to encode mock UEContextModificationRequest (Establishment): %v", err)
		return
	}
	c.DispatchMockCuPdu(data)
}

// mockPduSessionReleaseCommandF1 sends a UE Context Modification Request with NAS Release Command.
func (c *F1APClient) mockPduSessionReleaseCommandF1(cuUeF1apId, duUeF1apId int64, pduId uint8) {
	time.Sleep(500 * time.Millisecond)

	ctx := c.du.ueMgr.GetContextByDuId(duUeF1apId)
	if ctx == nil {
		return
	}

	// Use existing CU-UE-ID if available to stay in sync with real CU-CP
	if ctx.CuUeF1apId != 0 {
		cuUeF1apId = ctx.CuUeF1apId
	}

	// 1. Construct NAS PDU Session Release Command
	smRelease := new(nas.PduSessionReleaseCommand)
	smRelease.SetPti(1)
	smRelease.SetSessionId(pduId)
	smRelease.GsmCause = 0x24 // Regular deactivation

	smBytes, _ := nas.EncodeSm(smRelease)
	dlNas := &nas.DlNasTransport{
		PayloadContainerType: nas.PayloadContainerTypeN1SMInfo,
		PayloadContainer:     smBytes,
		PduSessionId:         &pduId,
	}

	mmBytes := c.applyNasSecurity(ctx, dlNas)
	rrcMsg := c.wrapInDlInformationTransfer(mmBytes)

	// 2. Build F1AP UE Context Modification Request to release bearer
	msg := ies.UEContextModificationRequest{
		GNBCUUEF1APID: cuUeF1apId,
		GNBDUUEF1APID: duUeF1apId,
		DRBsToBeReleasedList: []ies.DRBsToBeReleasedItem{
			{
				DRBID: int64(pduId),
			},
		},
		RRCContainer: rrcMsg,
		// Mandatory placeholder fields
		ExecuteDuplication: &ies.ExecuteDuplication{Value: ies.ExecuteDuplicationTrue},
		PC5LinkAMBR:        1000000000,
		ConditionalIntraDUMobilityInformation: &ies.ConditionalIntraDUMobilityInformation{
			ChoTriggerIntraDU: ies.CHOtriggerIntraDU{Value: ies.CHOtriggerIntraDUChoinitiation},
		},
	}

	data, err := f1ap.F1apEncode(&msg)
	if err != nil {
		c.Error("[MOCK-CU] Failed to encode mock UEContextModificationRequest (Release): %v", err)
		return
	}
	c.DispatchMockCuPdu(data)
}

// applyNasSecurity wraps a NAS message with necessary security headers, accessing the UE context via reflection.
func (c *F1APClient) applyNasSecurity(ctx *DuUeContext, msg *nas.DlNasTransport) []byte {
	// Use reflection to access the unexported secCtx field of UeContext
	ueVal := reflect.ValueOf(ctx.UeChannel.UE).Elem()
	secCtxField := ueVal.FieldByName("secCtx")

	var secCtx *sec.SecurityContext
	if secCtxField.IsValid() {
		secCtxPtr := unsafe.Pointer(secCtxField.UnsafeAddr())
		secCtx = *(**sec.SecurityContext)(secCtxPtr)
	}

	var amfNasCtx *nas.NasContext
	if secCtx != nil {
		msg.SetSecurityHeader(nas.NasSecBoth)
		ueNasCtx := secCtx.NasContext(true)
		encAlg, intAlg := ueNasCtx.SelectedAlgorithms()
		amfNasCtx = nas.NewNasContext(true)
		_ = amfNasCtx.DeriveKeys(encAlg, intAlg, secCtx.Kamf())
	} else {
		msg.SetSecurityHeader(nas.NasSecNone)
	}

	mmBytes, _ := nas.EncodeMm(amfNasCtx, msg, true)
	return mmBytes
}

// wrapInDlInformationTransfer wraps NAS bytes in an RRC DLInformationTransfer message.
func (c *F1APClient) wrapInDlInformationTransfer(nasBytes []byte) []byte {
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
								Value: nasBytes,
							},
						},
					},
				},
			},
		},
	}

	encodedRrc, _ := rrc.Encode(&dlDcchMessage)
	return encodedRrc
}
