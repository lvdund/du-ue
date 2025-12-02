package sec

//this file is taken as it is from UDM
import (
	"encoding/binary"
	"fmt"
)

const (
	SQN_MAX uint64 = 0xffffffffffff
	SQN_IND uint64 = 32
)

type Sqn struct {
	value uint64
}

// encode Bytes
func (n *Sqn) Bytes() []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, n.value)
	return buf[2:]
}

// increase by one
func (n *Sqn) Inc() {
	n.value++
	if n.value > SQN_MAX {
		n.value = 0
	}
}

func (n *Sqn) Set(sqn []byte) {
	var buf [8]byte
	copy(buf[2:], sqn)
	n.value = binary.BigEndian.Uint64(buf[:])
}

func (n *Sqn) GetVal() uint64 {
	return n.value
}

// reset due to resync
func (n *Sqn) reset(sqn []byte) {
	n.Set(sqn)
	n.value += SQN_IND - 1
	n.Inc()
}

func (n *Sqn) String() string {
	return fmt.Sprintf("%x", n.Bytes())
}
