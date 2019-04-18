package MoldUDP

import "net"

func (c *sockIf) Enabled(opts int) bool {
	if (opts & HasMmsg) != 0 {
		return true
	}
	return false
}

func (c *sockIf) MSend(buffs []Packet) (int, error) {
	if c.bRead {
		return 0, errModeRW
	}
	return Sendmmsg(c.fd, buffs, &c.dst)
}

func (c *sockIf) MRecv() ([]Packet, *net.UDPAddr, error) {
	if !c.bRead {
		return nil, nil, errModeRW
	}
	bufs := make([]Packet, maxBatch)
	copy(bufs, c.buffs[:])
	n, remoteAddr, err := Recvmmsg(c.fd, bufs, 0)
	if err != nil {
		return nil, nil, err
	}
	if n == 0 {
		return nil, nil, nil
	}
	rAddr := net.UDPAddr{Port: remoteAddr.Port}
	Addr := remoteAddr.Addr[:]
	rAddr.IP = net.IPv4(Addr[0], Addr[1], Addr[2], Addr[3])
	return bufs[:n], &rAddr, nil
}
