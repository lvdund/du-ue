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
