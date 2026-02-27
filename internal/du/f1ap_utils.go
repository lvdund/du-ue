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
func EncodeF1APPdu(procedureCode int64, criticality aper.Enumerated, ieList []ies.F1apMessageIE) ([]byte, error) {
	var buf bytes.Buffer
	aw := aper.NewWriter(&buf)

	// 1. Present: F1apPduSuccessfulOutcome (Index 1)
	// Choice Index (0=Initiating, 1=Successful, 2=Unsuccessful)
	if err := aw.WriteBool(aper.Zero); err != nil { // No extension
		return nil, err
	}
	if err := aw.WriteInteger(1, &aper.Constraint{Lb: 0, Ub: 2}, false); err != nil { // Index 1
		return nil, err
	}

	// 2. Procedure Code
	// Using generic Integer encoding as per Common.go
	if err := aw.WriteInteger(procedureCode, &aper.Constraint{Lb: 0, Ub: 255}, false); err != nil {
		return nil, err
	}

	// 3. Criticality
	if err := aw.WriteEnumerate(uint64(criticality), aper.Constraint{Lb: 0, Ub: 2}, false); err != nil {
		return nil, err
	}

	// 4. Message Payload (Open Type containing Sequence of IEs)
	if len(ieList) == 0 {
		return nil, fmt.Errorf("empty message IEs")
	}

	var ieBuf bytes.Buffer
	ieW := aper.NewWriter(&ieBuf)

	// Encode Sequence of IEs
	ieW.WriteBool(aper.Zero) // Sequence extension

	if err := aper.WriteSequenceOf[ies.F1apMessageIE](ieList, ieW, &aper.Constraint{
		Lb: 0,
		Ub: int64(aper.POW_16 - 1),
	}, false); err != nil {
		return nil, err
	}

	ieW.Close()

	// Write the Open Type container for the whole message
	if err := aw.WriteOpenType(ieBuf.Bytes()); err != nil {
		return nil, err
	}

	aw.Close()
	return buf.Bytes(), nil
}

