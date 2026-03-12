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

type UeContext struct {
	*logger.Logger
	id uint16

	state uint8 

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

	// Measurement context for handover
	measurement *MeasurementContext

	// Event queue for scheduling UE events
	eventQueue *eventQueue

	config *config.UEConfig

	mutex sync.Mutex
	ctx   context.Context

	// Active DU connection — replaced entirely on handover.
	activeDUConn *DUConnection
	connMu       sync.RWMutex

	// handoverCh receives a new DUConnection injected by DU target
	// after CU-CP completes UE Context Setup (F1AP).
	// measurement.go blocks on this channel during performRandomAccess.
	handoverCh chan *DUConnection

	// Legacy aliases — always point at the active DU connection's channels.
	ReceiveFromDuChannel chan []byte
	SendToDuChannel      chan []byte
	IsReadyConn          chan bool
}

// CreateUe creates a UeContext.
// pciToDUID has been removed — DU ID resolution is now done by CU-CP,
// not by the UE. The UE only reacts to the channel injected by DU target.
func CreateUe(
	conf config.UEConfig,
	ctx context.Context,
) *UeContext {
	ue := &UeContext{
		id:         1,
		mcc:        conf.PLMN.MCC,
		mnc:        conf.PLMN.MNC,
		msin:       conf.MSIN,
		secCap:     conf.GetUESecurityCapability(),
		state:      UE_STATE_DEREGISTERED,
		Logger:     logger.InitLogger("", map[string]string{"mod": "ue", "msin": conf.MSIN}),
		ctx:        ctx,
		config:     &conf,
		handoverCh: make(chan *DUConnection, 1),
	}

	// init AuthContext
	key, _ := hex.DecodeString(conf.Key)
	if len(conf.OPC) > 0 {
		op, _ := hex.DecodeString(conf.OPC)
		ue.auth.milenage, _ = sec.NewMilenage(key, op, true)
	} else {
		op, _ := hex.DecodeString(conf.OP)
		ue.auth.milenage, _ = sec.NewMilenage(key, op, false)
	}
	ue.auth.amf, _ = hex.DecodeString(conf.AMF)

	sqn := make([]byte, 6)
	ue.auth.sqn.Set(sqn)

	ue.auth.supi = fmt.Sprintf("imsi-%s%s%s", conf.PLMN.MCC, conf.PLMN.MNC, conf.MSIN)
	ue.supi = ue.auth.supi

	ue.createConcealSuci(conf.PLMN.MCC, conf.PLMN.MNC, conf)
	ue.initMeasurement()
	ue.initEventQueue()

	return ue
}

// ConnectToDU sets up the initial DU connection and starts the RRC receiver.
func (ue *UeContext) ConnectToDU(duID string) (*DUConnection, error) {
	ue.connMu.Lock()
	defer ue.connMu.Unlock()

	if ue.activeDUConn != nil {
		return nil, fmt.Errorf("UE %s already connected to DU %s; use SwitchActiveDU to handover",
			ue.msin, ue.activeDUConn.duID)
	}

	conn := newDUConnection(duID, ue.ctx)
	ue.setActiveConn(conn)
	go ue.runRrcReceiver(conn)

	ue.Info("Connected to DU: %s", duID)
	return conn, nil
}

// InjectHandoverConnection is called by DU target (via UEManager.HandoverUEToDU)
// after it receives UE Context Setup Request from CU-CP.
// This unblocks performRandomAccess in measurement.go.
func (ue *UeContext) InjectHandoverConnection(conn *DUConnection) {
	select {
	case ue.handoverCh <- conn:
		ue.Info("Handover connection injected for DU: %s", conn.duID)
	default:
		ue.Warn("handoverCh full, dropping connection for DU: %s", conn.duID)
	}
}

