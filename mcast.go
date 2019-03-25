package MoldUDP

import (
	"net"
	"strings"
	"syscall"
)

//加入组播域
func UDPMulticast(fd int, maddr net.IP, ifn *net.Interface) error {
	var mreq = &syscall.IPMreq{}
	copy(mreq.Multiaddr[:], maddr.To4())
	if ifn != nil {
		if addrs, err := ifn.Addrs(); err != nil {
			log.Info("Get if Addr", err)
		} else if len(addrs) > 0 {
			adr := strings.Split(addrs[0].String(), "/")[0]
			log.Infof("Try %s for MC group", adr)
			if ifAddr := net.ParseIP(adr); ifAddr != nil {
				copy(mreq.Interface[:], ifAddr.To4())
				log.Infof("Use %s for Multicast interface", adr)
			}
		} else {
			log.Infof("No addrs in if(%s)", ifn.Name)
		}
	}
	err := syscall.SetsockoptIPMreq(fd, syscall.IPPROTO_IP,
		syscall.IP_ADD_MEMBERSHIP, mreq)
	return err
}

//退出组播域
func ExitMultiCast(fd int, maddr net.IP) {
	var mreq = &syscall.IPMreq{}
	copy(mreq.Multiaddr[:], maddr.To4())
	syscall.SetsockoptIPMreq(fd, syscall.IPPROTO_IP,
		syscall.IP_DROP_MEMBERSHIP, mreq)
}

//设置路由的TTL值
func SetTTL(fd, ttl int) error {
	return syscall.SetsockoptInt(fd, syscall.IPPROTO_IP,
		syscall.IP_MULTICAST_TTL, ttl)
}
