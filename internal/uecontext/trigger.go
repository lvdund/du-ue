package uecontext

import (
	"github.com/reogac/nas"
)

// TriggerInitRegistration creates and sends Registration Request NAS message
func (ue *UeContext) TriggerInitRegistration() error {
	ue.Info("Initiating Registration")

	msg := &nas.RegistrationRequest{
		UeSecurityCapability: ue.secCap,
	}
	msg.RegistrationType = nas.NewRegistrationType(true, nas.RegistrationType5GSInitialRegistration)

	msg.MobileIdentity = ue.suci
	msg.Ngksi.Id = 7
	msg.SetSecurityHeader(nas.NasSecNone)

	nasPdu, err := nas.EncodeMm(nil, msg)
	if err != nil {
		ue.Error("Error encoding registration request: %v", err)
		return err
	}

	// Keep a copy for resending in security mode complete
	ue.mutex.Lock()
	ue.nasPdu = make([]byte, len(nasPdu))
	copy(ue.nasPdu, nasPdu)
	ue.mutex.Unlock()

	// Update state to REGISTERING
	ue.SetState(UE_STATE_REGISTERING)

	return nil
}

// TriggerInitPduSessionReleaseComplete sends PDU Session Release Complete
func (ue *UeContext) triggerInitPduSessionReleaseComplete(pduSession *PduSession) error {
	ue.Info("Initiating PDU Session Release Complete for Session ID %d", pduSession.Id)

	msg := &nas.PduSessionReleaseComplete{}
	// Set SM Header using setters because fields are unexported
	msg.SetSessionId(pduSession.Id)
	msg.SetPti(1) // Using default PTI 1

	// Note: ExtendedProtocolConfigurationOptions is optional and nil here

	// Encode 5GSM message
	gsmPdu, err := nas.EncodeSm(msg)
	if err != nil {
		ue.Error("Error encoding PDU Session Release Complete: %v", err)
		return err
	}

	// Create Uplink NAS Transport (MM message) carrying the GSM message
	ulNasTransport := &nas.UlNasTransport{
		PayloadContainerType: nas.PayloadContainerTypeN1SMInfo, // Payload container type for N1 SM Info
		PayloadContainer:     gsmPdu,
		PduSessionId:         &pduSession.Id,
		RequestType:          nil, // Optional
	}
	ulNasTransport.SetSecurityHeader(nas.NasSecBoth)

	// Encode MM message
	nasPdu, err := nas.EncodeMm(ue.getNasContext(), ulNasTransport)
	if err != nil {
		ue.Error("Error encoding Uplink NAS Transport for Release Complete: %v", err)
		return err
	}

	ue.Send_UlInformationTransfer_To_Du(nasPdu)

	// Update state
	ue.releasePduSession(pduSession.Id)

	return nil
}
