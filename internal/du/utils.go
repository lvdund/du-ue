package du

import (
	"bytes"
	"fmt"
	"net"

	f1apies "github.com/JocelynWS/f1-gen/ies"
	f1aper "github.com/lvdund/ngap/aper"
	rrcaper "github.com/lvdund/asn1go/aper"
	"github.com/lvdund/rrc"
	rrcies "github.com/lvdund/rrc/ies"
)

// --- F1AP Utilities & Workarounds ---
// Note: These use f1aper (github.com/lvdund/ngap/aper)

// IPToBitString deals with conversion of string IP to BitString
func IPToBitString(ipStr string) (f1aper.BitString, error) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return f1aper.BitString{}, fmt.Errorf("invalid IP address: %s", ipStr)
	}

	var ipBytes []byte
	if ip4 := ip.To4(); ip4 != nil {
		ipBytes = ip4
	} else {
		ipBytes = ip.To16()
	}

	return f1aper.BitString{
		Bytes:   ipBytes,
		NumBits: uint64(len(ipBytes) * 8),
	}, nil
}

// EncodeF1APPdu manually encodes an F1AP PDU with specified procedure code and IEs.
// This bypasses the generated library's strict validation and incorrect procedure codes.
func EncodeF1APPdu(present int, procedureCode int64, criticality f1aper.Enumerated, ieList []f1apies.F1apMessageIE) ([]byte, error) {
	var buf bytes.Buffer
	ieW := f1aper.NewWriter(&buf)

	// Encode Sequence of IEs
	// Match library expectations: a bool followed by the sequence of IEs
	ieW.WriteBool(f1aper.Zero) // Sequence extension or similar padding

	if err := f1aper.WriteSequenceOf[f1apies.F1apMessageIE](ieList, ieW, &f1aper.Constraint{
		Lb: 0,
		Ub: int64(f1aper.POW_16 - 1),
	}, false); err != nil {
		return nil, err
	}

	ieW.Close()
	return EncodeF1APPduWithPayload(present, procedureCode, criticality, buf.Bytes())
}

// EncodeF1APPduWithPayload manually encodes an F1AP PDU using a pre-encoded payload (sequence of IEs).
func EncodeF1APPduWithPayload(present int, procedureCode int64, criticality f1aper.Enumerated, payload []byte) ([]byte, error) {
	var buf bytes.Buffer
	aw := f1aper.NewWriter(&buf)

	// 1. First bit: manual header (Matched to library's encodeMessage)
	if err := aw.WriteBool(f1aper.Zero); err != nil {
		return nil, err
	}

	// 2. Choice Index
	// Even though encodeMessage uses extensible=true, we'll try matching it.
	// present should be f1apies.F1apPdu... (1, 2, or 3)
	if err := aw.WriteChoice(uint64(present), 2, true); err != nil {
		return nil, err
	}

	// 3. Procedure Code
	if err := aw.WriteInteger(procedureCode, &f1aper.Constraint{Lb: 0, Ub: 255}, false); err != nil {
		return nil, err
	}

	// 4. Criticality
	if err := aw.WriteEnumerate(uint64(criticality), f1aper.Constraint{Lb: 0, Ub: 2}, false); err != nil {
		return nil, err
	}

	// 5. Message Payload (Open Type containing Sequence of IEs)
	if err := aw.WriteOpenType(payload); err != nil {
		return nil, err
	}

	aw.Close()
	return buf.Bytes(), nil
}

