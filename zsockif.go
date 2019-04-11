package MoldUDP

import (
	"net"

	"github.com/kjx98/zsocket"
	"github.com/kjx98/zsocket/nettypes"
)

type zsockIf struct {
	zs    *zsocket.ZSocket
	bRead bool
	port  int
}

func newZSockIf() McastConn {
	return &zsockIf{}
}

func (c *zsockIf) Enabled(opts int) bool {
	if (opts & HasRingBuffer) != 0 {
		return true
	}
	return false
}

func (c *zsockIf) String() string {
	return "ZSocket Intf"
}

func (c *zsockIf) Close() error {
	if c.zs == nil {
		return errClosed
	}
	err := c.zs.Close()
	c.zs = nil
	return err
}

func (c *zsockIf) Open(ip net.IP, port int, ifn *net.Interface) (err error) {
	if c.zs != nil {
		return errOpened
	}
	c.zs, err = zsocket.NewZSocket(ifn.Index, zsocket.ENABLE_RX, 2048, 32, nettypes.IPv4)
	if err != nil {
		return
	}
	c.port = port
	fd := c.zs.Fd()
	if err := setBPF(fd, port); err != nil {
		log.Info("setBPF", err)
	}
	if err := JoinPacketMulticast(fd, ip.To4(), ifn); err != nil {
		log.Info("add Packet multicast group", err)
	}
	c.bRead = true
	return nil
}

func (c *zsockIf) OpenSend(ip net.IP, port int, bLoop bool, ifn *net.Interface) (err error) {
	if c.zs != nil {
		return errOpened
	}
	err = errNotSupport
	return
}

func (c *zsockIf) Send(buff []byte) (int, error) {
	if c.bRead {
		return 0, errModeRW
	}
	return 0, nil
}

func (c *zsockIf) Recv(buff []byte) (int, *net.UDPAddr, error) {
	if !c.bRead {
		return 0, nil, errModeRW
	}
	return 0, nil, errNotSupport
}

func (c *zsockIf) MSend(buffs []Packet) (int, error) {
	return 0, errNotSupport
}

func (c *zsockIf) MRecv() (buffs []Packet, rAddr *net.UDPAddr, errRet error) {
	errRet = errNotSupport
	return
}

func (c *zsockIf) Listen(fx func([]byte, *net.UDPAddr)) {
	// args: interface index, options, ring block count, frameOrder, framesInBlock packet types
	// unless you know what you're doing just pay attention to the interface index, whether
	// or not you want the tx ring, rx ring, or both enabled, and what nettype you are listening
	// for.
	rAddr := net.UDPAddr{IP: net.IPv4zero}
	c.zs.Listen(func(f *nettypes.Frame, frameLen, capturedLen uint16) {
		if f.MACEthertype(0) != nettypes.IPv4 {
			return
		}
		ln := frameLen
		mPay, mOff := f.MACPayload(0)
		ln -= mOff
		ip := nettypes.IPv4_P(mPay)
		if ip.Protocol() != nettypes.UDP {
			return
		}
		iPay, iOff := ip.Payload()
		ln -= iOff
		udp := nettypes.UDP_P(iPay)
		copy(rAddr.IP, ip.SourceIP())
		rAddr.Port = int(udp.SourcePort())
		// we don't verify checksum
		uBuff, _ := udp.Payload()
		fx(uBuff, &rAddr)
	})
}
