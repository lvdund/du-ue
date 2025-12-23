package uecontext

import (
	"context"
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/lvdund/rrc"
	rrcies "github.com/lvdund/rrc/ies"
	"github.com/reogac/nas"

	"du_ue/internal/common/logger"
	"du_ue/internal/uecontext/sec"
	"du_ue/pkg/config"
)

// UE state enum (simple state management)
const (
	UE_STATE_DEREGISTERED uint8 = iota
	UE_STATE_REGISTERING
	UE_STATE_REGISTERED
)

// 5GSM PDU Session States
const (
	SM5G_PDU_SESSION_INACTIVE uint8 = iota
	SM5G_PDU_SESSION_ACTIVE_PENDING
	SM5G_PDU_SESSION_ACTIVE

	// Aliases for compatibility
	PDUSessionInactive = SM5G_PDU_SESSION_INACTIVE
	PDUSessionActive   = SM5G_PDU_SESSION_ACTIVE
)

type UeContext struct {
	*logger.Logger
	id uint16

	state uint8 // Simple state: DEREGISTERED, REGISTERING, REGISTERED

	mcc    string
	mnc    string
	secCap *nas.UeSecurityCapability
	supi   string
	msin   string
	suci   nas.MobileIdentity
	guti   *nas.Guti
	nasPdu []byte // registration request for resending in security mode complete

	auth   AuthContext          // on-going authentication context
	secCtx *sec.SecurityContext // current security context

	sessions [16]*PduSession

	mutex sync.Mutex
	ctx   context.Context

	// comm: ue vs du
	ReceiveFromDuChannel chan []byte
	SendToDuChannel      chan []byte
	IsReadyConn          chan bool

	PduSessions map[uint8]*PduSession
}

type PduSession struct {
	Id                                   uint8
	PduAddress                           string
	ueIP                                 string // Cached IP string for helpers
	Dnn                                  *nas.Dnn
	SNssai                               *nas.SNssai
	SessionAmbr                          *nas.SessionAmbr
	AuthorizedQosRules                   *nas.QosRules
	AuthorizedQosFlowDescriptions        *nas.QosFlowDescriptions
	ExtendedProtocolConfigurationOptions *nas.ExtendedProtocolConfigurationOptions
	SscMode                              uint8
	State                                uint8 // 5GSM_PDU_SESSION_INACTIVE, ACTIVE_PENDING, ACTIVE
}

// Helper methods from incoming branch requirements

func (ue *UeContext) getPduSession(id uint8) *PduSession {
	ue.mutex.Lock()
	defer ue.mutex.Unlock()
	if session, ok := ue.PduSessions[id]; ok {
		return session
	}
	return nil
}

func (ue *UeContext) releasePduSession(id uint8) {
	ue.mutex.Lock()
	defer ue.mutex.Unlock()
	delete(ue.PduSessions, id)
}

func (ps *PduSession) Info(format string, args ...interface{}) {
	// Simple wrapper for now, ideally would use a logger but PduSession doesn't have one attached directly
	// For now, relies on the caller logging or using this as a placeholder
	// In a real implementation this might propagate to the parent UE logger
	// For this fix, we just print to stdout to avoid compilation error if used for logging
	fmt.Printf("[PDU-%d] "+format+"\n", append([]interface{}{ps.Id}, args...)...)
}

