package du

import (
	"fmt"
)

// PduSessionState represents the state of a PDU Session
type PduSessionState int

const (
	PduSessionStateReserved PduSessionState = iota // TEID allocated, waiting for UE
	PduSessionStateActive                          // UE confirmed, data path open
)

// GnbPDUSession stores PDU Session information
// Based on StormSIM pkg/model/pdusession.go
type GnbPDUSession struct {
	PduSessionId int64
	Teid         Teid
	Snssai       Snssai
	UpfIp        string
	PduType      uint64 // 0: IP, 1: Ethernet, 2: Unstructured
	QosId        int64
	FiveQi       int64
	PriArp       int64
	RlcMode      int64 // 0: AM, 1: UM-Bidirectional, 2: UM-UL, 3: UM-DL
	State        PduSessionState
}

// Teid stores Uplink and Downlink TEID
// Based on StormSIM pkg/model/pdusession.go
type Teid struct {
	UplinkTeid   uint32 // TEID for Uplink (DU -> UPF) - Assigned by UPF
	DownlinkTeid uint32 // TEID for Downlink (UPF -> DU) - Assigned by DU
}

// Snssai stores Network Slice Selection Assistance Information
// Based on StormSIM pkg/model/slice.go
type Snssai struct {
	Sst string // Slice/Service Type
	Sd  string // Slice Differentiator
}

// Helper methods for GnbPDUSession

// NewGnbPDUSession creates a new PDU Session
func NewGnbPDUSession(pduSessionId int64, upfIp string, sst, sd string, pduType uint64, qosId, priArp, fiveQi, rlcMode int64, ulTeid, dlTeid uint32) *GnbPDUSession {
	return &GnbPDUSession{
		PduSessionId: pduSessionId,
		UpfIp:        upfIp,
		Snssai: Snssai{
			Sst: sst,
			Sd:  sd,
		},
		PduType: pduType,
		QosId:   qosId,
		PriArp:  priArp,
		FiveQi:  fiveQi,
		RlcMode: rlcMode,
		Teid: Teid{
			UplinkTeid:   ulTeid,
			DownlinkTeid: dlTeid,
		},
		State: PduSessionStateReserved,
	}
}

// GetPduTypeString returns the string representation of PDU Type
func (s *GnbPDUSession) GetPduTypeString() string {
	switch s.PduType {
	case 0:
		return "IPv4"
	case 1:
		return "IPv6"
	case 2:
		return "IPv4v6"
	case 3:
		return "Ethernet"
	case 4:
		return "Unstructured"
	default:
		return fmt.Sprintf("Unknown(%d)", s.PduType)
	}
}
