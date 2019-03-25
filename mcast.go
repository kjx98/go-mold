package MoldUDP

import (
	"net"
	"strings"
	"syscall"
)

func getIfAddr(ifn *net.Interface) (net.IP, error) {
	ret := net.IPv4zero
	if addrs, err := ifn.Addrs(); err != nil {
		log.Info("Get if Addr", err)
		return ret, err
	} else if len(addrs) > 0 {
		adr := strings.Split(addrs[0].String(), "/")[0]
		if ifAddr := net.ParseIP(adr); ifAddr != nil {
			ret = ifAddr
		} else {
			log.Infof("No addrs in if(%s)", ifn.Name)
			return ret, errNoIP
		}
	}
	return ret, nil
}

//加入组播域
func JoinMulticast(fd int, maddr net.IP, ifn *net.Interface) error {
	var mreq = &syscall.IPMreq{}
	copy(mreq.Multiaddr[:], maddr.To4())
	if ifn != nil {
		if adr, err := getIfAddr(ifn); err == nil {
			copy(mreq.Interface[:], adr.To4())
			log.Infof("Use %s for Multicast interface", adr)
		}
	}
	err := syscall.SetsockoptIPMreq(fd, syscall.IPPROTO_IP,
		syscall.IP_ADD_MEMBERSHIP, mreq)
	return err
}

func SetMulticastInterface(fd int, ifn *net.Interface) error {
	var sVal string
	if ifAddr, err := getIfAddr(ifn); err != nil {
		return err
	} else {
		sVal = string(ifAddr.To4())
		log.Info("Set out Multicast interface to", ifAddr)
	}
	err := syscall.SetsockoptString(fd, syscall.IPPROTO_IP,
		syscall.IP_MULTICAST_IF, sVal)
	return err
}

//退出组播域
func ExitMulticast(fd int, maddr net.IP) {
	var mreq = &syscall.IPMreq{}
	copy(mreq.Multiaddr[:], maddr.To4())
	syscall.SetsockoptIPMreq(fd, syscall.IPPROTO_IP,
		syscall.IP_DROP_MEMBERSHIP, mreq)
}

//设置路由的TTL值
func SetMulticastTTL(fd, ttl int) error {
	return syscall.SetsockoptInt(fd, syscall.IPPROTO_IP,
		syscall.IP_MULTICAST_TTL, ttl)
}
