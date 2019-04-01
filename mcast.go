package MoldUDP

import (
	"errors"
	"net"
	"runtime"
	"strings"
	"time"
	"unsafe"
)

//#cgo LDFLAGS: -ldl
//#include <sys/types.h>          /* See NOTES */
//#include <sys/socket.h>
//#include <netinet/in.h>
import "C"

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

var (
	errGetsockopt = errors.New("getsockopt error")
	errSetsockopt = errors.New("setsockopt error")
	errSocket     = errors.New("socket error")
	errBind       = errors.New("bind error")
)

func GetsockoptInt(fd, level, opt int) (value int, err error) {
	optLen := C.uint(unsafe.Sizeof(value))
	ret := C.getsockopt(C.int(fd), C.int(level), C.int(opt),
		unsafe.Pointer(&value), &optLen)
	if ret != 0 {
		err = errGetsockopt
	}
	return
}

func SetsockoptInt(fd, level, opt, val int) error {
	optLen := C.uint(unsafe.Sizeof(val))
	ret := C.setsockopt(C.int(fd), C.int(level), C.int(opt),
		unsafe.Pointer(&val), optLen)
	if ret != 0 {
		return errSetsockopt
	}
	return nil
}

func Socket(domain, typ, proto int) (fd int, err error) {
	fd = int(C.socket(C.int(domain), C.int(typ), C.int(proto)))
	if fd < 0 {
		err = errSocket
	}
	return
}

func MyBind(fd int, port int) (err error) {
	saddr := C.struct_sockaddr_in{}
	saddr.sin_family = C.AF_INET
	saddr.sin_port = C.htons(C.ushort(port))
	ret := C.bind(C.int(fd), (*C.struct_sockaddr)(unsafe.Pointer(&saddr)),
		C.socklen_t(unsafe.Sizeof(saddr)))
	if ret < 0 {
		err = errBind
	}
	return
}

func ReserveRecvBuf(fd int) {
	bLen := 4 * 1024 * 1024
	if bl, err := GetsockoptInt(fd, C.SOL_SOCKET, C.SO_RCVBUF); err == nil {
		log.Infof("Socket RCVBUF is %d Kb", bl/1024)
	}
	log.Infof("Try set Socket RcvBuf to %d KB", bLen/1024)
	if err := SetsockoptInt(fd, C.SOL_SOCKET, C.SO_RCVBUF, bLen); err != nil {
		log.Error("SetsockoptInt, SO_RCVBUF", err)
	}
	if bl, err := GetsockoptInt(fd, C.SOL_SOCKET, C.SO_RCVBUF); err == nil {
		log.Infof("Socket RCVBUF is %d Kb", bl/1024)
	}
}

func ReserveSendBuf(fd int) {
	bLen := 2 * 1024 * 1024
	if bl, err := GetsockoptInt(fd, C.SOL_SOCKET, C.SO_SNDBUF); err == nil {
		log.Infof("Socket SNDBUF is %d Kb", bl/1024)
	}
	log.Infof("Try set Socket RcvBuf to %d KB", bLen/1024)
	if err := SetsockoptInt(fd, C.SOL_SOCKET, C.SO_SNDBUF, bLen); err != nil {
		log.Error("SetsockoptInt, SO_SNDBUF", err)
	}
	if bl, err := GetsockoptInt(fd, C.SOL_SOCKET, C.SO_SNDBUF); err == nil {
		log.Infof("Socket SNDBUF is %d Kb", bl/1024)
	}
}

//加入组播域
func JoinMulticast(fd int, maddr []byte, ifn *net.Interface) error {
	var mreq = [8]byte{}
	copy(mreq[:4], maddr)
	if ifn != nil {
		if adr, err := getIfAddr(ifn); err == nil {
			copy(mreq[4:], adr.To4())
			log.Infof("Use %s for Multicast interface", adr)
		}
	}
	optLen := C.uint(unsafe.Sizeof(mreq))
	res := C.setsockopt(C.int(fd), C.IPPROTO_IP, C.IP_ADD_MEMBERSHIP,
		unsafe.Pointer(&mreq), optLen)
	if res != 0 {
		return errSetsockopt
	}
	return nil
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
	optLen := C.uint(unsafe.Sizeof(sVal))
	res := C.setsockopt(C.int(fd), C.IPPROTO_IP, C.IP_MULTICAST_IF,
		unsafe.Pointer(&sVal), optLen)
	if res != 0 {
		return errSetsockopt
	}
	return nil
}

//退出组播域
func ExitMulticast(fd int, maddr net.IP) {
	var mreq = [8]byte{}
	optLen := C.uint(unsafe.Sizeof(mreq))
	copy(mreq[:4], maddr.To4())
	C.setsockopt(C.int(fd), C.IPPROTO_IP, C.IP_DROP_MEMBERSHIP,
		unsafe.Pointer(&mreq), optLen)
}

//设置路由的TTL值
func SetMulticastTTL(fd, ttl int) error {
	return SetsockoptInt(fd, C.IPPROTO_IP, C.IP_MULTICAST_TTL, ttl)
}

func SetMulticastLoop(fd int, bLoop bool) error {
	var iVal = 0
	if bLoop {
		iVal = 1
	}
	return SetsockoptInt(fd, C.IPPROTO_IP, C.IP_MULTICAST_LOOP, iVal)
}

func SetBroadcast(fd int, bLoop bool) error {
	var iVal = 0
	if bLoop {
		iVal = 1
	}
	return SetsockoptInt(fd, C.SOL_SOCKET, C.SO_BROADCAST, iVal)
}