// DecodeUEContextModificationRequestIEs manually decodes the missing IEs in UEContextModificationRequest.
// This is necessary because the f1-gen library's decoder is missing several mandatory/optional fields.
func DecodeUEContextModificationRequestIEs(data []byte, msg *f1apies.UEContextModificationRequest) error {
	r := f1aper.NewReader(bytes.NewReader(data))

	// 1. PDU Header
	if _, err := r.ReadBool(); err != nil {
		return err
	}
	choice, err := r.ReadChoice(2, false)
	if err != nil {
		return err
	}
	if choice != uint64(f1apies.F1apPduInitiatingMessage) {
		return fmt.Errorf("not an initiating message")
	}

	// 2. Procedure Code
	code, err := r.ReadInteger(&f1aper.Constraint{Lb: 0, Ub: 255}, false)
	if err != nil {
		return err
	}
	if code != 7 { // UEContextModification
		return fmt.Errorf("not a UEContextModificationRequest")
	}

	// 3. Criticality
	if _, err := r.ReadEnumerate(f1aper.Constraint{Lb: 0, Ub: 2}, false); err != nil {
		return err
	}

	// 4. Value (Open Type)
	buf, err := r.ReadOpenType()
	if err != nil {
		return err
	}

	// 5. UEContextModificationRequest SEQUENCE
	ieR := f1aper.NewReader(bytes.NewReader(buf))
	if _, err := ieR.ReadBool(); err != nil {
		return err
	}

	// 6. Loop through IEs and extract missing ones
	decodeIEFunc := func(argR *f1aper.AperReader) (msgIe *f1apies.F1apMessageIE, err error) {
		var id int64
		var c uint64
		var ieBuf []byte
		if id, err = argR.ReadInteger(&f1aper.Constraint{Lb: 0, Ub: int64(f1aper.POW_16) - 1}, false); err != nil {
			return
		}
		msgIe = new(f1apies.F1apMessageIE)
		msgIe.Id.Value = f1aper.Integer(id)
		if c, err = argR.ReadEnumerate(f1aper.Constraint{Lb: 0, Ub: 2}, false); err != nil {
			return
		}
		msgIe.Criticality.Value = f1aper.Enumerated(c)
		if ieBuf, err = argR.ReadOpenType(); err != nil {
			return
		}

		subR := f1aper.NewReader(bytes.NewReader(ieBuf))
		switch msgIe.Id.Value {
		case f1apies.ProtocolIEID_RRCContainer:
			tmp := f1apies.NewOCTETSTRING(nil, f1aper.Constraint{Lb: 0, Ub: 0}, false)
			if err = tmp.Decode(subR); err == nil {
				msg.RRCContainer = tmp.Value
			}
		case f1apies.ProtocolIEID_DRBsToBeSetupModList:
			tmp := f1apies.NewSequence[*f1apies.DRBsToBeSetupModItem](nil, f1aper.Constraint{Lb: 1, Ub: 64}, false)
			fn := func() *f1apies.DRBsToBeSetupModItem { return new(f1apies.DRBsToBeSetupModItem) }
			if err = tmp.Decode(subR, fn); err == nil {
				msg.DRBsToBeSetupModList = []f1apies.DRBsToBeSetupModItem{}
				for _, i := range tmp.Value {
					msg.DRBsToBeSetupModList = append(msg.DRBsToBeSetupModList, *i)
				}
			}
		case f1apies.ProtocolIEID_SRBsToBeSetupModList:
			tmp := f1apies.NewSequence[*f1apies.SRBsToBeSetupModItem](nil, f1aper.Constraint{Lb: 1, Ub: 8}, false)
			fn := func() *f1apies.SRBsToBeSetupModItem { return new(f1apies.SRBsToBeSetupModItem) }
			if err = tmp.Decode(subR, fn); err == nil {
				msg.SRBsToBeSetupModList = []f1apies.SRBsToBeSetupModItem{}
				for _, i := range tmp.Value {
					msg.SRBsToBeSetupModList = append(msg.SRBsToBeSetupModList, *i)
				}
			}
		case f1apies.ProtocolIEID_ExecuteDuplication:
			var tmp f1apies.ExecuteDuplication
			if err = tmp.Decode(subR); err == nil {
				msg.ExecuteDuplication = &tmp
			}
		case f1apies.ProtocolIEID_PC5LinkAMBR:
			tmp := f1apies.NewINTEGER(0, f1aper.Constraint{Lb: 0, Ub: 4000000000000}, false)
			if err = tmp.Decode(subR); err == nil {
				msg.PC5LinkAMBR = int64(tmp.Value)
			}
		}
		return msgIe, nil
	}

	_, err = f1aper.ReadSequenceOf[f1apies.F1apMessageIE](decodeIEFunc, ieR, &f1aper.Constraint{Lb: 0, Ub: int64(f1aper.POW_16 - 1)}, false)
	return err
}

