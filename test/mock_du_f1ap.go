package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"du_ue/internal/uecontext"
	"du_ue/pkg/config"

	f1ap "github.com/JocelynWS/f1-gen"
	"github.com/JocelynWS/f1-gen/ies"
	"github.com/ishidawataru/sctp"
	"github.com/lvdund/ngap/aper"
	"github.com/lvdund/rrc"
	rrcies "github.com/lvdund/rrc/ies"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// ─── Config ────────────────────────────────────────────────────────────────

const (
	CUCP_ADDR  = "192.168.1.2"
	CUCP_PORT  = 38472
	DU_ID      = int64(1)
	DU_NAME    = "MockDU"
	MCC        = "208"
	MNC        = "93"
	PCI        = uint16(1)
	NUE        = 10
	BASE_MSIN  = "0000000001"
	UE_KEY     = "8baf473f2f8fd09487cccbd7097c6862"
	UE_OPC     = "b9912fce303952b8e4af328992d3d497"
	UE_AMF     = "8000"
)

// ─── UE entry ──────────────────────────────────────────────────────────────

type UEEntry struct {
	msin        string
	duUeF1apID  int64
	cuUeF1apID  int64 // filled when CU assigns it
	crnti       int64
	toUE        chan []byte // MockDU -> UE
	fromUE      chan []byte // UE -> MockDU
	isInitial   bool       // true = next UL msg is RRCSetupRequest
}

// ─── MockDU ────────────────────────────────────────────────────────────────

type MockDU struct {
	conn    *sctp.SCTPConn
	mu      sync.RWMutex
	ues     map[int64]*UEEntry // key: duUeF1apID
	idCtr   atomic.Int64

	log zerolog.Logger
}

func NewMockDU() *MockDU {
	return &MockDU{
		ues: make(map[int64]*UEEntry),
		log: log.With().Str("mod", "mock_du").Logger(),
	}
}

// ─── Connect & F1 Setup ────────────────────────────────────────────────────

func (m *MockDU) Connect() error {
	remote, err := sctp.ResolveSCTPAddr("sctp", fmt.Sprintf("%s:%d", CUCP_ADDR, CUCP_PORT))
	if err != nil {
		return err
	}
	conn, err := sctp.DialSCTPExt("sctp", nil, remote, sctp.InitMsg{
		NumOstreams: 2, MaxInstreams: 2, MaxAttempts: 3, MaxInitTimeout: 3,
	})
	if err != nil {
		return err
	}
	conn.SubscribeEvents(sctp.SCTP_EVENT_DATA_IO | sctp.SCTP_EVENT_SHUTDOWN)
	conn.SetDefaultSentParam(&sctp.SndRcvInfo{PPID: 62})
	conn.SetReadBuffer(65536)
	m.conn = conn
	m.log.Info().Msg("SCTP connected to CU-CP")
	return nil
}

func (m *MockDU) send(data []byte) error {
	_, err := m.conn.SCTPWrite(data, &sctp.SndRcvInfo{PPID: 62, Stream: 0})
	return err
}

