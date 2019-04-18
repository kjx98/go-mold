// +build linux

package MoldUDP

import (
	"net"
	"syscall"
)

/*
#include <sys/types.h>
#include <linux/if_packet.h>  // AF_PACKET, sockaddr_ll
#include <linux/if_ether.h>  // ETH_P_ALL
#include <sys/socket.h>  // socket()
#include <unistd.h>  // close()
#include <string.h>
#include <arpa/inet.h>  // htons()
*/
import "C"

type rsockIf struct {
	fd      int
	ifIndex int
	dst     HardwareAddr
	src     HardwareAddr
	sll     *syscall.SockaddrLinklayer
	dstIP   [4]byte
	srcIP   [4]byte
	port    int
	bRead   bool
	buff    []byte
}

func newRSockIf() McastConn {
	return &rsockIf{fd: -1}
}

func init() {
	registerIf("rsock", newRSockIf)
}

func (c *rsockIf) Enabled(opts int) bool {
	if (opts & HasMmsg) != 0 {
		return true
	}
	return false
}

func (c *rsockIf) String() string {
	return "raw packet Socket Intf"
}

func (c *rsockIf) Close() error {
	if c.fd < 0 {
		return errClosed
	}
	err := Close(c.fd)
	c.fd = -1
	return err
}

func (c *rsockIf) Open(ip net.IP, port int, ifn *net.Interface) error {
	if c.fd >= 0 {
		return errOpened
	}
	return errNotSupport
}

func (c *rsockIf) OpenSend(ip net.IP, port int, bLoop bool, ifn *net.Interface) (err error) {
	if c.fd >= 0 {
		return errOpened
	}
	copy(c.dstIP[:], ip.To4())
	eT := int(C.htons(C.ushort(C.ETH_P_IP)))
	c.fd, err = Socket(syscall.AF_PACKET, syscall.SOCK_RAW, eT)
	if err != nil {
		log.Error("rsocket AF_PACKET", err)
		return
	}
	c.ifIndex = ifn.Index
	sll := syscall.SockaddrLinklayer{}
	sll.Protocol = uint16(eT)
	sll.Ifindex = c.ifIndex
	sll.Halen = _ETH_ALEN
	if err := syscall.Bind(c.fd, &sll); err != nil {
		log.Error("bind AF_PACKET", err)
		return err
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
	c.sll = &syscall.SockaddrLinklayer{}
	c.sll.Protocol = uint16(C.htons(C.ETH_P_IP))
	c.sll.Ifindex = c.ifIndex
	c.sll.Halen = C.ETH_ALEN
	copy(c.sll.Addr[:], c.dst)
	c.buff = make([]byte, 2048)
	copy(c.buff[:6], c.dst)
	copy(c.buff[6:12], c.src)
	log.Info("Using rsocket, via", c.src, "mcast on", c.dst)

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
	return
}

func (c *rsockIf) Recv(buff []byte) (int, *net.UDPAddr, error) {
	if !c.bRead {
		return 0, nil, errModeRW
	}
	return 0, nil, errNotSupport
}

func (c *rsockIf) Send(buff []byte) (int, error) {
	if c.bRead {
		return 0, errModeRW
	}
	n := len(buff)
	if n == 0 || n > maxUDPsize {
		return 0, errUDPlen
	}
	buildRawUDP(c.buff, n, c.port, c.srcIP[:], c.dstIP[:])
	copy(c.buff[14+28:], buff)
	return n, syscall.Sendto(c.fd, c.buff[:n+14+28], 0, c.sll)
}

func (c *rsockIf) MSend(buffs []Packet) (int, error) {
	if c.bRead {
		return 0, errModeRW
	}
	cnt := len(buffs)
	if cnt == 0 {
		return 0, nil
	}
	n := len(buffs[0])
	if len(buffs[cnt-1]) != n {
		cnt--
	}
	buildRawUDP(c.buff, n, c.port, c.srcIP[:], c.dstIP[:])
	return Sendmmsg2(c.fd, buffs[:cnt], c.buff[:14+28], c.ifIndex)
}

func (c *rsockIf) MRecv() ([]Packet, *net.UDPAddr, error) {
	if !c.bRead {
		return nil, nil, errModeRW
	}
	return nil, nil, errNotSupport
}

func (c *rsockIf) Listen(f func([]byte, *net.UDPAddr)) {
}