// BuildUEContextModificationResponseIEs constructs IEs for UEContextModificationResponse,
// omitting optional "mandatory" fields that are not relevant.
func BuildUEContextModificationResponseIEs(msg *ies.UEContextModificationResponse) []ies.F1apMessageIE {
	list := []ies.F1apMessageIE{}

	// 1. GNB-CU-UE-F1AP-ID (Mandatory)
	{
		val := ies.NewINTEGER(msg.GNBCUUEF1APID, aper.Constraint{Lb: 0, Ub: 4294967295}, false)
		list = append(list, ies.F1apMessageIE{
			Id:          ies.ProtocolIEID{Value: ies.ProtocolIEID_GNBCUUEF1APID},
			Criticality: ies.Criticality{Value: ies.Criticality_PresentReject},
			Value:       &val,
		})
	}

	// 2. GNB-DU-UE-F1AP-ID (Mandatory)
	{
		val := ies.NewINTEGER(msg.GNBDUUEF1APID, aper.Constraint{Lb: 0, Ub: 4294967295}, false)
		list = append(list, ies.F1apMessageIE{
			Id:          ies.ProtocolIEID{Value: ies.ProtocolIEID_GNBDUUEF1APID},
			Criticality: ies.Criticality{Value: ies.Criticality_PresentReject},
			Value:       &val,
		})
	}

	// 3. DUtoCURRCInformation (Mandatory)
	if msg.DUtoCURRCInformation != nil {
		list = append(list, ies.F1apMessageIE{
			Id:          ies.ProtocolIEID{Value: ies.ProtocolIEID_DUtoCURRCInformation},
			Criticality: ies.Criticality{Value: ies.Criticality_PresentReject},
			Value:       msg.DUtoCURRCInformation,
		})
	}

	// 4. DRBsSetupModList (Conditional Mandatory in spec, strict in lib)
	if len(msg.DRBsSetupModList) > 0 {
		items := []*ies.DRBsSetupModItem{}
		for _, item := range msg.DRBsSetupModList {
			val := item
			items = append(items, &val)
		}
		// Constraint: Lb: 1, Ub: 64
		seq := ies.NewSequence(items, aper.Constraint{Lb: 1, Ub: 64}, false)
		list = append(list, ies.F1apMessageIE{
			Id:          ies.ProtocolIEID{Value: ies.ProtocolIEID_DRBsSetupModList},
			Criticality: ies.Criticality{Value: ies.Criticality_PresentIgnore},
			Value:       &seq,
		})
	}

	// 5. DRBsModifiedList (Strict Mandatory in lib)
	{
		dummyItem := ies.DRBsModifiedItem{
			DRBID: 1, // Dummy ID
			DLUPTNLInformationToBeSetupList: []ies.DLUPTNLInformationToBeSetupItem{
				{
					// DLUPTNLAddress is NOT a field here.
					DLUPTNLInformation: ies.UPTransportLayerInformation{
						Choice: ies.UPTransportLayerInformationPresentGTPTunnel,
						GTPTunnel: &ies.GTPTunnel{
							TransportLayerAddress: aper.BitString{
								Bytes:   []byte{0x00, 0x00, 0x00, 0x00},
								NumBits: 32,
							},
							GTPTEID: []byte{0x00, 0x00, 0x00, 0x00},
						},
					},
				},
			},
		}
		items := []*ies.DRBsModifiedItem{&dummyItem}
		// Constraint: Lb: 1, Ub: 64
		seq := ies.NewSequence(items, aper.Constraint{Lb: 1, Ub: 64}, false)
		list = append(list, ies.F1apMessageIE{
			Id:          ies.ProtocolIEID{Value: ies.ProtocolIEID_DRBsModifiedList},
			Criticality: ies.Criticality{Value: ies.Criticality_PresentIgnore},
			Value:       &seq,
		})
	}

	// 6. SRBsFailedToBeSetupModList (Optional)
	if len(msg.SRBsFailedToBeSetupModList) > 0 {
		items := []*ies.SRBsFailedToBeSetupModItem{}
		for _, item := range msg.SRBsFailedToBeSetupModList {
			val := item
			items = append(items, &val)
		}
		// Constraint: Lb: 1, Ub: 8
		seq := ies.NewSequence(items, aper.Constraint{Lb: 1, Ub: 8}, false)
		list = append(list, ies.F1apMessageIE{
			Id:          ies.ProtocolIEID{Value: ies.ProtocolIEID_SRBsFailedToBeSetupModList},
			Criticality: ies.Criticality{Value: ies.Criticality_PresentIgnore},
			Value:       &seq,
		})
	}

	// 7. DRBsFailedToBeSetupModList (Optional)
	if len(msg.DRBsFailedToBeSetupModList) > 0 {
		items := []*ies.DRBsFailedToBeSetupModItem{}
		for _, item := range msg.DRBsFailedToBeSetupModList {
			val := item
			items = append(items, &val)
		}
		// Constraint: Lb: 1, Ub: 64
		seq := ies.NewSequence(items, aper.Constraint{Lb: 1, Ub: 64}, false)
		list = append(list, ies.F1apMessageIE{
			Id:          ies.ProtocolIEID{Value: ies.ProtocolIEID_DRBsFailedToBeSetupModList},
			Criticality: ies.Criticality{Value: ies.Criticality_PresentIgnore},
			Value:       &seq,
		})
	}

	// 8. BHChannelsSetupModList (Strict Mandatory)
	{
		dummyItem := ies.BHChannelsSetupModItem{
			BHRLCChannelID: aper.BitString{
				Bytes:   []byte{0x00, 0x00},
				NumBits: 16,
			},
		}
		items := []*ies.BHChannelsSetupModItem{&dummyItem}
		// Constraint: Lb: 1, Ub: 65536
		seq := ies.NewSequence(items, aper.Constraint{Lb: 1, Ub: 65536}, false)
		list = append(list, ies.F1apMessageIE{
			Id:          ies.ProtocolIEID{Value: ies.ProtocolIEID_BHChannelsSetupModList},
			Criticality: ies.Criticality{Value: ies.Criticality_PresentIgnore},
			Value:       &seq,
		})
	}

	// 9. BHChannelsModifiedList (Strict Mandatory)
	{
		dummyItem := ies.BHChannelsModifiedItem{
			BHRLCChannelID: aper.BitString{
				Bytes:   []byte{0x00, 0x00},
				NumBits: 16,
			},
		}
		items := []*ies.BHChannelsModifiedItem{&dummyItem}
		// Constraint: Lb: 1, Ub: 65536
		seq := ies.NewSequence(items, aper.Constraint{Lb: 1, Ub: 65536}, false)
		list = append(list, ies.F1apMessageIE{
			Id:          ies.ProtocolIEID{Value: ies.ProtocolIEID_BHChannelsModifiedList},
			Criticality: ies.Criticality{Value: ies.Criticality_PresentIgnore},
			Value:       &seq,
		})
	}

	// 10. RequestedTargetCellGlobalID (Strict Mandatory)
	targetCGI := msg.RequestedTargetCellGlobalID
	if targetCGI == nil {
		targetCGI = &ies.NRCGI{
			PLMNIdentity: []byte{0x00, 0x00, 0x00},
			NRCellIdentity: aper.BitString{
				Bytes:   []byte{0x00, 0x00, 0x00, 0x00, 0x00},
				NumBits: 36,
			},
		}
	}
	list = append(list, ies.F1apMessageIE{
		Id:          ies.ProtocolIEID{Value: ies.ProtocolIEID_RequestedTargetCellGlobalID},
		Criticality: ies.Criticality{Value: ies.Criticality_PresentReject},
		Value:       targetCGI,
	})

	return list
}