func (m *MockDU) SendF1SetupRequest() error {
	plmn := convertPlmn(MCC, MNC)
	tac := []byte{0x00, 0x00, 0x01}

	msg := ies.F1SetupRequest{
		TransactionID: 0,
		GNBDUID:       DU_ID,
		GNBDUName:     []byte(DU_NAME),
		GNBDURRCVersion: ies.RRCVersion{
			LatestRRCVersion: aper.BitString{Bytes: []byte{2, 248, 57}, NumBits: 3},
		},
		GNBDUServedCellsList: []ies.GNBDUServedCellsItem{
			{
				ServedCellInformation: ies.ServedCellInformation{
					NRCGI: ies.NRCGI{
						PLMNIdentity:   plmn,
						NRCellIdentity: aper.BitString{Bytes: []byte{0x0, 0x0, 0x01, 0x0, 0x0}, NumBits: 36},
					},
					NRPCI:     ies.NRPCI{Value: int64(PCI)},
					FiveGSTAC: tac,
					ServedPLMNs: []ies.ServedPLMNsItem{{PLMNIdentity: plmn}},
					MeasurementTimingConfiguration: []byte{1, 2, 3},
					NRModeInfo: ies.NRModeInfo{
						Choice: ies.NRModeInfoPresentFDD,
						FDD: &ies.FDDInfo{
							ULNRFreqInfo: ies.NRFreqInfo{NRARFCN: 1, FreqBandListNr: []ies.FreqBandNrItem{{FreqBandIndicatorNr: 1}}},
							DLNRFreqInfo: ies.NRFreqInfo{NRARFCN: 1, FreqBandListNr: []ies.FreqBandNrItem{{FreqBandIndicatorNr: 1}}},
							ULTransmissionBandwidth: ies.TransmissionBandwidth{NRSCS: ies.NRSCS{Value: ies.NRSCSscs15}, NRNRB: ies.NRNRB{Value: ies.NRNRBNrprachconfiglist}},
							DLTransmissionBandwidth: ies.TransmissionBandwidth{NRSCS: ies.NRSCS{Value: ies.NRSCSscs15}, NRNRB: ies.NRNRB{Value: ies.NRNRBNrprachconfiglist}},
						},
					},
				},
			},
		},
	}
	buf, err := f1ap.F1apEncode(&msg)
	if err != nil {
		return err
	}
	m.log.Info().Msg("Sending F1SetupRequest")
	return m.send(buf)
}

// ─── ReadLoop ──────────────────────────────────────────────────────────────

func (m *MockDU) ReadLoop(ctx context.Context) {
	buf := make([]byte, 65536)
	for {
		n, info, err := m.conn.SCTPRead(buf)
		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				m.log.Warn().Msg("CU-CP closed connection")
				return
			}
			if err == syscall.EAGAIN || err == syscall.EINTR {
				continue
			}
			select {
			case <-ctx.Done():
				return
			default:
				m.log.Error().Err(err).Msg("SCTP read error")
				return
			}
		}
		if info == nil || info.PPID != 62 {
			continue
		}
		go m.handleF1AP(append([]byte{}, buf[:n]...))
	}
}

// ─── F1AP Dispatcher ───────────────────────────────────────────────────────

func (m *MockDU) handleF1AP(data []byte) {
	pdu, err, _ := f1ap.F1apDecode(data)
	if err != nil {
		m.log.Error().Err(err).Msg("F1AP decode error")
		return
	}

	switch pdu.Present {
	case ies.F1apPduSuccessfulOutcome:
		switch pdu.Message.ProcedureCode.Value {
		case ies.ProcedureCode_F1Setup:
			m.log.Info().Msg("Received F1SetupResponse → starting UEs")
			m.startAllUEs()
		}
	case ies.F1apPduInitiatingMessage:
		switch pdu.Message.ProcedureCode.Value {
		case ies.ProcedureCode_DLRRCMessageTransfer:
			m.handleDLRRC(&pdu)
		case ies.ProcedureCode_UEContextSetup:
			m.handleUEContextSetupRequest(&pdu)
		}
	case ies.F1apPduUnsuccessfulOutcome:
		m.log.Warn().Msgf("Unsuccessful outcome proc=%d", pdu.Message.ProcedureCode.Value)
	}
}

// ─── DL RRC → forward to UE ────────────────────────────────────────────────

func (m *MockDU) handleDLRRC(pdu *f1ap.F1apPdu) {
	msg, ok := pdu.Message.Msg.(*ies.DLRRCMessageTransfer)
	if !ok {
		return
	}
	m.log.Info().Msgf("DLRRCMessageTransfer: CU-UE=%d DU-UE=%d SRB=%d len=%d",
		msg.GNBCUUEF1APID, msg.GNBDUUEF1APID, msg.SRBID, len(msg.RRCContainer))

	m.mu.Lock()
	ue := m.ues[msg.GNBDUUEF1APID]
	if ue != nil && ue.cuUeF1apID == 0 {
		ue.cuUeF1apID = msg.GNBCUUEF1APID
	}
	m.mu.Unlock()

	if ue == nil {
		m.log.Error().Msgf("No UE for DU-UE-ID=%d", msg.GNBDUUEF1APID)
		return
	}
	ue.toUE <- msg.RRCContainer
}

