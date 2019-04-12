package nettypes

import (
	"fmt"
	"net"
)

type EthType uint16

var (
	All                 = EthType(0x0003)
	IPv4                = EthType(0x0800)
	ARP                 = EthType(0x0806)
	WakeOnLAN           = EthType(0x0842)
	TRILL               = EthType(0x22F3)
	DECnetPhase4        = EthType(0x6003)
	RARP                = EthType(0x8035)
	AppleTalk           = EthType(0x809B)
	AARP                = EthType(0x80F3)
	IPX1                = EthType(0x8137)
	IPX2                = EthType(0x8138)
	QNXQnet             = EthType(0x8204)
	IPv6                = EthType(0x86DD)
	EthernetFlowControl = EthType(0x8808)
	IEEE802_3           = EthType(0x8809)
	CobraNet            = EthType(0x8819)
	MPLSUnicast         = EthType(0x8847)
	MPLSMulticast       = EthType(0x8848)
	PPPoEDiscovery      = EthType(0x8863)
	PPPoESession        = EthType(0x8864)
	JumboFrames         = EthType(0x8870)
	HomePlug1_0MME      = EthType(0x887B)
	IEEE802_1X          = EthType(0x888E)
	PROFINET            = EthType(0x8892)
	HyperSCSI           = EthType(0x889A)
	AoE                 = EthType(0x88A2)
	EtherCAT            = EthType(0x88A4)
	EthernetPowerlink   = EthType(0x88AB)
	LLDP                = EthType(0x88CC)
	SERCOS3             = EthType(0x88CD)
	HomePlugAVMME       = EthType(0x88E1)
	MRP                 = EthType(0x88E3)
	MACSec              = EthType(0x88E5)
	IEEE1588            = EthType(0x88F7)
	IEEE802_1ag         = EthType(0x8902)
	FCoE                = EthType(0x8906)
	FCoEInit            = EthType(0x8914)
	RoCE                = EthType(0x8915)
	CTP                 = EthType(0x9000)
	VeritasLLT          = EthType(0xCAFE)
)

func (e EthType) String() string {
	switch e {
	case All:
		return "All"
	case IPv4:
		return "IPv4"
	case ARP:
		return "ARP"
	case WakeOnLAN:
		return "WakeOnLAN"
	case TRILL:
		return "TRILL"
	case DECnetPhase4:
		return "DECnetPhase4"
	case RARP:
		return "RARP"
	case AppleTalk:
		return "AppleTalk"
	case AARP:
		return "AARP"
	case IPX1:
		return "IPX1"
	case IPX2:
		return "IPX2"
	case QNXQnet:
		return "QNXQnet"
	case IPv6:
		return "IPv6"
	case EthernetFlowControl:
		return "EthernetFlowControl"
	case IEEE802_3:
		return "IEEE802_3"
	case CobraNet:
		return "CobraNet"
	case MPLSUnicast:
		return "MPLSUnicast"
	case MPLSMulticast:
		return "MPLSMulticast"
	case PPPoEDiscovery:
		return "PPPoEDiscovery"
	case PPPoESession:
		return "PPPoESession"
	case JumboFrames:
		return "JumboFrames"
	case HomePlug1_0MME:
		return "HomePlug1_0MME"
	case IEEE802_1X:
		return "IEEE802_1X"
	case PROFINET:
		return "PROFINET"
	case HyperSCSI:
		return "HyperSCSI"
	case AoE:
		return "AoE"
	case EtherCAT:
		return "EtherCAT"
	case EthernetPowerlink:
		return "EthernetPowerlink"
	case LLDP:
		return "LLDP"
	case SERCOS3:
		return "SERCOS3"
	case HomePlugAVMME:
		return "HomePlugAVMME"
	case MRP:
		return "MRP"
	case MACSec:
		return "MACSec"
	case IEEE1588:
		return "IEEE1588"
	case IEEE802_1ag:
		return "IEEE802_1ag"
	case FCoE:
		return "FCoE"
	case FCoEInit:
		return "FCoEInit"
	case RoCE:
		return "RoCE"
	case CTP:
		return "CTP"
	case VeritasLLT:
		return "VeritasLLT"
	default:
		return fmt.Sprintf("unknown type:%x", uint16(e))
	}
}

