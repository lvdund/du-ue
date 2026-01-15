package du

import (
	"du_ue/internal/common/logger"
	"encoding/hex"
	"fmt"
	"io"
	"syscall"

	f1ap "github.com/JocelynWS/f1-gen"
	"github.com/JocelynWS/f1-gen/ies"
	"github.com/ishidawataru/sctp"
	"github.com/lvdund/ngap/aper"
)

const (
	F1AP_PPID uint32 = 62
)

// F1Client defines the interface for F1AP communication
type F1Client interface {
	Connect() error
	Close() error
	Send(data []byte) error
	SendF1SetupRequest() error
	ReadLoop()
}

// F1APClient handles SCTP connection and F1AP messaging with CU-CP
type F1APClient struct {
	*logger.Logger

	cuAddr    string
	cuPort    int
	localAddr string
	localPort int
	conn      *sctp.SCTPConn
	du        *DU
}

// NewF1APClient creates a new F1AP client
func NewF1APClient(remoteAddr string, remotePort int, localAddr string, localPort int, du *DU) (*F1APClient, error) {
	return &F1APClient{
		cuAddr:    remoteAddr,
		cuPort:    remotePort,
		localAddr: localAddr,
		localPort: localPort,
		du:        du,
		Logger: logger.InitLogger("info", map[string]string{
			"mod":   "f1ap_client",
			"du_id": fmt.Sprintf("%d", du.ID),
		}),
	}, nil
}

// Connect establishes SCTP connection to CU-CP
func (c *F1APClient) Connect() error {
	remoteAddr, err := sctp.ResolveSCTPAddr("sctp", fmt.Sprintf("%s:%d", c.cuAddr, c.cuPort))
	if err != nil {
		return fmt.Errorf("resolve remote SCTP addr: %w", err)
	}

	// var localAddr *sctp.SCTPAddr
	// if c.localAddr != "" && c.localPort > 0 {
	// 	localAddr, err = sctp.ResolveSCTPAddr("sctp", fmt.Sprintf("%s:%d", c.localAddr, c.localPort))
	// 	if err != nil {
	// 		return fmt.Errorf("resolve local SCTP addr: %w", err)
	// 	}
	// }

	conn, err := sctp.DialSCTPExt("sctp", nil, remoteAddr, sctp.InitMsg{
		NumOstreams:    2,
		MaxInstreams:   2,
		MaxAttempts:    2,
		MaxInitTimeout: 2,
	})
	if err != nil {
		return fmt.Errorf("dial SCTP: %w", err)
	}

	events := sctp.SCTP_EVENT_DATA_IO | sctp.SCTP_EVENT_SHUTDOWN | sctp.SCTP_EVENT_ASSOCIATION
	if err := conn.SubscribeEvents(events); err != nil {
		return fmt.Errorf("subscribe events: %w", err)
	}

	info := &sctp.SndRcvInfo{PPID: 62}
	if err := conn.SetDefaultSentParam(info); err != nil {
		return fmt.Errorf("set default sent param: %w", err)
	}

	if err := conn.SetReadBuffer(8192); err != nil {
		return fmt.Errorf("set read buffer: %w", err)
	}

	c.Info("Connection configured with PPID=%d", 62)

	c.conn = conn
	c.Info("SCTP connection established to CU-CP")
	return nil
}

// Close closes the SCTP connection
func (c *F1APClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Send sends F1AP message to CU-CP
func (c *F1APClient) Send(data []byte) error {
	if c.conn == nil {
		return fmt.Errorf("SCTP connection not established")
	}

	info := &sctp.SndRcvInfo{
		PPID:   62,
		Stream: 0,
	}

	_, err := c.conn.SCTPWrite(data, info)
	if err != nil {
		return fmt.Errorf("SCTP write: %w", err)
	}

	return nil
}

// ReadLoop reads messages from CU-CP
func (c *F1APClient) ReadLoop() {
	buf := make([]byte, 8192)

	for {
		// Read using SCTPRead
		n, info, err := c.conn.SCTPRead(buf)
		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				c.Error("Connection closed by server")
				return
			}
			if err == syscall.EAGAIN || err == syscall.EINTR {
				continue
			}
			c.Error("Read error: %v", err)
			return
		}

		if info == nil {
			c.Error("Received nil info")
			continue
		}

		if info.PPID != 62 {
			c.Error("Wrong PPID: %d", info.PPID)
			continue
		}

		// c.Info("Received %d bytes (PPID=%d, Stream=%d)", n, info.PPID, info.Stream)
		go c.handleMessage(buf[:n])
	}
}