// ─── UE Context Setup Request ──────────────────────────────────────────────

func (m *MockDU) handleUEContextSetupRequest(pdu *f1ap.F1apPdu) {
	msg, ok := pdu.Message.Msg.(*ies.UEContextSetupRequest)
	if !ok {
		return
	}
	m.log.Info().Msgf("UEContextSetupRequest: CU-UE=%d", msg.GNBCUUEF1APID)

	// forward RRC container if present
	if len(msg.RRCContainer) > 0 {
		var duUeId int64
		if msg.GNBDUUEF1APID != nil {
			duUeId = *msg.GNBDUUEF1APID
		}
		m.mu.RLock()
		ue := m.ues[duUeId]
		m.mu.RUnlock()
		if ue != nil {
			ue.toUE <- msg.RRCContainer
		}
	}

	// Send UEContextSetupResponse
	var duUeId int64
	if msg.GNBDUUEF1APID != nil {
		duUeId = *msg.GNBDUUEF1APID
	}
	m.sendUEContextSetupResponse(msg.GNBCUUEF1APID, duUeId)
}

func (m *MockDU) sendUEContextSetupResponse(cuUeId, duUeId int64) {
	plmn := convertPlmn(MCC, MNC)
	crnti := int64(1)
	resp := &ies.UEContextSetupResponse{
		GNBCUUEF1APID: cuUeId,
		GNBDUUEF1APID: duUeId,
		DUtoCURRCInformation: ies.DUtoCURRCInformation{CellGroupConfig: []byte{}},
		CRNTI:         &crnti,
		RequestedTargetCellGlobalID: &ies.NRCGI{
			PLMNIdentity:   plmn,
			NRCellIdentity: aper.BitString{Bytes: []byte{0x0, 0x0, 0x01, 0x0, 0x0}, NumBits: 36},
		},
	}
	buf, err := f1ap.F1apEncode(resp)
	if err != nil {
		m.log.Error().Err(err).Msg("encode UEContextSetupResponse")
		return
	}
	m.send(buf)
}

// ─── Start all UEs after F1 Setup ─────────────────────────────────────────

func (m *MockDU) startAllUEs() {
	baseMSINInt, _ := strconv.ParseUint(BASE_MSIN, 10, 64)

	for i := 0; i < NUE; i++ {
		msin := fmt.Sprintf("%010d", baseMSINInt+uint64(i))
		duUeId := m.idCtr.Add(1) - 1

		toUE := make(chan []byte, 100)
		fromUE := make(chan []byte, 100)

		ue := &UEEntry{
			msin:       msin,
			duUeF1apID: duUeId,
			crnti:      duUeId + 1,
			toUE:       toUE,
			fromUE:     fromUE,
			isInitial:  true,
		}

		m.mu.Lock()
		m.ues[duUeId] = ue
		m.mu.Unlock()

		// goroutine: đọc từ UE, gửi lên CU-CP qua F1AP
		go m.handleULFromUE(ue)

		// stagger để tránh flood
		delay := time.Duration(i) * 300 * time.Millisecond
		go func(ue *UEEntry, delay time.Duration) {
			time.Sleep(delay)
			m.log.Info().Str("msin", ue.msin).Msg("Starting UE")

			ueCfg := config.UEConfig{
				MSIN: ue.msin,
				Key:  UE_KEY,
				OPC:  UE_OPC,
				AMF:  UE_AMF,
				PLMN: config.PLMNConfig{MCC: MCC, MNC: MNC},
			}
			ueCtx := uecontext.InitUE(toUE, fromUE, ueCfg)
			if ueCtx == nil {
				m.log.Error().Str("msin", ue.msin).Msg("InitUE failed")
			} else {
				m.log.Info().Str("msin", ue.msin).Msg("✓ RRC initialized")
			}
		}(ue, delay)
	}
}

// ─── UL from UE → encode F1AP → send to CU-CP ─────────────────────────────

