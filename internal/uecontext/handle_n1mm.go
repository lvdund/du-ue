package uecontext

import (
	"time"

	"github.com/reogac/nas"

	"du_ue/internal/uecontext/sec"
)

// HandleNasMsg decodes and handles incoming NAS messages
func (ue *UeContext) HandleNasMsg(nasBytes []byte) {
	if len(nasBytes) == 0 {
		ue.Error("NAS message is empty")
		return
	}

	var nasMsg nas.NasMessage
	var err error
	if nasMsg, err = nas.Decode(ue.getNasContext(), nasBytes); err != nil {
		ue.Error("Decode Nas message failed: %s", err.Error())
		return
	}

	ue.handleNas_n1mm(&nasMsg)
}

func (ue *UeContext) handleNas_n1mm(nasMsg *nas.NasMessage) {
	gsm := nasMsg.Gsm
	if gsm != nil {
		ue.handleNas_n1sm(nasMsg)
		return
	}

	gmm := nasMsg.Gmm
	if gmm == nil {
		ue.Error("NAS message has no N1MM content")
		return
	}

	switch gmm.MsgType {
	case nas.IdentityRequestMsgType:
		ue.Info("Receive Identity Request")
		ue.handleIdentityRequest(gmm.IdentityRequest)

	case nas.AuthenticationRequestMsgType:
		ue.Info("Receive Authentication Request")
		ue.handleAuthenticationRequest(gmm.AuthenticationRequest)

	case nas.SecurityModeCommandMsgType:
		ue.Info("Receive Security Mode Command")
		ue.handleSecurityModeCommand(gmm.SecurityModeCommand)

	case nas.RegistrationAcceptMsgType:
		ue.Info("Receive Registration Accept")
		ue.handleRegistrationAccept(gmm.RegistrationAccept)
		ue.SetState(UE_STATE_REGISTERED)

	case nas.RegistrationRejectMsgType:
		ue.Error("Receive Registration Reject")
		ue.handleRegistrationReject(gmm.RegistrationReject)
		ue.SetState(UE_STATE_DEREGISTERED)

	case nas.AuthenticationRejectMsgType:
		ue.Error("Receive Authentication Reject")
		ue.handleAuthenticationReject(gmm.AuthenticationReject)
		ue.SetState(UE_STATE_DEREGISTERED)

	case nas.GmmStatusMsgType:
		ue.Error("Receive Status 5GMM")
		ue.handleGmmStatus(gmm.GmmStatus)
	case nas.DlNasTransportMsgType:
		ue.Info("Receive DL NAS Transport")
		ue.handleDlNasTransport(gmm.DlNasTransport)

	case nas.DlNasTransportMsgType:
		ue.Info("Receive DL NAS Transport")
		ue.handleDlNasTransport(gmm.DlNasTransport)

	default:
		ue.Warn("Received unknown NAS message 0x%x", nasMsg.Gmm.MsgType)
	}
}

func (ue *UeContext) handleCause5GMM(cause *uint8) {
	if cause != nil {
		ue.Error("UE received a 5GMM Failure, cause: %s", cause5GMMToString(uint8(*cause)))
	}
}

func (ue *UeContext) handleAuthenticationReject(message *nas.AuthenticationReject) {
	_ = message
	ue.Error("Authentication of UE failed")
	ue.SetState(UE_STATE_DEREGISTERED)
	ue.ResetSecurityContext()
}

func (ue *UeContext) handleRegistrationReject(message *nas.RegistrationReject) {
	ue.handleCause5GMM(&message.GmmCause)
	ue.SetState(UE_STATE_DEREGISTERED)
	ue.ResetSecurityContext()
}

func (ue *UeContext) handleGmmStatus(message *nas.GmmStatus) {
	ue.handleCause5GMM(&message.GmmCause)
}

