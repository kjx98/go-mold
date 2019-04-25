// +build linux

package MoldUDP

import (
	"net"
	"time"

	"github.com/kjx98/go-mold/nettypes"
)

type zsockIf struct {
	zs    *ZSocket
	dst   HardwareAddr
	src   HardwareAddr
	dstIP [4]byte
	srcIP [4]byte
	bRead bool
	port  int
	fake  bool
}

func newZSockIf() McastConn {
	return &zsockIf{}
}

func init() {
	registerIf("zsock", newZSockIf)
	registerIf("zsocket", newZSockIf)
}

func (c *zsockIf) Enabled(opts int) bool {
	if (opts & HasRingBuffer) != 0 {
		return true
	}
	if (opts & HasMmsg) != 0 {
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
	//c.zs, err = NewZSocket(ifn.Index, ENABLE_RX, 1024, 16384, ETH_IP)
	c.zs, err = NewZSocket(ifn.Index, ENABLE_RX, 2048, 8192, ETH_IP)
	if err != nil {
		return
	}
	c.port = port
	c.src = HardwareAddr(make([]byte, 6))
	copy(c.src, ifn.HardwareAddr)
	log.Info("Using zsocket, listen on", c.src)
	//log.Info("Using zsocket, max PacketSize:", c.zs.MaxPacketSize())
	fd := c.zs.Fd()
	//ReserveRecvBuf(fd)
	if err := setBPF(fd, port); err != nil {
		log.Info("setBPF", err)
	}
	if err := JoinPacketMulticast(fd, ip.To4(), ifn); err != nil {
		log.Info("add Packet multicast group", err)
	} else {
		copy(c.dstIP[:], ip.To4())
	}
	c.bRead = true
	return nil
}

func (c *zsockIf) OpenSend(ip net.IP, port int, bLoop bool, ifn *net.Interface) (err error) {
	if c.zs != nil {
		return errOpened
	}
	c.zs, err = NewZSocket(ifn.Index, ENABLE_TX|DISABLE_TX_LOSS, 2048, 4096, ETH_IP)
	if err != nil {
		// if in testing, no return now
		if !c.fake {
			return
		}
	}
	c.port = port
	c.src = HardwareAddr(make([]byte, 6))
	copy(c.src, ifn.HardwareAddr)
	if adr, err := getIfAddr(ifn); err == nil {
		copy(c.srcIP[:], adr.To4())
		log.Infof("Use %s for Multicast interface", adr)
	}
	if dst := ip.To4(); dst != nil {
		copy(c.dstIP[:], dst)
	}
	c.dst = GetMulticastHWAddr(ip)
	log.Info("Using zsocket, via", c.src, "mcast on", c.dst)
	//log.Info("Using zsocket, max PacketSize:", c.zs.MaxPacketSize())
	c.bRead = false
	return nil
}

func (c *zsockIf) Send(buff []byte) (int, error) {
	if c.bRead {
		return 0, errModeRW
	}
	n := len(buff)
	if _, err := c.zs.CopyToBuffer(buff, uint16(len(buff)), c.copyFx); err != nil {
		return 0, err
	}
	if _, err, _ := c.zs.FlushFrames(); err != nil {
		log.Error("zsocket flushFrame", err)
		return 0, err
	}
	return n, nil
}

func (c *zsockIf) copyFx(dst, src []byte, l int) uint16 {
	if l <= 0 {
		l = len(src)
	}
	if l > maxUDPsize {
		return 0
	}
	copy(dst, c.dst)
	copy(dst[6:], c.src)
	//dst[12] = 8
	//dst[13] = 0
	buildRawUDP(dst, l, c.port, c.srcIP[:], c.dstIP[:])
	copy(dst[14+28:], src)
	/*
		if time.Now().Unix() > logTime+1 {
			logTime = time.Now().Unix()
			f := nettypes.Frame(dst[:l+42])
			log.Info("IP packet:", f.String(uint16(l+42), 0))
		}
	*/
	return uint16(l + 28 + 14)
}

func (c *zsockIf) Recv(buff []byte) (int, *net.UDPAddr, error) {
	if !c.bRead {
		return 0, nil, errModeRW
	}
	return 0, nil, errNotSupport
}

func (c *zsockIf) MSend(buffs []Packet) (int, error) {
	if len(buffs) == 0 {
		return 0, nil
	}
	var n int
	for n = 0; n < len(buffs); n++ {
		buf := buffs[n]
		if _, err := c.zs.CopyToBuffer(buf, uint16(len(buf)), c.copyFx); err != nil {
			break
		}
	}
	if n > 0 {
		if _, err, _ := c.zs.FlushFrames(); err != nil {
			log.Error("zsocket flushFrame", err)
			return 0, err
		}
		return n, nil
	}
	return 0, nil
}

func (c *zsockIf) MRecv() (buffs []Packet, rAddr *net.UDPAddr, errRet error) {
	errRet = errNotSupport
	return
}

var logTime int64

func tryLog(ss string) {
	if time.Now().Unix() > logTime {
		logTime = time.Now().Unix()
		log.Info(ss)
	}
}

func (c *zsockIf) Listen(fx func([]byte, *net.UDPAddr)) {
	// args: interface index, options, ring block count, frameOrder, framesInBlock packet types
	// unless you know what you're doing just pay attention to the interface index, whether
	// or not you want the tx ring, rx ring, or both enabled, and what nettype you are listening
	// for.
	rAddr := net.UDPAddr{IP: net.IPv4zero}
	c.zs.Listen(func(fb []byte, frameLen, capturedLen uint16) {
		ln := capturedLen
		f := nettypes.Frame(fb[:ln])
		if f.MACEthertype(0) != nettypes.IPv4 {
			tryLog("MAC EtherType dismatch")
			return
		}
		mPay, mOff := f.MACPayload(0)
		ln -= mOff
		ip := nettypes.IPv4_P(mPay)
		if ip.Protocol() != nettypes.UDP {
			if time.Now().Unix() > logTime {
				logTime = time.Now().Unix()
				log.Info("IP Proto dismatch")
				log.Info("IP packet:", ip.String(ln, 0))
			}
			return
		}
		if ln < ip.Length() {
			tryLog("IP length too short")
			return
		}
		iPay, iOff := ip.Payload()
		udp := nettypes.UDP_P(iPay)
		ips := ip.SourceIP()
		rAddr.IP = net.IPv4(ips[0], ips[1], ips[2], ips[3])
		if int(udp.DestinationPort()) != c.port {
			tryLog("UDP port dismatch")
			return
		}
		rAddr.Port = int(udp.SourcePort())
		/*
			if time.Now().Unix() > logTime+1 {
				logTime = time.Now().Unix()
				log.Info("IP packet:", ip.String(ln, 0))
			}
		*/
		ln -= iOff
		if ln < udp.Length() {
			tryLog("UDP length too short")
			return
		}
		// we don't verify checksum
		uBuff, uOff := udp.Payload()
		ln -= uOff
		//fx(uBuff[:ln], &rAddr)
		fx(uBuff, &rAddr)
	})
}
