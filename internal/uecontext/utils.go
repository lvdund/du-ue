package uecontext

import "github.com/reogac/nas"

func deriveSNN(mcc, mnc string) string {
	// 5G:mnc093.mcc208.3gppnetwork.org
	var resu string
	if len(mnc) == 2 {
		resu = "5G:mnc0" + mnc + ".mcc" + mcc + ".3gppnetwork.org"
	} else {
		resu = "5G:mnc" + mnc + ".mcc" + mcc + ".3gppnetwork.org"
	}
	return resu
}

func cause5GMMToString(cause5GMM uint8) string {
	switch cause5GMM {
	case nas.Cause5GMMIllegalUE:
		return "Illegal UE"
	case nas.Cause5GMMPEINotAccepted:
		return "PEI not accepted"
	case nas.Cause5GMMIllegalME:
		return "5GS services not allowed"
	case nas.Cause5GMM5GSServicesNotAllowed:
		return "5GS services not allowed"
	case nas.Cause5GMMUEIdentityCannotBeDerivedByTheNetwork:
		return "UE identity cannot be derived by the network"
	case nas.Cause5GMMImplicitlyDeregistered:
		return "Implicitly de-registered"
	case nas.Cause5GMMPLMNNotAllowed:
		return "PLMN not allowed"
	case nas.Cause5GMMTrackingAreaNotAllowed:
		return "Tracking area not allowed"
	case nas.Cause5GMMRoamingNotAllowedInThisTrackingArea:
		return "Roaming not allowed in this tracking area"
	case nas.Cause5GMMNoSuitableCellsInTrackingArea:
		return "No suitable cells in tracking area"
	case nas.Cause5GMMMACFailure:
		return "MAC failure"
	case nas.Cause5GMMSynchFailure:
		return "Synch failure"
	case nas.Cause5GMMCongestion:
		return "Congestion"
	case nas.Cause5GMMUESecurityCapabilitiesMismatch:
		return "UE security capabilities mismatch"
	case nas.Cause5GMMSecurityModeRejectedUnspecified:
		return "Security mode rejected, unspecified"
	case nas.Cause5GMMNon5GAuthenticationUnacceptable:
		return "Non-5G authentication unacceptable"
	case nas.Cause5GMMN1ModeNotAllowed:
		return "N1 mode not allowed"
	case nas.Cause5GMMRestrictedServiceArea:
		return "Restricted service area"
	case nas.Cause5GMMLADNNotAvailable:
		return "LADN not available"
	case nas.Cause5GMMMaximumNumberOfPDUSessionsReached:
		return "Maximum number of PDU sessions reached"
	case nas.Cause5GMMInsufficientResourcesForSpecificSliceAndDNN:
		return "Insufficient resources for specific slice and DNN"
	case nas.Cause5GMMInsufficientResourcesForSpecificSlice:
		return "Insufficient resources for specific slice"
	case nas.Cause5GMMngKSIAlreadyInUse:
		return "ngKSI already in use"
	case nas.Cause5GMMNon3GPPAccessTo5GCNNotAllowed:
		return "Non-3GPP access to 5GCN not allowed"
	case nas.Cause5GMMServingNetworkNotAuthorized:
		return "Serving network not authorized"
	case nas.Cause5GMMPayloadWasNotForwarded:
		return "Payload was not forwarded"
	case nas.Cause5GMMDNNNotSupportedOrNotSubscribedInTheSlice:
		return "DNN not supported or not subscribed in the slice"
	case nas.Cause5GMMInsufficientUserPlaneResourcesForThePDUSession:
		return "Insufficient user-plane resources for the PDU session"
	case nas.Cause5GMMSemanticallyIncorrectMessage:
		return "Semantically incorrect message"
	case nas.Cause5GMMInvalidMandatoryInformation:
		return "Invalid mandatory information"
	case nas.Cause5GMMMessageTypeNonExistentOrNotImplemented:
		return "Message type non-existent or not implementedE"
	case nas.Cause5GMMMessageTypeNotCompatibleWithTheProtocolState:
		return "Message type not compatible with the protocol state"
	case nas.Cause5GMMInformationElementNonExistentOrNotImplemented:
		return "Information element non-existent or not implemented"
	case nas.Cause5GMMConditionalIEError:
		return "Conditional IE error"
	case nas.Cause5GMMMessageNotCompatibleWithTheProtocolState:
		return "Message not compatible with the protocol state"
	case nas.Cause5GMMProtocolErrorUnspecified:
		return "Protocol error, unspecified. Please share the pcap with me"
	default:
		return "Protocol error, unspecified. Please share the pcap with me"
	}
}