// EncodeUEContextModificationResponse manually encodes the response to workaround a vendor bug where
// Procedure 7 and Procedure 8 successful outcomes are swapped.
func EncodeUEContextModificationResponse(msg *f1apies.UEContextModificationResponse) ([]byte, error) {
	var ieList []f1apies.F1apMessageIE
	intConst := f1aper.Constraint{Lb: 0, Ub: 4294967295}

	// 1. GNBCUUEF1APID
	gnbCuId := f1apies.NewINTEGER(msg.GNBCUUEF1APID, intConst, false)
	ieList = append(ieList, f1apies.F1apMessageIE{
		Id:          f1apies.ProtocolIEID{Value: f1apies.ProtocolIEID_GNBCUUEF1APID},
		Criticality: f1apies.Criticality{Value: f1apies.Criticality_PresentReject},
		Value:       &gnbCuId,
	})

	// 2. GNBDUUEF1APID
	gnbDuId := f1apies.NewINTEGER(msg.GNBDUUEF1APID, intConst, false)
	ieList = append(ieList, f1apies.F1apMessageIE{
		Id:          f1apies.ProtocolIEID{Value: f1apies.ProtocolIEID_GNBDUUEF1APID},
		Criticality: f1apies.Criticality{Value: f1apies.Criticality_PresentReject},
		Value:       &gnbDuId,
	})

	// 3. DUtoCURRCInformation
	if msg.DUtoCURRCInformation != nil {
		ieList = append(ieList, f1apies.F1apMessageIE{
			Id:          f1apies.ProtocolIEID{Value: f1apies.ProtocolIEID_DUtoCURRCInformation},
			Criticality: f1apies.Criticality{Value: f1apies.Criticality_PresentReject},
			Value:       msg.DUtoCURRCInformation,
		})
	}

	// 4. DRBsSetupModList (Mandatory in some library versions)
	drbList := msg.DRBsSetupModList
	if len(drbList) == 0 {
		// Add a dummy entry (ID 1) if empty to satisfy mandatory constraints
		// Trace: DRBsSetupModItem -> DLUPTNLInformationToBeSetupList -> DLUPTNLInformation -> Choice GTPTunnel
		dummyGtp := f1apies.GTPTunnel{
			TransportLayerAddress: f1aper.BitString{
				Bytes:   []byte{192, 168, 1, 1},
				NumBits: 32,
			},
			GTPTEID: []byte{0, 0, 0, 1},
		}
		dummyItem := f1apies.DRBsSetupModItem{
			DRBID: 1,
			DLUPTNLInformationToBeSetupList: []f1apies.DLUPTNLInformationToBeSetupItem{
				{
					DLUPTNLInformation: f1apies.UPTransportLayerInformation{
						Choice: f1apies.UPTransportLayerInformationPresentGTPTunnel,
						GTPTunnel: &dummyGtp,
					},
				},
			},
		}
		drbList = []f1apies.DRBsSetupModItem{dummyItem}
	}
	tmpDRB := f1apies.NewSequence[*f1apies.DRBsSetupModItem](nil, f1aper.Constraint{Lb: 1, Ub: 64}, false)
	for i := range drbList {
		tmpDRB.Value = append(tmpDRB.Value, &drbList[i])
	}
	ieList = append(ieList, f1apies.F1apMessageIE{
		Id:          f1apies.ProtocolIEID{Value: f1apies.ProtocolIEID_DRBsSetupModList},
		Criticality: f1apies.Criticality{Value: f1apies.Criticality_PresentIgnore},
		Value:       &tmpDRB,
	})

	// 5. DRBsModifiedList (Mandatory in some library versions)
	drbModifiedList := msg.DRBsModifiedList
	if len(drbModifiedList) == 0 {
		// Even for Modified, some library versions expect TNL info
		dummyGtp := f1apies.GTPTunnel{
			TransportLayerAddress: f1aper.BitString{
				Bytes:   []byte{192, 168, 1, 1},
				NumBits: 32,
			},
			GTPTEID: []byte{0, 0, 0, 1},
		}
		drbModifiedList = []f1apies.DRBsModifiedItem{
			{
				DRBID: 1,
				DLUPTNLInformationToBeSetupList: []f1apies.DLUPTNLInformationToBeSetupItem{
					{
						DLUPTNLInformation: f1apies.UPTransportLayerInformation{
							Choice: f1apies.UPTransportLayerInformationPresentGTPTunnel,
							GTPTunnel: &dummyGtp,
						},
					},
				},
			},
		}
	}
	tmpDRBMod := f1apies.NewSequence[*f1apies.DRBsModifiedItem](nil, f1aper.Constraint{Lb: 1, Ub: 64}, false)
	for i := range drbModifiedList {
		tmpDRBMod.Value = append(tmpDRBMod.Value, &drbModifiedList[i])
	}
	ieList = append(ieList, f1apies.F1apMessageIE{
		Id:          f1apies.ProtocolIEID{Value: f1apies.ProtocolIEID_DRBsModifiedList},
		Criticality: f1apies.Criticality{Value: f1apies.Criticality_PresentIgnore},
		Value:       &tmpDRBMod,
	})

	// 5b. SRBsSetupModList (Mandatory in some library versions)
	srbList := msg.SRBsSetupModList
	if len(srbList) == 0 {
		srbList = []f1apies.SRBsSetupModItem{{SRBID: 1}}
	}
	tmpSRB := f1apies.NewSequence[*f1apies.SRBsSetupModItem](nil, f1aper.Constraint{Lb: 1, Ub: 8}, false)
	for i := range srbList {
		tmpSRB.Value = append(tmpSRB.Value, &srbList[i])
	}
	ieList = append(ieList, f1apies.F1apMessageIE{
		Id:          f1apies.ProtocolIEID{Value: f1apies.ProtocolIEID_SRBsSetupModList},
		Criticality: f1apies.Criticality{Value: f1apies.Criticality_PresentIgnore},
		Value:       &tmpSRB,
	})

	// 5c. SRBsModifiedList (Mandatory in some library versions)
	srbModifiedList := msg.SRBsModifiedList
	if len(srbModifiedList) == 0 {
		srbModifiedList = []f1apies.SRBsModifiedItem{{SRBID: 1}}
	}
	tmpSRBMod := f1apies.NewSequence[*f1apies.SRBsModifiedItem](nil, f1aper.Constraint{Lb: 1, Ub: 8}, false)
	for i := range srbModifiedList {
		tmpSRBMod.Value = append(tmpSRBMod.Value, &srbModifiedList[i])
	}
	ieList = append(ieList, f1apies.F1apMessageIE{
		Id:          f1apies.ProtocolIEID{Value: f1apies.ProtocolIEID_SRBsModifiedList},
		Criticality: f1apies.Criticality{Value: f1apies.Criticality_PresentIgnore},
		Value:       &tmpSRBMod,
	})

	// 6. BHChannelsSetupModList
	if len(msg.BHChannelsSetupModList) > 0 {
		tmp := f1apies.NewSequence[*f1apies.BHChannelsSetupModItem](nil, f1aper.Constraint{Lb: 1, Ub: 64}, false)
		for i := range msg.BHChannelsSetupModList {
			tmp.Value = append(tmp.Value, &msg.BHChannelsSetupModList[i])
		}
		ieList = append(ieList, f1apies.F1apMessageIE{
			Id:          f1apies.ProtocolIEID{Value: f1apies.ProtocolIEID_BHChannelsSetupModList},
			Criticality: f1apies.Criticality{Value: f1apies.Criticality_PresentIgnore},
			Value:       &tmp,
		})
	}

	// 7. BHChannelsModifiedList
	if len(msg.BHChannelsModifiedList) > 0 {
		tmp := f1apies.NewSequence[*f1apies.BHChannelsModifiedItem](nil, f1aper.Constraint{Lb: 1, Ub: 64}, false)
		for i := range msg.BHChannelsModifiedList {
			tmp.Value = append(tmp.Value, &msg.BHChannelsModifiedList[i])
		}
		ieList = append(ieList, f1apies.F1apMessageIE{
			Id:          f1apies.ProtocolIEID{Value: f1apies.ProtocolIEID_BHChannelsModifiedList},
			Criticality: f1apies.Criticality{Value: f1apies.Criticality_PresentIgnore},
			Value:       &tmp,
		})
	}

	// 8. RequestedTargetCellGlobalID
	if msg.RequestedTargetCellGlobalID != nil {
		ieList = append(ieList, f1apies.F1apMessageIE{
			Id:          f1apies.ProtocolIEID{Value: f1apies.ProtocolIEID_RequestedTargetCellGlobalID},
			Criticality: f1apies.Criticality{Value: f1apies.Criticality_PresentReject},
			Value:       msg.RequestedTargetCellGlobalID,
		})
	}

	// Use Choice 2 (SuccessfulOutcome) and Procedure 7 (UEContextModification)
	return EncodeF1APPdu(2, 7, f1apies.Criticality_PresentReject, ieList)
}