// SwitchActiveDU switches to a new DU connection that was already prepared
// by DU target and injected via InjectHandoverConnection.
// Called internally by performRandomAccess after receiving the new conn.
func (ue *UeContext) SwitchActiveDU(conn *DUConnection) {
	ue.connMu.Lock()
	defer ue.connMu.Unlock()

	if ue.activeDUConn != nil {
		oldID := ue.activeDUConn.duID
		ue.activeDUConn.Close()
		ue.Info("Closed connection to old DU: %s", oldID)
	}

	ue.setActiveConn(conn)
	go ue.runRrcReceiver(conn)

	ue.Info("Switched to new DU: %s", conn.duID)
}

// WaitForHandoverConnection blocks until DU target injects a new connection
// or context is cancelled. Called by performRandomAccess.
func (ue *UeContext) WaitForHandoverConnection() (*DUConnection, error) {
	select {
	case conn := <-ue.handoverCh:
		return conn, nil
	case <-ue.ctx.Done():
		return nil, fmt.Errorf("context cancelled while waiting for handover connection")
	}
}

// GetActiveDUID returns the ID of the DU the UE is currently connected to.
func (ue *UeContext) GetActiveDUID() string {
	ue.connMu.RLock()
	defer ue.connMu.RUnlock()
	if ue.activeDUConn == nil {
		return ""
	}
	return ue.activeDUConn.duID
}

// GetActiveDUConn returns the current active DU connection.
func (ue *UeContext) GetActiveDUConn() *DUConnection {
	ue.connMu.RLock()
	defer ue.connMu.RUnlock()
	return ue.activeDUConn
}

// sendToActiveDU queues a PDU on the active DU connection.
func (ue *UeContext) sendToActiveDU(pdu []byte) error {
	ue.connMu.RLock()
	conn := ue.activeDUConn
	ue.connMu.RUnlock()

	if conn == nil {
		return fmt.Errorf("UE %s has no active DU connection", ue.msin)
	}
	if !conn.Send(pdu) {
		return fmt.Errorf("failed to send PDU to DU %s (closed or context done)", conn.duID)
	}
	return nil
}

// runRrcReceiver listens for inbound RRC messages on conn.
func (ue *UeContext) runRrcReceiver(conn *DUConnection) {
	ue.Info("Started RRC receiver for DU: %s", conn.duID)

	for {
		select {
		case rrcBytes, ok := <-conn.ReceiveFromDu:
			if !ok {
				ue.Info("ReceiveFromDu closed, RRC receiver exiting for DU: %s", conn.duID)
				return
			}
			ue.Info("Received RRC message from DU %s, length: %d", conn.duID, len(rrcBytes))
			if err := ue.HandleRrcMsg(rrcBytes); err != nil {
				ue.Error("Failed to handle RRC message from DU %s: %v", conn.duID, err)
			}

		case <-conn.ctx.Done():
			ue.Info("Connection context done, RRC receiver exiting for DU: %s", conn.duID)
			return
		}
	}
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

func (ue *UeContext) getNasContext() *nas.NasContext {
	if ue.secCtx != nil {
		return ue.secCtx.NasContext(true)
	}
	return nil
}

func (ue *UeContext) createConcealSuci(mcc, mnc string, ueConf config.UEConfig) {
	suci := new(nas.SupiImsi)
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
	ue.connMu.Lock()
	defer ue.connMu.Unlock()

	if ue.activeDUConn != nil {
		ue.activeDUConn.Close()
		ue.activeDUConn = nil
	}
	ue.Info("UE Terminated: MSIN=%s", ue.msin)
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
		ue.Error("Failed to encode UL Information Transfer: %v", err)
		return
	}

	if err := ue.sendToActiveDU(encoded); err != nil {
		ue.Error("Failed to send UL Information Transfer: %v", err)
	}
}

// setActiveConn updates activeDUConn and syncs legacy channel aliases.
// Must be called with connMu write-locked.
func (ue *UeContext) setActiveConn(conn *DUConnection) {
	ue.activeDUConn = conn
	ue.ReceiveFromDuChannel = conn.ReceiveFromDu
	ue.SendToDuChannel = conn.SendToDu
	ue.IsReadyConn = conn.IsReady
}