// handleMessage decodes and handles incoming F1AP messages
func (c *F1APClient) handleMessage(data []byte) error {
	c.Info("Handling F1AP message, length: %d", len(data))
	pdu, err, _ := f1ap.F1apDecode(data)
	if err != nil {
		c.Fatal("Failed to decode F1AP PDU: %v", err)
		return fmt.Errorf("decode F1AP PDU: %w", err)
	}

	switch pdu.Present {
	case ies.F1apPduSuccessfulOutcome:
		switch pdu.Message.ProcedureCode.Value {
		case ies.ProcedureCode_F1Setup:
			c.Info("Received F1 Setup Response")
			if response, ok := pdu.Message.Msg.(*ies.F1SetupResponse); ok {
				c.handleF1SetupResponse(response)
			}
		case ies.ProcedureCode_UEContextSetup:
			c.Info("Received UE Context Setup Response")
			c.du.HandleUeContextSetupResponse(&pdu)
		case ies.ProcedureCode_UEContextModificationRequired:
			c.Info("Received UE Context Modification Confirm (Handover Response)")
			if err := c.du.HandleUeContextModificationConfirm(&pdu); err != nil {
				c.Error("Failed to handle UE Context Modification Confirm: %v", err)
			}
		default:
			c.Warn("Received successful outcome %d", pdu.Message.ProcedureCode.Value)
		}
	case ies.F1apPduInitiatingMessage:
		switch pdu.Message.ProcedureCode.Value {
		case ies.ProcedureCode_DLRRCMessageTransfer:
			c.Info("Received DL RRC Message Transfer")
			if err := c.du.HandleDlRrcMessageTransfer(&pdu); err != nil {
				c.Error("Failed to handle DL RRC Message Transfer: %v", err)
			}
		case ies.ProcedureCode_UEContextSetup:
			c.Info("Received UE Context Setup Request")
			if err := c.du.HandleUeContextSetupRequest(&pdu); err != nil {
				c.Error("Failed to handle UE Context Setup Request: %v", err)
			}
		default:
			c.Info("Received initiating message %d", pdu.Message.ProcedureCode.Value)
		}
	case ies.F1apPduUnsuccessfulOutcome:
		c.Warn("Received unsuccessful outcome %d", pdu.Message.ProcedureCode.Value)
	}

	return nil
}

// handleF1SetupResponse processes F1 Setup Response
func (c *F1APClient) handleF1SetupResponse(response *ies.F1SetupResponse) {
	c.Info("F1 Setup Response received %d", response.TransactionID)
	// Notify DU that setup is complete
	c.du.OnF1SetupResponse()
}