func (ps *PduSession) setIp(content []byte) {
	// PDU Address content: [Address (4 or 16 bytes)]
	// The type byte is stripped by PduAddress.Content()
	if len(content) == 4 {
		// IPv4
		ps.ueIP = fmt.Sprintf("%d.%d.%d.%d", content[0], content[1], content[2], content[3])
	} else if len(content) == 16 {
		// IPv6
		ps.ueIP = fmt.Sprintf("%x:%x:%x:%x:%x:%x:%x:%x",
			content[0:2], content[2:4], content[4:6], content[6:8],
			content[8:10], content[10:12], content[12:14], content[14:16])
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

func (ps *PduSession) SetState(state uint8) {
	ps.State = state
}

func CreateUe(
	conf config.UEConfig,
	ctx context.Context,
) *UeContext {
	ue := &UeContext{
		id:          1, // Fixed ID for single UE
		mcc:         conf.PLMN.MCC,
		mnc:         conf.PLMN.MNC,
		msin:        conf.MSIN,
		secCap:      conf.GetUESecurityCapability(),
		state:       UE_STATE_DEREGISTERED,
		Logger:      logger.InitLogger("", map[string]string{"mod": "ue", "msin": conf.MSIN}),
		ctx:         ctx,
		PduSessions: make(map[uint8]*PduSession),
	}

	// init AuthContext
	key, _ := hex.DecodeString(conf.Key)
	if len(conf.OPC) > 0 {
		op, _ := hex.DecodeString(conf.OPC)
		ue.auth.milenage, _ = sec.NewMilenage(key, op, true) // use OPC
	} else {
		op, _ := hex.DecodeString(conf.OP)
		ue.auth.milenage, _ = sec.NewMilenage(key, op, false) // use OP
	}
	ue.auth.amf, _ = hex.DecodeString(conf.AMF)

	// Fixed initial SQN (can be from config if needed)
	sqn := make([]byte, 6)
	ue.auth.sqn.Set(sqn)

	// add supi
	ue.auth.supi = fmt.Sprintf("imsi-%s%s%s", conf.PLMN.MCC, conf.PLMN.MNC, conf.MSIN)
	ue.supi = ue.auth.supi

	// create SUCI (simplified - using fixed values)
	ue.createConcealSuci(conf.PLMN.MCC, conf.PLMN.MNC, conf)

	return ue
}

func (ue *UeContext) GetMsin() string {
	return ue.msin
}

func (ue *UeContext) GetState() uint8 {
	ue.mutex.Lock()
	defer ue.mutex.Unlock()
	return ue.state
}

func (ue *UeContext) SetState(state uint8) {
	ue.mutex.Lock()
	defer ue.mutex.Unlock()
	ue.state = state
}

func (ue *UeContext) ResetSecurityContext() {
	ue.mutex.Lock()
	defer ue.mutex.Unlock()
	ue.secCtx = nil
	ue.auth.ngKsi.Id = 7
}

// return current nas security context for encoding/decoding nas message
func (ue *UeContext) getNasContext() *nas.NasContext {
	if ue.secCtx != nil {
		return ue.secCtx.NasContext(true)
	}
	return nil
}

func (ue *UeContext) createConcealSuci(mcc, mnc string, ueConf config.UEConfig) {
	// Simplified SUCI creation with fixed values
	// For simplicity, use null scheme (no encryption)
	suci := new(nas.SupiImsi)
	// Fixed values: routing indicator = "0000", protection scheme = 0 (null), key ID = 0
	suci.Parse([]string{mcc, mnc, "0000", "0", "0", ue.msin})
	ue.suci = nas.MobileIdentity{Id: &nas.Suci{Content: suci}}
}
func (ue *UeContext) set5gGuti(guti *nas.MobileIdentity) {
	if guti.GetType() != nas.MobileIdentity5GSType5gGuti {
		ue.Warn("Invalid GUTI type")
		return
	}
	ue.mutex.Lock()
	defer ue.mutex.Unlock()
	ue.guti = guti.Id.(*nas.Guti)
}

func (ue *UeContext) Terminate() {
	ue.mutex.Lock()
	defer ue.mutex.Unlock()
	ue.Info("UE Terminated")
}

func (ue *UeContext) Send_UlInformationTransfer_To_Du(nas_message []byte) {

	uldccchMessage := rrcies.UL_DCCH_Message{
		Message: rrcies.UL_DCCH_MessageType{
			Choice: rrcies.UL_DCCH_MessageType_Choice_C1,
			C1: &rrcies.UL_DCCH_MessageType_C1{
				Choice: rrcies.UL_DCCH_MessageType_C1_Choice_UlInformationTransfer,
				UlInformationTransfer: &rrcies.ULInformationTransfer{
					CriticalExtensions: rrcies.ULInformationTransfer_CriticalExtensions{
						Choice: rrcies.ULInformationTransfer_CriticalExtensions_Choice_UlInformationTransfer,
						UlInformationTransfer: &rrcies.ULInformationTransfer_IEs{
							DedicatedNAS_Message: &rrcies.DedicatedNAS_Message{
								Value: nas_message,
							},
						},
					},
				},
			},
		},
	}

	encoded, err := rrc.Encode(&uldccchMessage)
	if err != nil {
		ue.Error("Failed to encode RRCSetupComplete: %v", err)
		return
	}

	ue.SendToDuChannel <- encoded
}
