package MoldUDP

import (
	"github.com/kjx98/go-mold/nettypes"
	"net"
	"testing"
)

func TestCopyUDP(t *testing.T) {
	buff := make([]byte, 256)
	srcBuf := []byte("test only")
	ifName := "lo"
	var ifn *net.Interface
	if ifnn, err := net.InterfaceByName(ifName); err != nil {
		t.Errorf("Ifn(%s) error: %v", ifName, err)
	} else {
		ifn = ifnn
	}
	zs := &zsockIf{fake: true}
	if err := zs.OpenSend(net.IPv4(239, 192, 168, 1), 5858, false, ifn); err != nil {
		t.Error("zsock OpenSend", err)
	}
	nn := zs.copyFx(buff, srcBuf, 0)
	if nn < 14+28 {
		t.Error("Packet too small", nn)
	}
	f := nettypes.Frame(buff[:nn])
	t.Log("dump IP frame:", f.String(nn, 4))
}