func (ue *UeContext) handleAuthenticationRequest(message *nas.AuthenticationRequest) {
	var responsePdu []byte
	var response nas.GmmMessage

	if message.Ngksi.Id == 7 {
		ue.Fatal("Error in Authentication Request, ngKSI not the expected value")
	}

	if len(message.Abba) == 0 {
		ue.Fatal("Error in Authentication Request, ABBA Content is empty")
	}
	if message.AuthenticationParameterRand == nil {
		ue.Fatal("Error in Authentication Request, RAND is missing")
	}

	if message.AuthenticationParameterAutn == nil {
		ue.Fatal("Error in Authentication Request, AUTN is missing")
	}
	// getting NgKsi, RAND and AUTN from the message.
	ue.auth.ngKsi = message.Ngksi
	ue.auth.rand = message.AuthenticationParameterRand
	ue.auth.milenage.SetRand(ue.auth.rand)

	autn := message.AuthenticationParameterAutn
	abba := message.Abba

	// getting resStar
	errCode, paramDat := ue.auth.processAuthenticationInfo(autn, abba)
	switch errCode {

	case AUTH_MAC_FAILURE:
		ue.Info("Authenticity of the authentication request message: FAILED")
		ue.Info("Send authentication failure with MAC failure")
		msg := &nas.AuthenticationFailure{
			GmmCause: nas.Cause5GMMMACFailure,
		}
		msg.SetSecurityHeader(nas.NasSecNone)
		response = msg
	case AUTH_SYNC_FAILURE:
		ue.Info("Authenticity of the authentication request message: OK")
		ue.Info("SQN of the authentication request message: INVALID")
		ue.Info("Send authentication failure with Synch failure")
		msg := &nas.AuthenticationFailure{
			GmmCause:                       nas.Cause5GMMSynchFailure,
			AuthenticationFailureParameter: paramDat,
		}
		msg.SetSecurityHeader(nas.NasSecNone)
		response = msg

	case AUTH_SUCCESS:
		ue.Info("Authenticity of the authentication request message: OK")
		ue.Info("SQN of the authentication request message: VALID")
		ue.Info("Send authentication response")
		msg := &nas.AuthenticationResponse{
			AuthenticationResponseParameter: paramDat,
		}
		msg.SetSecurityHeader(nas.NasSecNone)
		response = msg
		// create an inactive security context
		ue.secCtx = sec.NewSecurityContext(&ue.auth.ngKsi, ue.auth.kamf, false)
	}

	responsePdu, _ = nas.EncodeMm(nil, response)
	ue.Send_UlInformationTransfer_To_Du(responsePdu)
}

func (ue *UeContext) handleSecurityModeCommand(message *nas.SecurityModeCommand) {
	//check for existing NgKsi
	if message.Ngksi.Id == 7 || ue.auth.ngKsi.Id != message.Ngksi.Id || ue.auth.ngKsi.Tsc != message.Ngksi.Tsc {
		ue.Error("Error in Security Mode Command, ngKSI not the expected value")
		ue.SetState(UE_STATE_DEREGISTERED)
		return
	}

	algs := message.SelectedNasSecurityAlgorithms
	switch algs.EncAlg() {
	case nas.AlgCiphering128NEA0:
		ue.Info("Type of ciphering algorithm is 5G-0")
	case nas.AlgCiphering128NEA1:
		ue.Info("Type of ciphering algorithm is 128-5G-1")
	case nas.AlgCiphering128NEA2:
		ue.Info("Type of ciphering algorithm is 128-5G-2")
	case nas.AlgCiphering128NEA3:
		ue.Info("Type of ciphering algorithm is 128-5G-3")
	}
	switch algs.IntAlg() {
	case nas.AlgIntegrity128NIA0:
		ue.Info("Type of integrity algorithm is 5G-IA0")
	case nas.AlgIntegrity128NIA1:
		ue.Info("Type of integrity algorithm is 128-5G-IA1")
	case nas.AlgIntegrity128NIA2:
		ue.Info("Type of integrity algorithm is 128-5G-IA2")
	case nas.AlgIntegrity128NIA3:
		ue.Info("Type of integrity algorithm is 128-5G-IA3")
	}

	rinmr := false
	if message.AdditionalSecurityInformation != nil {
		// checking BIT RINMR that triggered registration request in security mode complete.
		rinmr = message.AdditionalSecurityInformation.GetRetransmission()
		ue.Info("Have Additional Secutity Information, retransmission = %v", rinmr)
	}

	//derive NasContext keys (then the security context is activated)
	ue.secCtx.NasContext(true).DeriveKeys(algs.EncAlg(), algs.IntAlg(), ue.secCtx.Kamf())

	// Fixed IMEISV value
	imeisv := nas.Imei{IsSv: true}
	imeisv.Parse("1110000000000000") // fixed dummy imei
	response := &nas.SecurityModeComplete{
		Imeisv: &nas.MobileIdentity{
			Id: &imeisv,
		},
	}
	nasCtx := ue.getNasContext()
	// Include registration request in NasMessageContainer if RINMR bit is set
	if rinmr && len(ue.nasPdu) > 0 {
		response.NasMessageContainer = ue.nasPdu
	}

	response.SetSecurityHeader(nas.NasSecBothNew)
	responsePdu, _ := nas.EncodeMm(nasCtx, response)
	ue.Send_UlInformationTransfer_To_Du(responsePdu)
}

