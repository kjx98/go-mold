package MoldUDP

import (
	"net"
	"syscall"
)

type sockIf struct {
	dst   SockaddrInet4
	fd    int
	bRead bool
	buffs [maxBatch]Packet
}

func NewSockIf() McastConn {
	return &sockIf{fd: -1}
}

func (c *sockIf) HasMmsg() bool {
	return true
}

func (c *sockIf) String() string {
	return "rawSocket Intf"
}

func (c *sockIf) Close() error {
	if c.fd < 0 {
		return errClosed
	}
	err := Close(c.fd)
	c.fd = -1
	return err
}

func (c *sockIf) Open(ip net.IP, port int, ifn *net.Interface) error {
	if c.fd >= 0 {
		return errOpened
	}
	var err error
	copy(c.dst.Addr[:], ip.To4())
	c.dst.Port = port
	c.fd, err = Socket(syscall.AF_INET, syscall.SOCK_DGRAM, 0)
	if err != nil {
		return err
	}
	ReserveRecvBuf(c.fd)
	SetsockoptInt(c.fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
	err = Bind(c.fd, &SockaddrInet4{Port: port})
	if err != nil {
		Close(c.fd)
		log.Error("syscall.Bind", err)
		return err
	}
	c.bRead = true
	// set Multicast
	err = JoinMulticast(c.fd, c.dst.Addr[:], ifn)
	if err != nil {
		log.Info("add multi group", err)
	}
	return nil
}

func (c *sockIf) OpenSend(ip net.IP, port int, bLoop bool, ifn *net.Interface) (err error) {
	if c.fd >= 0 {
		return errOpened
	}
	copy(c.dst.Addr[:], ip.To4())
	c.dst.Port = port
	c.fd, err = Socket(syscall.AF_INET, syscall.SOCK_DGRAM, 0)
	if err != nil {
		return
	}

	laddr := SockaddrInet4{Port: port}
	SetsockoptInt(c.fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
	if bLoop {
		laddr.Port = 0
	}
	err = Bind(c.fd, &laddr)
	if err != nil {
		Close(c.fd)
		log.Error("syscall.Bind", err)
		return
	}
	c.bRead = false
	log.Info("Server listen", LocalAddr(c.fd))
	// set Multicast
	/*
		err = JoinMulticast(c.fd, c.dst.Addr[:], ifn)
		if err != nil {
			log.Info("add multi group", err)
		}
	*/
	//ReserveRecvBuf(c.fd)
	ReserveSendBuf(c.fd)
	log.Infof("Try Multicast %s:%d", ip, port)
	if err := SetMulticastInterface(c.fd, ifn); err != nil {
		log.Info("set multicast interface", err)
	}
	if bLoop {
		if err = SetMulticastLoop(c.fd, true); err != nil {
			log.Info("set multicast loopback", err)
		}
	}
	return
}

func (c *sockIf) Recv(buff []byte) (int, *net.UDPAddr, error) {
	if !c.bRead {
		return 0, nil, errModeRW
	}
	n, remoteAddr, err := Recvfrom(c.fd, buff, 0)
	if err != nil {
		return 0, nil, err
	}
	rAddr := net.UDPAddr{Port: remoteAddr.Port}
	Addr := remoteAddr.Addr[:]
	rAddr.IP = net.IPv4(Addr[0], Addr[1], Addr[2], Addr[3])
	return n, &rAddr, nil
}

func (c *sockIf) Send(buff []byte) (int, error) {
	if c.bRead {
		return 0, errModeRW
	}
	return Sendto(c.fd, buff, 0, &c.dst)
}

func (c *sockIf) MSend(buffs []Packet) (int, error) {
	return 0, nil
}

func (c *sockIf) MRecv() ([]Packet, *net.UDPAddr, error) {
	return nil, nil, nil
}
