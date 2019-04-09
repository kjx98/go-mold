package MoldUDP

import (
	"net"
	"syscall"
)

type sockIf struct {
	dst SockaddrInet4
	fd  int
}

func NewSockIf() McastConn {
	return &sockIf{fd: -1}
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
	// set Multicast
	err = JoinMulticast(c.fd, c.dst.Addr[:], ifn)
	if err != nil {
		log.Info("add multi group", err)
	}
	return nil
}

func (c *sockIf) Recv(buff []byte) (int, *net.UDPAddr, error) {
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
	return Sendto(c.fd, buff, 0, &c.dst)
}

func (c *sockIf) MSend(buffs []Packet) (int, error) {
	return 0, nil
}

func (c *sockIf) MRecv(buffs []Packet) (int, *net.UDPAddr, error) {
	return 0, nil, nil
}