// --- RRC Shim Fixes (Bit-shifting workarounds) ---
// Note: These use rrcaper (github.com/lvdund/asn1go/aper)

// FixRRCMessage attempts to decode an RRC message that was encoded with the library's bit-shifting bug,
// and then re-encodes it correctly for the real CU-CP.
func (du *DU) FixRRCMessage(rrcBytes []byte) []byte {
	// 1. Try to decode as UL-DCCH with the bug-aware decoder
	var ulDcch rrcies.UL_DCCH_Message
	if err := ManualDecodeULDCCH(rrcBytes, &ulDcch); err == nil {
		// Successfully decoded broken message, now re-encode correctly
		fixed, err := ManualEncodeULDCCH(&ulDcch)
		if err == nil {
			du.Info("Fixed bit-shifted UL-DCCH message from UE")
			return fixed
		}
	}

	// 2. Try to decode as UL-CCCH with the bug-aware decoder
	var ulCcch rrcies.UL_CCCH_Message
	if err := ManualDecodeULCCCH(rrcBytes, &ulCcch); err == nil {
		fixed, err := ManualEncodeULCCCH(&ulCcch)
		if err == nil {
			du.Info("Fixed bit-shifted UL-CCCH message from UE")
			return fixed
		}
	}

	return rrcBytes
}