type VLANTag uint32

const (
	NotTagged VLANTag = 0
	Tagged    VLANTag = 4
)

type PCP uint8

const (
	BK = PCP(0x01)
	BE = PCP(0x00)
	EE = PCP(0x02)
	CA = PCP(0x03)
	VI = PCP(0x04)
	VO = PCP(0x05)
	IC = PCP(0x06)
	NC = PCP(0x07)
)

func (pcp PCP) String() string {
	switch pcp {
	case BK:
		return "BK - Background"
	case BE:
		return "BE - Best Effort"
	case EE:
		return "EE - Excellent Effort"
	case CA:
		return "CA - Critical Applications"
	case VI:
		return "VI - Video"
	case VO:
		return "VO - Voice"
	case IC:
		return "IC - Internetwork Control"
	case NC:
		return "NC - Network Control"
	}
	return fmt.Sprintf("corrupt type: %v", uint8(pcp))
}

type Frame []byte

func (f *Frame) String(l uint16, indent int) string {
	s := fmt.Sprintf(padLeft("Mac Len    : %d\n", "\t", indent), l) +
		fmt.Sprintf(padLeft("MAC Source : %s\n", "\t", indent), f.MACSource()) +
		fmt.Sprintf(padLeft("MAC Dest   : %s\n", "\t", indent), f.MACDestination())
	mT := f.VLANTag()
	if mT == Tagged {
		s += fmt.Sprint(padLeft("VLAN Info  : \n", "\t", indent))
		s += fmt.Sprintf(padLeft("PCP        : %s\n", "\t", indent), f.VLANPCP())
		s += fmt.Sprintf(padLeft("DEI        : %s\n", "\t", indent), f.VLANDEI())
		s += fmt.Sprintf(padLeft("ID         : %s\n", "\t", indent), f.VLANID())
	}
	s += fmt.Sprintf(padLeft("MAC Type   : %s\n", "\t", indent), f.MACEthertype(mT)) +
		f.GetPayString(l, indent, mT)
	return s
}

func (f *Frame) MACSource() net.HardwareAddr {
	return net.HardwareAddr((*f)[6:12])
}

func (f *Frame) MACDestination() net.HardwareAddr {
	return net.HardwareAddr((*f)[:6])
}

func (f *Frame) VLANTag() VLANTag {
	if (*f)[12] == 0x81 && (*f)[13] == 0x00 {
		return Tagged
	}
	return NotTagged
}

func (f *Frame) VLANPCP() PCP {
	return PCP((*f)[14] & 0xE0)
}

func (f *Frame) VLANDEI() bool {
	return (*f)[14]&0x20 == 0x20
}

func (f *Frame) VLANID() uint16 {
	return uint16((*f)[14]&0x0f)<<8 | uint16((*f)[15])
}

func (f *Frame) MACEthertype(tag VLANTag) EthType {
	pos := 12 + tag
	tt := uint16((*f)[pos])<<8 | uint16((*f)[pos+1])
	return EthType(tt)
}

func (f *Frame) MACPayload(tag VLANTag) ([]byte, uint16) {
	off := 14 + uint16(tag)
	return (*f)[off:], off
}

func (f *Frame) GetPayString(frameLen uint16, indent int, tag VLANTag) string {
	p, off := f.MACPayload(tag)
	frameLen -= off
	indent++
	switch f.MACEthertype(tag) {
	case IPv4:
		return IPv4_P(p).String(frameLen, indent)
	default:
		return "unknown eth payload...\n"
	}
}

func IsMACBroadcast(addr net.HardwareAddr) bool {
	return addr[0] == 0xFF && addr[1] == 0xFF && addr[2] == 0xFF && addr[3] == 0xFF && addr[4] == 0xFF && addr[5] == 0xFF
}

func IsMACMulticastIPv4(addr net.HardwareAddr) bool {
	return addr[0] == 0x01 && addr[1] == 0x00 && addr[2] == 0x5E
}

func IsMACMulticastIPv6(addr net.HardwareAddr) bool {
	return addr[0] == 0x33 && addr[1] == 0x33
}
