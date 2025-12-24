package uecontext

import (
	"du_ue/internal/common/logger"
	"fmt"
	"sync"
	"time"

	"github.com/reogac/nas"
)

// DecodeGPRSTimer3 decodes GPRS Timer 3 value (TS 24.008 10.5.7.4)
// NOTE: The underlying NAS library (github.com/reogac/nas) provides the GprsTimer3 struct
// but does not implement the logic to interpret the unit/value fields.
// We implement this decoding manually here to bridge that gap.
func DecodeGPRSTimer3(val uint8) time.Duration {
	unit := (val & 0xE0) >> 5
	timerValue := time.Duration(val & 0x1F)

	switch unit {
	case 0: // 10 minutes
		return timerValue * 10 * time.Minute
	case 1: // 1 hour
		return timerValue * 1 * time.Hour
	case 2: // 10 hours
		return timerValue * 10 * time.Hour
	case 3: // 2 seconds
		return timerValue * 2 * time.Second
	case 4: // 30 seconds
		return timerValue * 30 * time.Second
	case 5: // 1 minute
		return timerValue * 1 * time.Minute
	case 6: // 320 hours
		return timerValue * 320 * time.Hour
	case 7: // Deactivated
		return 0
	default:
		return 0
	}
}

// PDU Session States
const (
	PDUSessionInactive uint8 = iota
	PDUSessionActivePending
	PDUSessionActive
	PDUSessionModificationPending
	PDUSessionInactivePending
)

// PduSession represents a single PDU session
type PduSession struct {
	*logger.Logger
	Id    uint8  // PDU Session ID (1-15)
	state uint8  // Current state
	ueIP  string // UE IP address (formatted string)

	// Session parameters
	PduAddress                           string   // Raw hex string or similar? Handled by setIp logic usually.
	Dnn                                  *nas.Dnn // Changed from string to *nas.Dnn to match handler logic
	pduSessionType                       uint8
	SscMode                              uint8 // Exported for handler
	SNssai                               *nas.SNssai
	SessionAmbr                          *nas.SessionAmbr
	AuthorizedQosRules                   *nas.QosRules
	AuthorizedQosFlowDescriptions        *nas.QosFlowDescriptions
	ExtendedProtocolConfigurationOptions *nas.ExtendedProtocolConfigurationOptions
	AlwaysOnPduSessionIndication         uint8

	// Rejection info
	BackOffTimer   time.Duration // Extracted from BackOffTimerValue
	AllowedSscMode uint8         // Extracted from AllowedSscMode

	// Transaction
	pti uint8 // Procedure Transaction Identity

	// Parent UE
	ue *UeContext

	mutex sync.Mutex
}

// NewPduSession creates a new PDU Session
func NewPduSession(ue *UeContext, sessionId uint8) *PduSession {
	return &PduSession{
		Id:    sessionId,
		state: PDUSessionInactive,
		pti:   1, // Start with PTI = 1
		ue:    ue,
		Logger: logger.InitLogger("", map[string]string{
			"mod":   "pdu_session",
			"msin":  ue.msin,
			"pduId": fmt.Sprintf("%d", sessionId),
		}),
	}
}

func (ps *PduSession) SetSNssai(sst uint8, sd string) error {
	if ps.SNssai == nil {
		ps.SNssai = &nas.SNssai{}
	}
	return ps.SNssai.Set(sst, sd)
}

func (ps *PduSession) SetMappedSNssai(sst uint8, sd string) error {
	if ps.SNssai == nil {
		ps.SNssai = &nas.SNssai{}
	}
	return ps.SNssai.SetMapped(sst, sd)
}

// GetState returns current state (thread-safe)
func (ps *PduSession) GetState() uint8 {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()
	return ps.state
}

// SetState changes state (thread-safe)
func (ps *PduSession) SetState(state uint8) {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()
	ps.state = state
	ps.Info("PDU Session state changed to: %s", ps.stateToString(state))
}

// stateToString converts state to readable string
func (ps *PduSession) stateToString(state uint8) string {
	switch state {
	case PDUSessionInactive:
		return "INACTIVE"
	case PDUSessionActivePending:
		return "ACTIVE_PENDING"
	case PDUSessionActive:
		return "ACTIVE"
	case PDUSessionModificationPending:
		return "MODIFICATION_PENDING"
	case PDUSessionInactivePending:
		return "INACTIVE_PENDING"
	default:
		return "UNKNOWN"
	}
}

// GetNextPTI returns next PTI and increments (thread-safe)
func (ps *PduSession) GetNextPTI() uint8 {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()
	pti := ps.pti
	ps.pti++
	if ps.pti == 0 { // PTI wraps around at 255
		ps.pti = 1
	}
	return pti
}

// setIp sets the UE IP address for this PDU session
func (ps *PduSession) setIp(ip []byte) {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()

	if len(ip) == 4 {
		// IPv4
		ps.ueIP = fmt.Sprintf("%d.%d.%d.%d", ip[0], ip[1], ip[2], ip[3])
	} else if len(ip) == 16 {
		// IPv6
		ps.ueIP = fmt.Sprintf("%02x%02x:%02x%02x:%02x%02x:%02x%02x:%02x%02x:%02x%02x:%02x%02x:%02x%02x",
			ip[0], ip[1], ip[2], ip[3], ip[4], ip[5], ip[6], ip[7],
			ip[8], ip[9], ip[10], ip[11], ip[12], ip[13], ip[14], ip[15])
	}
}

// GetIP returns the UE IP address
func (ps *PduSession) GetIP() string {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()
	return ps.ueIP
}

// IsActive checks if session is active
func (ps *PduSession) IsActive() bool {
	return ps.GetState() == PDUSessionActive
}
