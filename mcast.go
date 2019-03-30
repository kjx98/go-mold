package MoldUDP

import (
	"net"
	"runtime"
	"strings"
	"syscall"
	"time"
)

func getIfAddr(ifn *net.Interface) (net.IP, error) {
	ret := net.IPv4zero
	if ifn == nil {
		return ret, nil
	}
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

func Sleep(interv time.Duration) {
	tt := time.Now()
	for {
		runtime.Gosched()
		du := time.Now().Sub(tt)
		if du < interv {
			continue
		}
		break
	}
}

func ReserveRecvBuf(fd int) {
	bLen := 4 * 1024 * 1024
	if bl, err := syscall.GetsockoptInt(fd, syscall.SOL_SOCKET,
		syscall.SO_RCVBUF); err == nil {
		log.Infof("Socket RCVBUF is %d Kb", bl/1024)
	}
	log.Infof("Try set Socket RcvBuf to %d KB", bLen/1024)
	if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_RCVBUF,
		bLen); err != nil {
		log.Error("SetsockoptInt, SO_RCVBUF", err)
	}
	if bl, err := syscall.GetsockoptInt(fd, syscall.SOL_SOCKET,
		syscall.SO_RCVBUF); err == nil {
		log.Infof("Socket RCVBUF is %d Kb", bl/1024)
	}
}

func ReserveSendBuf(fd int) {
	bLen := 2 * 1024 * 1024
	if bl, err := syscall.GetsockoptInt(fd, syscall.SOL_SOCKET,
		syscall.SO_SNDBUF); err == nil {
		log.Infof("Socket SNDBUF is %d Kb", bl/1024)
	}
	log.Infof("Try set Socket RcvBuf to %d KB", bLen/1024)
	if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_SNDBUF,
		bLen); err != nil {
		log.Error("SetsockoptInt, SO_SNDBUF", err)
	}
	if bl, err := syscall.GetsockoptInt(fd, syscall.SOL_SOCKET,
		syscall.SO_SNDBUF); err == nil {
		log.Infof("Socket SNDBUF is %d Kb", bl/1024)
	}
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
	var sVal [4]byte
	//var sVal string
	if ifn == nil {
		return nil
	}
	if ifAddr, err := getIfAddr(ifn); err != nil {
		return err
	} else {
		//sVal = string(ifAddr.To4())
		copy(sVal[:], ifAddr.To4())
		log.Info("Set out Multicast interface to", ifAddr)
	}
	/*
		err := syscall.SetsockoptString(fd, syscall.IPPROTO_IP,
			syscall.IP_MULTICAST_IF, sVal)
	*/
	err := syscall.SetsockoptInet4Addr(fd, syscall.IPPROTO_IP,
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

func SetMulticastLoop(fd int, bLoop bool) error {
	var iVal = 0
	if bLoop {
		iVal = 1
	}
	return syscall.SetsockoptInt(fd, syscall.IPPROTO_IP,
		syscall.IP_MULTICAST_LOOP, iVal)
}

func SetBroadcast(fd int, bLoop bool) error {
	var iVal = 0
	if bLoop {
		iVal = 1
	}
	return syscall.SetsockoptInt(fd, syscall.SOL_SOCKET,
		syscall.SO_BROADCAST, iVal)
}
