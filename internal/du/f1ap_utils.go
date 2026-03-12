package du

import (
	"bytes"
	"fmt"
	"net"

	"github.com/JocelynWS/f1-gen/ies"
	"github.com/lvdund/ngap/aper"
)

// IPToBitString deals with conversion of string IP to BitString
func IPToBitString(ipStr string) (aper.BitString, error) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return aper.BitString{}, fmt.Errorf("invalid IP address: %s", ipStr)
	}

	var ipBytes []byte
	if ip4 := ip.To4(); ip4 != nil {
		ipBytes = ip4
	} else {
		ipBytes = ip.To16()
	}

	return aper.BitString{
		Bytes:   ipBytes,
		NumBits: uint64(len(ipBytes) * 8),
	}, nil
}

// EncodeF1APPdu manually encodes an F1AP PDU with specified procedure code and IEs.
// This bypasses the generated library's strict validation and incorrect procedure codes.
func EncodeF1APPdu(present int, procedureCode int64, criticality aper.Enumerated, ieList []ies.F1apMessageIE) ([]byte, error) {
	var buf bytes.Buffer
	ieW := aper.NewWriter(&buf)

	// Encode Sequence of IEs
	// Match library expectations: a bool followed by the sequence of IEs
	ieW.WriteBool(aper.Zero) // Sequence extension or similar padding

	if err := aper.WriteSequenceOf[ies.F1apMessageIE](ieList, ieW, &aper.Constraint{
		Lb: 0,
		Ub: int64(aper.POW_16 - 1),
	}, false); err != nil {
		return nil, err
	}

	ieW.Close()
	return EncodeF1APPduWithPayload(present, procedureCode, criticality, buf.Bytes())
}

// EncodeF1APPduWithPayload manually encodes an F1AP PDU using a pre-encoded payload (sequence of IEs).
func EncodeF1APPduWithPayload(present int, procedureCode int64, criticality aper.Enumerated, payload []byte) ([]byte, error) {
	var buf bytes.Buffer
	aw := aper.NewWriter(&buf)

	// 1. First bit: manual header (Matched to library's encodeMessage)
	if err := aw.WriteBool(aper.Zero); err != nil {
		return nil, err
	}

	// 2. Choice Index
	// Even though encodeMessage uses extensible=true, we'll try matching it.
	// present should be ies.F1apPdu... (1, 2, or 3)
	if err := aw.WriteChoice(uint64(present), 2, true); err != nil {
		return nil, err
	}

	// 3. Procedure Code
	if err := aw.WriteInteger(procedureCode, &aper.Constraint{Lb: 0, Ub: 255}, false); err != nil {
		return nil, err
	}

	// 4. Criticality
	if err := aw.WriteEnumerate(uint64(criticality), aper.Constraint{Lb: 0, Ub: 2}, false); err != nil {
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
func DecodeUEContextModificationRequestIEs(data []byte, msg *ies.UEContextModificationRequest) error {
	r := aper.NewReader(bytes.NewReader(data))

	// 1. PDU Header
	if _, err := r.ReadBool(); err != nil {
		return err
	}
	choice, err := r.ReadChoice(2, false)
	if err != nil {
		return err
	}
	if choice != uint64(ies.F1apPduInitiatingMessage) {
		return fmt.Errorf("not an initiating message")
	}

	// 2. Procedure Code
	code, err := r.ReadInteger(&aper.Constraint{Lb: 0, Ub: 255}, false)
	if err != nil {
		return err
	}
	if code != 7 { // UEContextModification
		return fmt.Errorf("not a UEContextModificationRequest")
	}

	// 3. Criticality
	if _, err := r.ReadEnumerate(aper.Constraint{Lb: 0, Ub: 2}, false); err != nil {
		return err
	}

	// 4. Value (Open Type)
	buf, err := r.ReadOpenType()
	if err != nil {
		return err
	}

	// 5. UEContextModificationRequest SEQUENCE
	ieR := aper.NewReader(bytes.NewReader(buf))
	if _, err := ieR.ReadBool(); err != nil {
		return err
	}

	// 6. Loop through IEs and extract missing ones
	decodeIEFunc := func(argR *aper.AperReader) (msgIe *ies.F1apMessageIE, err error) {
		var id int64
		var c uint64
		var ieBuf []byte
		if id, err = argR.ReadInteger(&aper.Constraint{Lb: 0, Ub: int64(aper.POW_16) - 1}, false); err != nil {
			return
		}
		msgIe = new(ies.F1apMessageIE)
		msgIe.Id.Value = aper.Integer(id)
		if c, err = argR.ReadEnumerate(aper.Constraint{Lb: 0, Ub: 2}, false); err != nil {
			return
		}
		msgIe.Criticality.Value = aper.Enumerated(c)
		if ieBuf, err = argR.ReadOpenType(); err != nil {
			return
		}

		subR := aper.NewReader(bytes.NewReader(ieBuf))
		switch msgIe.Id.Value {
		case ies.ProtocolIEID_RRCContainer:
			tmp := ies.NewOCTETSTRING(nil, aper.Constraint{Lb: 0, Ub: 0}, false)
			if err = tmp.Decode(subR); err == nil {
				msg.RRCContainer = tmp.Value
			}
		case ies.ProtocolIEID_DRBsToBeSetupModList:
			tmp := ies.NewSequence[*ies.DRBsToBeSetupModItem](nil, aper.Constraint{Lb: 1, Ub: 64}, false)
			fn := func() *ies.DRBsToBeSetupModItem { return new(ies.DRBsToBeSetupModItem) }
			if err = tmp.Decode(subR, fn); err == nil {
				msg.DRBsToBeSetupModList = []ies.DRBsToBeSetupModItem{}
				for _, i := range tmp.Value {
					msg.DRBsToBeSetupModList = append(msg.DRBsToBeSetupModList, *i)
				}
			}
		case ies.ProtocolIEID_SRBsToBeSetupModList:
			tmp := ies.NewSequence[*ies.SRBsToBeSetupModItem](nil, aper.Constraint{Lb: 1, Ub: 8}, false)
			fn := func() *ies.SRBsToBeSetupModItem { return new(ies.SRBsToBeSetupModItem) }
			if err = tmp.Decode(subR, fn); err == nil {
				msg.SRBsToBeSetupModList = []ies.SRBsToBeSetupModItem{}
				for _, i := range tmp.Value {
					msg.SRBsToBeSetupModList = append(msg.SRBsToBeSetupModList, *i)
				}
			}
		case ies.ProtocolIEID_ExecuteDuplication:
			var tmp ies.ExecuteDuplication
			if err = tmp.Decode(subR); err == nil {
				msg.ExecuteDuplication = &tmp
			}
		case ies.ProtocolIEID_PC5LinkAMBR:
			tmp := ies.NewINTEGER(0, aper.Constraint{Lb: 0, Ub: 4000000000000}, false)
			if err = tmp.Decode(subR); err == nil {
				msg.PC5LinkAMBR = int64(tmp.Value)
			}
		}
		return msgIe, nil
	}

	_, err = aper.ReadSequenceOf[ies.F1apMessageIE](decodeIEFunc, ieR, &aper.Constraint{Lb: 0, Ub: int64(aper.POW_16 - 1)}, false)
	return err
}