func (ue *UeContext) handleRegistrationAccept(message *nas.RegistrationAccept) {
	ue.Info("Handle Registration Accept")

	// Save 5G GUTI if assigned
	if message.Guti != nil {
		ue.set5gGuti(message.Guti)
		ue.Info("UE 5G GUTI: %s", ue.guti.String())
	} else {
		ue.Warn("UE was not assigned a 5G-GUTI by AMF")
	}

	// Send Registration Complete
	response := &nas.RegistrationComplete{}
	response.SetSecurityHeader(nas.NasSecBoth)
	nasCtx := ue.getNasContext() // must be non-nil
	responsePdu, _ := nas.EncodeMm(nasCtx, response)
	ue.Send_UlInformationTransfer_To_Du(responsePdu)

	ue.Info("Registration Complete sent")

	// After registration is complete, trigger PDU Session Establishment
	go func() {
		// Small delay to ensure Registration Complete is processed first
		time.Sleep(100 * time.Millisecond)

		ue.Info("Auto-triggering default PDU Session establishment")
		if err := ue.TriggerDefaultPduSession(); err != nil {
			ue.Error("Failed to trigger default PDU session: %v", err)
		}
	}()
}

func (ue *UeContext) handleIdentityRequest(message *nas.IdentityRequest) {
	switch uint8(message.IdentityType) {
	case nas.MobileIdentity5GSTypeSuci:
		ue.Info("Requested SUCI 5GS type")
	default:
		ue.Error("Only SUCI identity is supported")
		return
	}

	// Fixed SUCI response
	rsp := &nas.IdentityResponse{
		MobileIdentity: ue.suci,
	}
	nasCtx := ue.getNasContext()
	if nasCtx != nil {
		rsp.SetSecurityHeader(nas.NasSecBoth)
	} else {
		rsp.SetSecurityHeader(nas.NasSecNone)
	}

	if nasPdu, err := nas.EncodeMm(nasCtx, rsp); err != nil {
		ue.Error("Error encoding identity response: %v", err)
	} else {
		ue.Send_UlInformationTransfer_To_Du(nasPdu)
	}
}

func (ue *UeContext) handleDlNasTransport(message *nas.DlNasTransport) {
	if uint8(message.PayloadContainerType) != nas.PayloadContainerTypeN1SMInfo {
		ue.Error("Error in DL NAS Transport, Payload Container Type not expected value")
		return
	}

	if message.PduSessionId == nil {
		ue.Error("Error in DL NAS Transport, PDU Session ID is missing")
		return
	}

	// Decode the packed 5GSM message from the Payload Container
	nasMsg, err := nas.Decode(nil, message.PayloadContainer)
	if err != nil {
		ue.Error("Error in DL NAS Transport, fail to decode N1Sm: %v", err)
		return
	}

	// Route to 5GSM handler
	ue.handleNas_n1sm(&nasMsg)
}