// FIX: there are many fixed value
// SendF1SetupRequest encodes and sends F1 Setup Request
func (c *F1APClient) SendF1SetupRequest() error {
	cfg := c.du.Config

	// Convert MCC/MNC to PLMN bytes
	plmnBytes := convertMccMncToPlmn(cfg.PLMN.MCC, cfg.PLMN.MNC)

	// Create RRC Version (3 bits: 0x0c = 0b110 = RRC Release 15)
	rrcVersion := ies.RRCVersion{
		LatestRRCVersion: aper.BitString{
			Bytes:   []byte{2, 248, 57},
			NumBits: 3,
		},
	}

	tac := []byte{0x0, 0x0, 0x01}

	// Create served cell information
	servedCellInfo := ies.ServedCellInformation{
		NRCGI: ies.NRCGI{
			PLMNIdentity:   []byte{152, 249, 225}, // 99970
			NRCellIdentity: aper.BitString{Bytes: []byte{0x0, 0x0, 0x01, 0x0, 0x0}, NumBits: 36},
		},
		NRPCI: ies.NRPCI{
			Value: int64(cfg.Cell.PCI),
		},
		ServedPLMNs: []ies.ServedPLMNsItem{
			{
				PLMNIdentity: plmnBytes,
			},
		},
		FiveGSTAC:                      tac,
		MeasurementTimingConfiguration: []byte{1, 2, 3}, //FIX:
		NRModeInfo: ies.NRModeInfo{
			Choice: ies.NRModeInfoPresentFDD,
			FDD: &ies.FDDInfo{
				ULNRFreqInfo: ies.NRFreqInfo{
					NRARFCN: 1,
					FreqBandListNr: []ies.FreqBandNrItem{
						ies.FreqBandNrItem{
							FreqBandIndicatorNr: 1,
							SupportedSULBandList: []ies.SupportedSULFreqBandItem{
								ies.SupportedSULFreqBandItem{
									FreqBandIndicatorNr: 1,
								},
							},
						},
					},
				},
				DLNRFreqInfo: ies.NRFreqInfo{
					NRARFCN: 1,
					FreqBandListNr: []ies.FreqBandNrItem{
						ies.FreqBandNrItem{
							FreqBandIndicatorNr: 1,
						},
					},
				},
				ULTransmissionBandwidth: ies.TransmissionBandwidth{
					NRSCS: ies.NRSCS{Value: ies.NRSCSscs15},
					NRNRB: ies.NRNRB{Value: ies.NRNRBNrprachconfiglist},
				},
				DLTransmissionBandwidth: ies.TransmissionBandwidth{
					NRSCS: ies.NRSCS{Value: ies.NRSCSscs15},
					NRNRB: ies.NRNRB{Value: ies.NRNRBNrprachconfiglist},
				},
			},
		},
	}

	// Create served cells list item
	servedCellItem := ies.GNBDUServedCellsItem{
		ServedCellInformation: servedCellInfo,
		// GNBDUSystemInformation is optional, skip for now
	}

	// Create F1 Setup Request
	msg := ies.F1SetupRequest{
		TransactionID:   0, // Fixed transaction ID
		GNBDUID:         cfg.ID,
		GNBDUName:       []byte(cfg.Name),
		GNBDURRCVersion: rrcVersion,
		GNBDUServedCellsList: []ies.GNBDUServedCellsItem{
			servedCellItem,
		},
	}

	// Encode message
	buf, err := f1ap.F1apEncode(&msg)
	if err != nil {
		return fmt.Errorf("encode F1 Setup Request: %w", err)
	}

	c.Info("Sending F1 Setup Request")
	// Send via SCTP
	return c.Send(buf)
}

func convertMccMncToPlmn(mcc, mnc string) []byte {
	// Reverse MCC and MNC (as done in central-unit)
	reverse := func(s string) string {
		var aux string
		for _, v := range s {
			aux = string(v) + aux
		}
		return aux
	}

	revMcc := reverse(mcc)
	revMnc := reverse(mnc)

	var hexStr string
	if len(mnc) == 2 {
		// Format: mcc[1]mcc[2]fmcc[0]mnc[0]mnc[1]
		hexStr = fmt.Sprintf("%c%cf%c%c%c", revMcc[1], revMcc[2], revMcc[0], revMnc[0], revMnc[1])
	} else {
		// Format: mcc[1]mcc[2]mnc[2]mcc[0]mnc[0]mnc[1]
		hexStr = fmt.Sprintf("%c%c%c%c%c%c", revMcc[1], revMcc[2], revMnc[2], revMcc[0], revMnc[0], revMnc[1])
	}

	// Decode hex string to bytes
	result, err := hex.DecodeString(hexStr)
	if err != nil {
		// Fallback: return empty bytes if conversion fails
		return []byte{0x00, 0x00, 0x00}
	}
	return result
}