// UnfixRRCMessage attempts to decode a standard-compliant RRC message from the real CU-CP
// and re-encodes it using the buggy library logic so the simulator's UE can read it.
func (du *DU) UnfixRRCMessage(rrcBytes []byte) []byte {
	// 1. Try to decode as DL-DCCH with the manual correct decoder
	var dlDcch rrcies.DL_DCCH_Message
	if err := ManualDecodeDLDCCH(rrcBytes, &dlDcch); err == nil {
		// Successfully decoded correct message, now re-encode with BUGS
		buggy, err := rrc.Encode(&dlDcch)
		if err == nil {
			du.Info("Re-encoded DL-DCCH message with simulator-compatible bugs for UE")
			return buggy
		}
	}

	// 2. Try to decode as DL-CCCH with the manual correct decoder
	var dlCcch rrcies.DL_CCCH_Message
	if err := ManualDecodeDLCCCH(rrcBytes, &dlCcch); err == nil {
		buggy, err := rrc.Encode(&dlCcch)
		if err == nil {
			du.Info("Re-encoded DL-CCCH message with simulator-compatible bugs for UE")
			return buggy
		}
	}

	return rrcBytes
}

// --- Manual RRC Encoders (Correct 3GPP bit-widths) ---

func ManualEncodeULDCCH(msg *rrcies.UL_DCCH_Message) ([]byte, error) {
	var buf bytes.Buffer
	w := rrcaper.NewWriter(&buf)
	if err := EncodeULDCCHMessageType(w, &msg.Message); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func ManualEncodeULCCCH(msg *rrcies.UL_CCCH_Message) ([]byte, error) {
	var buf bytes.Buffer
	w := rrcaper.NewWriter(&buf)
	if err := EncodeULCCCHMessageType(w, &msg.Message); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func ManualEncodeDLDCCH(msg *rrcies.DL_DCCH_Message) ([]byte, error) {
	var buf bytes.Buffer
	w := rrcaper.NewWriter(&buf)
	// CHOICE between 2 items -> uBound = 1 (1 bit)
	if err := w.WriteChoice(msg.Message.Choice, 1, false); err != nil {
		return nil, err
	}
	switch msg.Message.Choice {
	case rrcies.DL_DCCH_MessageType_Choice_C1:
		if err := EncodeDLDCCHMessageTypeC1Correct(w, msg.Message.C1); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("DL-DCCH choice not implemented in manual encoder")
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func ManualEncodeCellGroupConfig(msg *rrcies.CellGroupConfig) ([]byte, error) {
	var buf bytes.Buffer
	w := rrcaper.NewWriter(&buf)
	if err := msg.Encode(w); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func EncodeDLDCCHMessageTypeC1Correct(w *rrcaper.AperWriter, ie *rrcies.DL_DCCH_MessageType_C1) error {
	// CHOICE between 16 items -> uBound = 15 (4 bits)
	if err := w.WriteChoice(ie.Choice, 15, false); err != nil {
		return err
	}
	switch ie.Choice {
	case rrcies.DL_DCCH_MessageType_C1_Choice_RrcReconfiguration:
		return ie.RrcReconfiguration.Encode(w)
	case rrcies.DL_DCCH_MessageType_C1_Choice_DlInformationTransfer:
		return ie.DlInformationTransfer.Encode(w)
	default:
		return ie.Encode(w)
	}
}

func ManualDecodeULDCCHBuggy(data []byte, msg *rrcies.UL_DCCH_Message) error {
	r := rrcaper.NewReader(bytes.NewReader(data))
	// Library uses uBound = 2 for choice of 2 items (Incorrect)
	choice, err := r.ReadChoice(2, false)
	if err != nil {
		return err
	}
	msg.Message.Choice = choice
	switch choice {
	case rrcies.UL_DCCH_MessageType_Choice_C1:
		msg.Message.C1 = new(rrcies.UL_DCCH_MessageType_C1)
		return DecodeULDCCHMessageTypeC1Buggy(r, msg.Message.C1)
	default:
		return fmt.Errorf("choice not handled in buggy decoder")
	}
}

func EncodeULCCCHMessageType(w *rrcaper.AperWriter, ie *rrcies.UL_CCCH_MessageType) error {
	if err := w.WriteChoice(ie.Choice, 1, false); err != nil {
		return err
	}
	switch ie.Choice {
	case rrcies.UL_CCCH_MessageType_Choice_C1:
		return EncodeULCCCHMessageTypeC1(w, ie.C1)
	case rrcies.UL_CCCH_MessageType_Choice_MessageClassExtension:
		return nil
	default:
		return fmt.Errorf("invalid UL_CCCH_MessageType choice: %d", ie.Choice)
	}
}

func EncodeULCCCHMessageTypeC1(w *rrcaper.AperWriter, ie *rrcies.UL_CCCH_MessageType_C1) error {
	if err := w.WriteChoice(ie.Choice, 3, false); err != nil {
		return err
	}
	switch ie.Choice {
	case rrcies.UL_CCCH_MessageType_C1_Choice_RrcSetupRequest:
		return ie.RrcSetupRequest.Encode(w)
	case rrcies.UL_CCCH_MessageType_C1_Choice_RrcResumeRequest:
		return ie.RrcResumeRequest.Encode(w)
	case rrcies.UL_CCCH_MessageType_C1_Choice_RrcReestablishmentRequest:
		return ie.RrcReestablishmentRequest.Encode(w)
	case rrcies.UL_CCCH_MessageType_C1_Choice_RrcSystemInfoRequest:
		return ie.RrcSystemInfoRequest.Encode(w)
	default:
		return fmt.Errorf("invalid UL_CCCH C1 choice: %d", ie.Choice)
	}
}

func EncodeULDCCHMessageType(w *rrcaper.AperWriter, ie *rrcies.UL_DCCH_MessageType) error {
	if err := w.WriteChoice(ie.Choice, 1, false); err != nil {
		return err
	}
	switch ie.Choice {
	case rrcies.UL_DCCH_MessageType_Choice_C1:
		return EncodeULDCCHMessageTypeC1(w, ie.C1)
	case rrcies.UL_DCCH_MessageType_Choice_MessageClassExtension:
		if ie.MessageClassExtension != nil {
			return ie.MessageClassExtension.Encode(w)
		}
		return nil
	default:
		return fmt.Errorf("invalid UL_DCCH_MessageType choice: %d", ie.Choice)
	}
}

func EncodeULDCCHMessageTypeC1(w *rrcaper.AperWriter, ie *rrcies.UL_DCCH_MessageType_C1) error {
	if err := w.WriteChoice(ie.Choice, 15, false); err != nil {
		return err
	}
	switch ie.Choice {
	case rrcies.UL_DCCH_MessageType_C1_Choice_MeasurementReport:
		return EncodeMeasurementReport(w, ie.MeasurementReport)
	case rrcies.UL_DCCH_MessageType_C1_Choice_RrcReconfigurationComplete:
		return EncodeRRCReconfigurationComplete(w, ie.RrcReconfigurationComplete)
	case rrcies.UL_DCCH_MessageType_C1_Choice_UlInformationTransfer:
		return ie.UlInformationTransfer.Encode(w)
	case rrcies.UL_DCCH_MessageType_C1_Choice_RrcSetupComplete:
		return ie.RrcSetupComplete.Encode(w)
	case rrcies.UL_DCCH_MessageType_C1_Choice_SecurityModeComplete:
		return ie.SecurityModeComplete.Encode(w)
	case rrcies.UL_DCCH_MessageType_C1_Choice_SecurityModeFailure:
		return ie.SecurityModeFailure.Encode(w)
	case rrcies.UL_DCCH_MessageType_C1_Choice_UeCapabilityInformation:
		return ie.UeCapabilityInformation.Encode(w)
	default:
		return ie.Encode(w)
	}
}

func EncodeMeasurementReport(w *rrcaper.AperWriter, ie *rrcies.MeasurementReport) error {
	if err := w.WriteChoice(ie.CriticalExtensions.Choice, 1, false); err != nil {
		return err
	}
	switch ie.CriticalExtensions.Choice {
	case rrcies.MeasurementReport_CriticalExtensions_Choice_MeasurementReport:
		return ie.CriticalExtensions.MeasurementReport.Encode(w)
	case rrcies.MeasurementReport_CriticalExtensions_Choice_CriticalExtensionsFuture:
		return nil
	default:
		return fmt.Errorf("invalid MeasurementReport choice: %d", ie.CriticalExtensions.Choice)
	}
}

func EncodeRRCReconfigurationComplete(w *rrcaper.AperWriter, ie *rrcies.RRCReconfigurationComplete) error {
	if err := ie.Rrc_TransactionIdentifier.Encode(w); err != nil {
		return err
	}
	if err := w.WriteChoice(ie.CriticalExtensions.Choice, 1, false); err != nil {
		return err
	}
	switch ie.CriticalExtensions.Choice {
	case rrcies.RRCReconfigurationComplete_CriticalExtensions_Choice_RrcReconfigurationComplete:
		return ie.CriticalExtensions.RrcReconfigurationComplete.Encode(w)
	case rrcies.RRCReconfigurationComplete_CriticalExtensions_Choice_CriticalExtensionsFuture:
		return nil
	default:
		return fmt.Errorf("invalid RRCReconfigurationComplete choice: %d", ie.CriticalExtensions.Choice)
	}
}

// --- Manual RRC Bug-Aware Decoders (Expecting shifted bits from UE simulator) ---

func ManualDecodeULDCCH(data []byte, msg *rrcies.UL_DCCH_Message) error {
	r := rrcaper.NewReader(bytes.NewReader(data))
	choice, err := r.ReadChoice(2, false)
	if err != nil {
		return err
	}
	msg.Message.Choice = choice
	switch choice {
	case rrcies.UL_DCCH_MessageType_Choice_C1:
		msg.Message.C1 = new(rrcies.UL_DCCH_MessageType_C1)
		return DecodeULDCCHMessageTypeC1Buggy(r, msg.Message.C1)
	case rrcies.UL_DCCH_MessageType_Choice_MessageClassExtension:
		msg.Message.MessageClassExtension = new(rrcies.UL_DCCH_MessageType_MessageClassExtension)
		return msg.Message.MessageClassExtension.Decode(r)
	default:
		return fmt.Errorf("invalid choice")
	}
}

func DecodeULDCCHMessageTypeC1Buggy(r *rrcaper.AperReader, ie *rrcies.UL_DCCH_MessageType_C1) error {
	choice, err := r.ReadChoice(16, false)
	if err != nil {
		return err
	}
	ie.Choice = choice
	switch choice {
	case rrcies.UL_DCCH_MessageType_C1_Choice_MeasurementReport:
		ie.MeasurementReport = new(rrcies.MeasurementReport)
		return DecodeMeasurementReportBuggy(r, ie.MeasurementReport)
	case rrcies.UL_DCCH_MessageType_C1_Choice_RrcReconfigurationComplete:
		ie.RrcReconfigurationComplete = new(rrcies.RRCReconfigurationComplete)
		return DecodeRRCReconfigurationCompleteBuggy(r, ie.RrcReconfigurationComplete)
	case rrcies.UL_DCCH_MessageType_C1_Choice_UlInformationTransfer:
		ie.UlInformationTransfer = new(rrcies.ULInformationTransfer)
		return ie.UlInformationTransfer.Decode(r)
	case rrcies.UL_DCCH_MessageType_C1_Choice_RrcSetupComplete:
		ie.RrcSetupComplete = new(rrcies.RRCSetupComplete)
		return ie.RrcSetupComplete.Decode(r)
	case rrcies.UL_DCCH_MessageType_C1_Choice_SecurityModeComplete:
		ie.SecurityModeComplete = new(rrcies.SecurityModeComplete)
		return ie.SecurityModeComplete.Decode(r)
	case rrcies.UL_DCCH_MessageType_C1_Choice_SecurityModeFailure:
		ie.SecurityModeFailure = new(rrcies.SecurityModeFailure)
		return ie.SecurityModeFailure.Decode(r)
	case rrcies.UL_DCCH_MessageType_C1_Choice_UeCapabilityInformation:
		ie.UeCapabilityInformation = new(rrcies.UECapabilityInformation)
		return ie.UeCapabilityInformation.Decode(r)
	default:
		return ie.Decode(r)
	}
}

func DecodeMeasurementReportBuggy(r *rrcaper.AperReader, ie *rrcies.MeasurementReport) error {
	choice, err := r.ReadChoice(2, false)
	if err != nil {
		return err
	}
	ie.CriticalExtensions.Choice = choice
	switch choice {
	case rrcies.MeasurementReport_CriticalExtensions_Choice_MeasurementReport:
		ie.CriticalExtensions.MeasurementReport = new(rrcies.MeasurementReport_IEs)
		return ie.CriticalExtensions.MeasurementReport.Decode(r)
	default:
		return fmt.Errorf("invalid choice")
	}
}

func DecodeRRCReconfigurationCompleteBuggy(r *rrcaper.AperReader, ie *rrcies.RRCReconfigurationComplete) error {
	if err := ie.Rrc_TransactionIdentifier.Decode(r); err != nil {
		return err
	}
	choice, err := r.ReadChoice(2, false)
	if err != nil {
		return err
	}
	ie.CriticalExtensions.Choice = choice
	switch choice {
	case rrcies.RRCReconfigurationComplete_CriticalExtensions_Choice_RrcReconfigurationComplete:
		ie.CriticalExtensions.RrcReconfigurationComplete = new(rrcies.RRCReconfigurationComplete_IEs)
		return ie.CriticalExtensions.RrcReconfigurationComplete.Decode(r)
	default:
		return fmt.Errorf("invalid choice")
	}
}

func ManualDecodeULCCCH(data []byte, msg *rrcies.UL_CCCH_Message) error {
	r := rrcaper.NewReader(bytes.NewReader(data))
	choice, err := r.ReadChoice(2, false)
	if err != nil {
		return err
	}
	msg.Message.Choice = choice
	switch choice {
	case rrcies.UL_CCCH_MessageType_Choice_C1:
		msg.Message.C1 = new(rrcies.UL_CCCH_MessageType_C1)
		return DecodeULCCCHMessageTypeC1Buggy(r, msg.Message.C1)
	default:
		return fmt.Errorf("invalid choice")
	}
}

func DecodeULCCCHMessageTypeC1Buggy(r *rrcaper.AperReader, ie *rrcies.UL_CCCH_MessageType_C1) error {
	choice, err := r.ReadChoice(4, false)
	if err != nil {
		return err
	}
	ie.Choice = choice
	switch choice {
	case rrcies.UL_CCCH_MessageType_C1_Choice_RrcSetupRequest:
		ie.RrcSetupRequest = new(rrcies.RRCSetupRequest)
		return ie.RrcSetupRequest.Decode(r)
	case rrcies.UL_CCCH_MessageType_C1_Choice_RrcResumeRequest:
		ie.RrcResumeRequest = new(rrcies.RRCResumeRequest)
		return ie.RrcResumeRequest.Decode(r)
	case rrcies.UL_CCCH_MessageType_C1_Choice_RrcReestablishmentRequest:
		ie.RrcReestablishmentRequest = new(rrcies.RRCReestablishmentRequest)
		return ie.RrcReestablishmentRequest.Decode(r)
	case rrcies.UL_CCCH_MessageType_C1_Choice_RrcSystemInfoRequest:
		ie.RrcSystemInfoRequest = new(rrcies.RRCSystemInfoRequest)
		return ie.RrcSystemInfoRequest.Decode(r)
	default:
		return ie.Decode(r)
	}
}

// --- Manual Correct RRC Decoders (Expecting correct bits from a real CU-CP) ---

func ManualDecodeDLDCCH(data []byte, msg *rrcies.DL_DCCH_Message) error {
	r := rrcaper.NewReader(bytes.NewReader(data))
	choice, err := r.ReadChoice(1, false)
	if err != nil {
		return err
	}
	msg.Message.Choice = choice
	switch choice {
	case rrcies.DL_DCCH_MessageType_Choice_C1:
		msg.Message.C1 = new(rrcies.DL_DCCH_MessageType_C1)
		return DecodeDLDCCHMessageTypeC1Correct(r, msg.Message.C1)
	default:
		return fmt.Errorf("invalid DL-DCCH choice")
	}
}

func DecodeDLDCCHMessageTypeC1Correct(r *rrcaper.AperReader, ie *rrcies.DL_DCCH_MessageType_C1) error {
	choice, err := r.ReadChoice(15, false)
	if err != nil {
		return err
	}
	ie.Choice = choice
	switch choice {
	case rrcies.DL_DCCH_MessageType_C1_Choice_RrcReconfiguration:
		ie.RrcReconfiguration = new(rrcies.RRCReconfiguration)
		return ie.RrcReconfiguration.Decode(r)
	case rrcies.DL_DCCH_MessageType_C1_Choice_DlInformationTransfer:
		ie.DlInformationTransfer = new(rrcies.DLInformationTransfer)
		return ie.DlInformationTransfer.Decode(r)
	default:
		return ie.Decode(r)
	}
}

func ManualDecodeDLCCCH(data []byte, msg *rrcies.DL_CCCH_Message) error {
	r := rrcaper.NewReader(bytes.NewReader(data))
	choice, err := r.ReadChoice(1, false)
	if err != nil {
		return err
	}
	msg.Message.Choice = choice
	switch choice {
	case rrcies.DL_CCCH_MessageType_Choice_C1:
		msg.Message.C1 = new(rrcies.DL_CCCH_MessageType_C1)
		return DecodeDLCCCHMessageTypeC1Correct(r, msg.Message.C1)
	default:
		return fmt.Errorf("invalid DL-CCCH choice")
	}
}

func DecodeDLCCCHMessageTypeC1Correct(r *rrcaper.AperReader, ie *rrcies.DL_CCCH_MessageType_C1) error {
	choice, err := r.ReadChoice(7, false)
	if err != nil {
		return err
	}
	ie.Choice = choice
	switch choice {
	case rrcies.DL_CCCH_MessageType_C1_Choice_RrcSetup:
		ie.RrcSetup = new(rrcies.RRCSetup)
		return ie.RrcSetup.Decode(r)
	default:
		return ie.Decode(r)
	}
}
