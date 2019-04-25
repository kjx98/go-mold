package MoldUDP

import (
	"github.com/kjx98/golib/nettypes"
	"net"
	"testing"
	"unsafe"
)

func TestHeaderSize(t *testing.T) {
	if n := unsafe.Sizeof(ipHeader{}); n != 20 {
		t.Errorf("error ipHeader size: %d should be 20", n)
	}
	if n := unsafe.Sizeof(udpHeader{}); n != 8 {
		t.Errorf("error udpHeader size: %d should be 8", n)
	}
}

func TestBuildUDP(t *testing.T) {
	buff := make([]byte, 128)
	srcBuf := []byte("test only, should be fine")
	n := len(srcBuf)
	srcIP := net.IPv4(192, 168, 0, 1).To4()
	dstIP := net.IPv4(239, 192, 168, 1).To4()
	buildIP(buff, n, srcIP, dstIP)
	ip := nettypes.IPv4_P(buff)
	ckSum := ip.CalculateChecksum()
	buff[10] = byte(ckSum >> 8)
	buff[11] = byte(ckSum & 0xff)
	if ip.PacketCorrupt() {
		t.Errorf("IP checksum dismatch: %x expect %x", ip.Checksum(),
			ip.CalculateChecksum())
	}
	buildUDP(buff[20:], 5858, n)
	copy(buff[28:], srcBuf)
	nn := n + 28
	t.Log("dump IP frame:", ip.String(uint16(nn), 0))
}

func TestBuildRawUDP(t *testing.T) {
	buff := make([]byte, 128)
	srcBuf := []byte("test only, should be fine")
	n := len(srcBuf)
	srcIP := net.IPv4(192, 168, 0, 1).To4()
	dstIP := net.IPv4(239, 192, 168, 1).To4()
	buildRawUDP(buff, n, 5858, srcIP, dstIP)
	ip := nettypes.IPv4_P(buff[14:])
	if ip.PacketCorrupt() {
		t.Errorf("IP checksum dismatch: %x expect %x", ip.Checksum(),
			ip.CalculateChecksum())
	}
	copy(buff[14+28:], srcBuf)
	nn := n + 28 + 14
	f := nettypes.Frame(buff[:nn])
	t.Log("dump Ether frame:", f.String(uint16(nn), 0))
}

func BenchmarkBuildIP(b *testing.B) {
	buff := [128]byte{}
	srcBuf := []byte("test only, should be fine")
	n := len(srcBuf)
	srcIP := net.IPv4(192, 168, 0, 1).To4()
	dstIP := net.IPv4(239, 192, 168, 1).To4()
	for i := 0; i < b.N; i++ {
		buildIP(buff[:], n, srcIP, dstIP)
	}
}

func BenchmarkBuildIPv4(b *testing.B) {
	buff := [128]byte{}
	srcBuf := []byte("test only, should be fine")
	n := len(srcBuf)
	srcIP := net.IPv4(192, 168, 0, 1).To4()
	dstIP := net.IPv4(239, 192, 168, 1).To4()
	for i := 0; i < b.N; i++ {
		buildIPv4(buff[:], n, srcIP, dstIP)
	}
}
