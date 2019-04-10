package MoldUDP

import (
	"errors"
	"net"
)

type Packet []byte

const (
	maxBatch = 32
)

type McastConn interface {
	HasMmsg() bool
	Close() error
	Open(ip net.IP, port int, ifn *net.Interface) error
	OpenSend(ip net.IP, port int, bLoop bool, ifn *net.Interface) error
	Send(buff []byte) (int, error)
	Recv(buff []byte) (int, *net.UDPAddr, error)
	MSend(buffs []Packet) (int, error)
	MRecv() ([]Packet, *net.UDPAddr, error)
}

var (
	errNotSupport = errors.New("Interface not support")
	errOpened     = errors.New("Already opened")
	errModeRW     = errors.New("Open/OpenSend for Recv/Send")
)

type netIf struct {
	bRead bool
	conn  *net.UDPConn
	adr   net.UDPAddr
}

func NewIf(netMode string) (netif McastConn) {
	switch netMode {
	case "sock", "socket":
		netif = newSockIf()
	case "net":
		fallthrough
	default:
		netif = newNetIf()
	}
	return
}

func newNetIf() McastConn {
	return &netIf{}
}

func (c *netIf) HasMmsg() bool {
	return false
}

func (c *netIf) String() string {
	return "net Intf"
}

func (c *netIf) Close() error {
	if c.conn == nil {
		return errClosed
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

func (c *netIf) Open(ip net.IP, port int, ifn *net.Interface) (err error) {
	if c.conn != nil {
		return errOpened
	}
	var fd int = -1
	laddr := net.UDPAddr{IP: net.IPv4(0, 0, 0, 0), Port: port}
	c.conn, err = net.ListenUDP("udp4", &laddr)
	if err != nil {
		return err
	}
	c.bRead = true
	c.adr.IP = ip
	c.adr.Port = port
	if ff, err := c.conn.File(); err == nil {
		fd = int(ff.Fd())
	} else {
		log.Error("Get UDPConn fd", err)
	}
	if fd >= 0 {
		ReserveRecvBuf(fd)
	}
	if err := JoinMulticast(fd, ip.To4(), ifn); err != nil {
		log.Info("add multicast group", err)
	}
	return nil
}

func (c *netIf) OpenSend(ip net.IP, port int, bLoop bool, ifn *net.Interface) (err error) {
	if c.conn != nil {
		return errOpened
	}
	var fd int = -1
	laddr := net.UDPAddr{IP: net.IPv4(0, 0, 0, 0), Port: port}
	if bLoop {
		// let system allc port
		laddr.Port = 0
	}
	c.conn, err = net.ListenUDP("udp4", &laddr)
	if err != nil {
		return err
	}
	c.bRead = false
	c.adr.IP = ip
	c.adr.Port = port
	if ff, err := c.conn.File(); err == nil {
		fd = int(ff.Fd())
	} else {
		log.Error("Get UDPConn fd", err)
	}
	if fd >= 0 {
		ReserveSendBuf(fd)
	}
	log.Info("Server listen", c.conn.LocalAddr())
	/*
		if err := JoinMulticast(fd, ip.To4(), ifn); err != nil {
			log.Info("add multicast group", err)
		}
	*/
	log.Infof("Try Multicast %s:%d", ip, port)
	if err := SetMulticastInterface(fd, ifn); err != nil {
		log.Info("set multicast interface", err)
	}
	if bLoop {
		if err := SetMulticastLoop(fd, true); err != nil {
			log.Info("set multicast loopback", err)
		}
	}
	return
}

func (c *netIf) Send(buff []byte) (int, error) {
	if c.bRead {
		return 0, errModeRW
	}
	return c.conn.WriteToUDP(buff, &c.adr)
}

func (c *netIf) Recv(buff []byte) (int, *net.UDPAddr, error) {
	if !c.bRead {
		return 0, nil, errModeRW
	}
	return c.conn.ReadFromUDP(buff)
}

func (c *netIf) MSend(buffs []Packet) (int, error) {
	return 0, errNotSupport
}
func (c *netIf) MRecv() (buffs []Packet, rAddr *net.UDPAddr, errRet error) {
	errRet = errNotSupport
	return
}
