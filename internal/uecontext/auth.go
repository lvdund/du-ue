package uecontext

import (
	"bytes"

	"github.com/reogac/nas"

	"du_ue/internal/uecontext/sec"
)

const (
	AUTH_SUCCESS uint8 = iota
	AUTH_MAC_FAILURE
	AUTH_SYNC_FAILURE
)

type AuthContext struct {
	supi     string
	snn      []byte
	kamf     []byte
	rand     []byte
	ngKsi    nas.KeySetIdentifier
	sqn      sec.Sqn
	amf      []byte
	milenage *sec.Milenage
}

func (auth *AuthContext) processAuthenticationInfo(autn, abba []byte) (errCode uint8, output []byte) {
	ueSqn := auth.sqn.Bytes()

	// 1. Generate RES, CK, IK, AK
	res, ak := auth.milenage.F2F5()
	ck := auth.milenage.F3()
	ik := auth.milenage.F4()
	key := append(ck, ik...)

	//2.derive netSqn, netMacA from autn
	netSqn := make([]byte, 6)
	//netAmf := autn[6:8]
	sqnXorAk := autn[0:6]
	netMacA := autn[8:]
	for i := range 6 {
		netSqn[i] = sqnXorAk[i] ^ ak[i]
	}

	//3. calculate MacA and verify
	macA, _, _ := auth.milenage.F1(netSqn, auth.amf)
	if !bytes.Equal(macA, netMacA) {
		errCode = AUTH_MAC_FAILURE
		return
	}

	//4. check for sqn sync
	tmpSqn := new(sec.Sqn)
	tmpSqn.Set(netSqn)                                 //calculate net sqn in int64
	syncFailure := auth.sqn.GetVal() > tmpSqn.GetVal() //ue's sqn is greater than network's sqn
	if syncFailure {
		//4.1 prepare auts
		amfSync := []byte{0, 0} //resync AMF
		akStar := auth.milenage.F5star()
		// get mac_s using sqn ue.
		_, macS, _ := auth.milenage.F1(ueSqn, amfSync)

		sqnXorAk = make([]byte, 6)
		for i := range ueSqn {
			sqnXorAk[i] = ueSqn[i] ^ akStar[i]
		}

		output = append(sqnXorAk, macS...)
		errCode = AUTH_SYNC_FAILURE
		return
	}
	//update SQN from network
	auth.sqn.Set(netSqn)

	//5. derive KAMF
	//5.1 derive KAUSF
	sqnXorAk = make([]byte, 6)
	for i := range netSqn {
		sqnXorAk[i] = netSqn[i] ^ ak[i]
	}
	kAusf, _ := sec.KAUSF(key, auth.snn, sqnXorAk)

	//5.2 derive KSEAF
	kSeaf, _ := sec.SeafKey(kAusf, auth.snn)
	//5.3 derive KAMF
	auth.kamf, _ = sec.KAMF(kSeaf, []byte(auth.supi[5:]), abba)

	//6. prepare resStar
	_, output, _ = sec.ResstarXresstar(key, auth.snn, auth.rand, res)
	return
}