func (m *MockDU) handleULFromUE(ue *UEEntry) {
	l := m.log.With().Str("msin", ue.msin).Logger()

	for rrcBytes := range ue.fromUE {
		l.Info().Msgf("UL from UE: %d bytes (initial=%v)", len(rrcBytes), ue.isInitial)

		if ue.isInitial {
			ue.isInitial = false
			if err := m.sendInitialULRRC(ue, rrcBytes); err != nil {
				l.Error().Err(err).Msg("sendInitialULRRC failed")
			}
		} else {
			if err := m.sendULRRC(ue, rrcBytes); err != nil {
				l.Error().Err(err).Msg("sendULRRC failed")
			}
		}
	}
}

func (m *MockDU) sendInitialULRRC(ue *UEEntry, rrcBytes []byte) error {
	plmn := convertPlmn(MCC, MNC)

	cellGroupConfig := rrcies.CellGroupConfig{CellGroupId: rrcies.CellGroupId{Value: 0}}
	cgBytes, _ := rrc.Encode(&cellGroupConfig)

	msg := ies.InitialULRRCMessageTransfer{
		GNBDUUEF1APID: ue.duUeF1apID,
		NRCGI: ies.NRCGI{
			PLMNIdentity:   plmn,
			NRCellIdentity: aper.BitString{Bytes: []byte{0x0F, 0xFF, 0xFF, 0xFF, 0xFF}, NumBits: 36},
		},
		CRNTI:              ue.crnti,
		RRCContainer:       rrcBytes,
		TransactionID:      0,
		DUtoCURRCContainer: cgBytes,
	}
	buf, err := f1ap.F1apEncode(&msg)
	if err != nil {
		return err
	}
	m.log.Info().Str("msin", ue.msin).Msg("→ InitialULRRCMessageTransfer")
	return m.send(buf)
}

func (m *MockDU) sendULRRC(ue *UEEntry, rrcBytes []byte) error {
	m.mu.RLock()
	cuUeId := ue.cuUeF1apID
	m.mu.RUnlock()

	msg := ies.ULRRCMessageTransfer{
		GNBCUUEF1APID: cuUeId,
		GNBDUUEF1APID: ue.duUeF1apID,
		SRBID:         1,
		RRCContainer:  rrcBytes,
	}
	buf, err := f1ap.F1apEncode(&msg)
	if err != nil {
		return err
	}
	m.log.Info().Str("msin", ue.msin).Msgf("→ ULRRCMessageTransfer (CU-UE=%d)", cuUeId)
	return m.send(buf)
}

// ─── Helpers ───────────────────────────────────────────────────────────────

func convertPlmn(mcc, mnc string) []byte {
	// 3GPP TS 38.413 PLMN encoding:
	// Byte 0: MCC digit 2 | MCC digit 1
	// Byte 1: MNC digit 3 (or 0xF if 2-digit MNC) | MCC digit 3
	// Byte 2: MNC digit 2 | MNC digit 1
	b := make([]byte, 3)
	b[0] = ((mcc[1] - '0') << 4) | (mcc[0] - '0')
	if len(mnc) == 2 {
		b[1] = 0xF0 | (mcc[2] - '0')
	} else {
		b[1] = ((mnc[2] - '0') << 4) | (mcc[2] - '0')
	}
	b[2] = ((mnc[1] - '0') << 4) | (mnc[0] - '0')
	return b
}

// ─── Main ──────────────────────────────────────────────────────────────────

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	log.Info().Msgf("=== MockDU F1AP Test: %d UEs → free5gc ===", NUE)

	du := NewMockDU()
	if err := du.Connect(); err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to CU-CP")
	}
	defer du.conn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start read loop
	go du.ReadLoop(ctx)

	// Send F1 Setup Request
	if err := du.SendF1SetupRequest(); err != nil {
		log.Fatal().Err(err).Msg("Failed to send F1SetupRequest")
	}

	// Wait for Ctrl+C
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	log.Info().Msg("Running... Press Ctrl+C to stop")
	<-sig

	log.Info().Msg("Shutting down")
	cancel()
}