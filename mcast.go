package MoldUDP

import (
	"net"
)

type Packet []byte
type McastConn interface {
	Close() error
	Open(ip net.IP, port int, ifn *net.Interface) error
	OpenSend(ip net.IP, port int, bLoop bool, ifn *net.Interface) error
	Send(buff []byte) (int, error)
	Recv(buff []byte) (int, *net.UDPAddr, error)
	MSend(buffs []Packet) (int, error)
	MRecv(buffs []Packet) (int, *net.UDPAddr, error)
}

type netIf struct {
	conn *net.UDPConn
	adr  net.UDPAddr
}

func NewNetIf() McastConn {
	return &netIf{}
}

func (c *netIf) String() string {
	return "net Intf"
}

func (c *netIf) Close() error {
	return c.conn.Close()
}

func (c *netIf) Open(ip net.IP, port int, ifn *net.Interface) (err error) {
	var fd int = -1
	laddr := net.UDPAddr{IP: net.IPv4(0, 0, 0, 0), Port: port}
	c.conn, err = net.ListenUDP("udp4", &laddr)
	if err != nil {
		return err
	}
	laddr.IP = ip
	c.adr = laddr
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
	var fd int = -1
	laddr := net.UDPAddr{IP: net.IPv4(0, 0, 0, 0), Port: port}
	c.conn, err = net.ListenUDP("udp4", &laddr)
	if err != nil {
		return err
	}
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
	if err := JoinMulticast(fd, ip.To4(), ifn); err != nil {
		log.Info("add multicast group", err)
	}
	log.Info("Try Multicast ", ip, ":", port)
	if bLoop {
		if err = SetMulticastLoop(fd, true); err != nil {
			log.Info("set multicast loopback", err)
		}
	}
	return
}

//var nlogs = 0

func (c *netIf) Send(buff []byte) (int, error) {
	/*
		if nlogs < 2 {
			nlogs++
			log.Infof("try mc %d bytes to %s:%d", len(buff), c.adr.IP, c.adr.Port)
		}
	*/
	return c.conn.WriteToUDP(buff, &c.adr)
}

func (c *netIf) Recv(buff []byte) (int, *net.UDPAddr, error) {
	return c.conn.ReadFromUDP(buff)
}

func (c *netIf) MSend(buffs []Packet) (int, error) {
	return 0, nil
}
func (c *netIf) MRecv(buffs []Packet) (cnt int, rAddr *net.UDPAddr, errRet error) {
	for cnt = 0; cnt < len(buffs); cnt++ {
		_, adr, err := c.conn.ReadFromUDP(buffs[cnt])
		if cnt == 0 && err != nil {
			return 0, nil, err
		}
		if err != nil {
			break
		}
		if cnt == 0 {
			rAddr = adr
		}
	}
	return
}
