package nettypes

import (
	"fmt"
)

type UDP_P []byte

func (t UDP_P) IPProtocol() IPProtocol {
	return UDP
}

func (t UDP_P) Bytes() []byte {
	return t
}

func (t UDP_P) String(frameLen uint16, indent int) string {
	ret := fmt.Sprintf(padLeft("UDP Len      : %d/%d\n", "\t", indent),
		frameLen, t.Length()) +
		fmt.Sprintf(padLeft("Source Port  : %d\n", "\t", indent), t.SourcePort()) +
		fmt.Sprintf(padLeft("Dest Port    : %d\n", "\t", indent), t.DestinationPort()) +
		fmt.Sprintf(padLeft("Checksum     : %02x\n", "\t", indent), t.Checksum()) +
		fmt.Sprintf(padLeft("CalcChecksum : %02x\n", "\t", indent), t.CalculateChecksum())
	if t.Length() == frameLen && frameLen < 256 && frameLen > 8 {
		ss, _ := t.Payload()
		ss = ss[:int(frameLen-8)]
		ret += fmt.Sprintf(padLeft("Payload      : %s\n", "\t", indent), string(ss))
	}
	return ret
}

func (t UDP_P) SourcePort() uint16 {
	return uint16(t[0])<<8 | uint16(t[1])
	//return inet.NToHS(t[0:2])
}

func (t UDP_P) DestinationPort() uint16 {
	return uint16(t[2])<<8 | uint16(t[3])
	//return inet.NToHS(t[2:4])
}

func (t UDP_P) Length() uint16 {
	return uint16(t[4])<<8 | uint16(t[5])
	//return inet.NToHS(t[4:6])
}

func (t UDP_P) SetLength(v uint16) {
	t[4] = byte(v >> 8)
	t[5] = byte(v & 0xff)
	//inet.PutShort(t[4:6], v)
}

func (t UDP_P) Checksum() uint16 {
	return uint16(t[6])<<8 | uint16(t[7])
	//return inet.NToHS(t[6:8])
}

func (t UDP_P) CalculateChecksum() uint16 {
	var cs uint32
	i := 0
	fl := t.Length()
	for ; fl > 1; i, fl = i+2, fl-2 {
		cs += uint32(byteOrder.Uint16(t[i : i+2]))
		if cs&0x80000000 > 0 {
			cs = (cs & 0xffff) + (cs >> 16)
		}
	}
	if fl > 0 {
		cs += uint32(uint8(t[i]))
	}
	for cs>>16 > 0 {
		cs = (cs & 0xffff) + (cs >> 16)
	}
	return ^uint16(cs)
}

func (t UDP_P) SetChecksum(v uint16) {
	t[6] = byte(v >> 8)
	t[7] = byte(v & 0xff)
	//inet.PutShort(t[6:8], v)
}

func (t UDP_P) Payload() ([]byte, uint16) {
	return t[8:], 8
}
