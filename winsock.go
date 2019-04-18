// +build windows

package MoldUDP

import (
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/kjx98/go-mold/nettypes"
)

//#cgo LDFLAGS: -lws2_32
//#include <sys/types.h>          /* See NOTES */
//#include <winsock2.h>
//#include <ws2tcpip.h>
//#include <unistd.h>
//#include <string.h>
//#include <errno.h>
/*
int inline errNo() { return errno; }

static inline void newSockaddrIn(int port, const void *addr, struct sockaddr_in *saddr)
{
	saddr->sin_family = AF_INET;
	saddr->sin_port = htons(port);
	memcpy(& saddr->sin_addr, addr, 4);
}

static inline int iniSocket() {
	WSADATA	wsd;
	return WSAStartup(MAKEWORD(2,2), &wsd);
}

static inline void copyAddr(struct sockaddr_in *addr, void *dstAddr) {
	memcpy(dstAddr, &addr->sin_addr, 4);
}
*/
import "C"

func init() {
	if C.iniSocket() != 0 {
		panic("WSAStartup")
	}
}

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

type ipHeader struct {
	IhlVer                byte
	tos                   byte
	tot_len, id, frag_off uint16
	ttl, protocol         byte
	check                 uint16
	saddr                 [4]byte
	daddr                 [4]byte
}

func buildRawUDP(buff []byte, udpLen int, port int, src, dst []byte) {
	// set MACEtherType to IPv4
	buff[12] = 8
	buff[13] = 0
	buildIP(buff[14:], udpLen, src, dst)
	ip := nettypes.IPv4_P(buff[14:])
	ckSum := ip.CalculateChecksum()
	buff[14+10] = byte(ckSum >> 8)
	buff[14+11] = byte(ckSum & 0xff)
	buildUDP(buff[14+20:], port, udpLen)
}

func buildIP(buff []byte, udpLen int, src, dst []byte) {
	ipHdr := (*ipHeader)(unsafe.Pointer(&buff[0]))
	ipHdr.IhlVer = 0x45
	ipHdr.tos = 0
	ipHdr.tot_len = uint16(C.htons(C.ushort(udpLen + 28)))
	ipHdr.id = 0
	ipHdr.frag_off = uint16(C.htons(0x4000))
	ipHdr.ttl = 2
	ipHdr.protocol = 0x11
	ipHdr.check = 0
	copy(ipHdr.saddr[:], src)
	copy(ipHdr.daddr[:], dst)
}

type udpHeader struct {
	Source, Dest, Len, Check uint16
}

func buildUDP(buff []byte, dstPort, dataLen int) {
	udpHdr := (*udpHeader)(unsafe.Pointer(&buff[0]))
	udpHdr.Source = uint16(C.htons(C.ushort(dstPort + 1)))
	udpHdr.Dest = uint16(C.htons(C.ushort(dstPort)))
	udpHdr.Len = uint16(C.htons(C.ushort(dataLen + 8)))
	udpHdr.Check = 0
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

func GetsockoptInt(fd, level, opt int) (value int, err error) {
	optLen := C.int(unsafe.Sizeof(value))
	ret := C.getsockopt(C.SOCKET(fd), C.int(level), C.int(opt),
		(*C.char)(unsafe.Pointer(&value)), &optLen)
	if ret != 0 {
		err = syscall.Errno(C.errNo())
	}
	return
}

func SetsockoptInt(fd, level, opt, val int) (err error) {
	optLen := C.int(unsafe.Sizeof(val))
	ret := C.setsockopt(C.SOCKET(fd), C.int(level), C.int(opt),
		(*C.char)(unsafe.Pointer(&val)), optLen)
	if ret != 0 {
		err = syscall.Errno(C.errNo())
	}
	return
}

func Socket(domain, typ, proto int) (fd int, err error) {
	fd = int(C.socket(C.int(domain), C.int(typ), C.int(proto)))
	//fd = int(C.socket(C.int(domain), C.int(typ)|C.SOCK_NONBLOCK, C.int(proto)))
	if fd < 0 {
		err = syscall.Errno(C.errNo())
	}
	return
}

func Close(fd int) (err error) {
	ret := C.closesocket(C.SOCKET(fd))
	if ret < 0 {
		err = syscall.Errno(C.errNo())
	}
	C.WSACleanup()
	return
}

type SockaddrInet4 struct {
	Port int
	Addr [4]byte
}

type HardwareAddr []byte

func (adr HardwareAddr) String() string {
	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", adr[0], adr[1],
		adr[2], adr[3], adr[4], adr[5])
}

//type SockaddrInet4 = syscall.SockaddrInet4

func (adr *SockaddrInet4) IP() string {
	return fmt.Sprintf("%d.%d.%d.%d", adr.Addr[0], adr.Addr[1], adr.Addr[2],
		adr.Addr[3])
}

func (adr *SockaddrInet4) String() string {
	return fmt.Sprintf("%d.%d.%d.%d:%d", adr.Addr[0], adr.Addr[1],
		adr.Addr[2], adr.Addr[3], adr.Port)
}

func LocalAddr(fd int) *SockaddrInet4 {
	saddr := C.struct_sockaddr_in{}
	aLen := C.socklen_t(unsafe.Sizeof(saddr))
	if C.getsockname(C.SOCKET(fd), (*C.struct_sockaddr)(unsafe.Pointer(&saddr)), &aLen) < 0 {
		return nil
	}
	laddr := &SockaddrInet4{Port: int(C.ntohs(saddr.sin_port))}
	C.copyAddr(&saddr, unsafe.Pointer(&laddr.Addr[0]))
	return laddr
}

func Bind(fd int, laddr *SockaddrInet4) (err error) {
	saddr := C.struct_sockaddr_in{}
	C.newSockaddrIn(C.int(laddr.Port), unsafe.Pointer(&laddr.Addr[0]), &saddr)
	//saddr.sin_family = C.AF_INET
	//saddr.sin_port = C.htons(C.ushort(port))
	ret := C.bind(C.SOCKET(fd), (*C.struct_sockaddr)(unsafe.Pointer(&saddr)),
		C.socklen_t(unsafe.Sizeof(saddr)))
	if ret < 0 {
		err = syscall.Errno(C.errNo())
	}
	return
}

func Recvfrom(fd int, p []byte, flags int) (n int, from *SockaddrInet4, err error) {
	raddr := C.struct_sockaddr_in{}
	raddrLen := C.socklen_t(unsafe.Sizeof(raddr))
	ret := C.recvfrom(C.SOCKET(fd), (*C.char)(unsafe.Pointer(&p[0])),
		C.int(len(p)), C.int(flags),
		(*C.struct_sockaddr)(unsafe.Pointer(&raddr)), &raddrLen)
	if ret < 0 {
		errN := C.errNo()
		if errN != 0 && errN != C.EAGAIN && errN != C.EWOULDBLOCK {
			err = syscall.Errno(C.errNo())
		}
	} else {
		n = int(ret)
	}
	from = &SockaddrInet4{Port: int(C.ntohs(raddr.sin_port))}
	C.copyAddr(&raddr, unsafe.Pointer(&from.Addr[0]))
	return
}

func Sendto(fd int, p []byte, flags int, to *SockaddrInet4) (ret int, err error) {
	taddr := C.struct_sockaddr_in{}
	C.newSockaddrIn(C.int(to.Port), unsafe.Pointer(&to.Addr[0]), &taddr)
	ret = int(C.sendto(C.SOCKET(fd), (*C.char)(unsafe.Pointer(&p[0])),
		C.int(len(p)), C.int(flags), (*C.struct_sockaddr)(unsafe.Pointer(&taddr)),
		C.int(unsafe.Sizeof(taddr))))
	if ret < 0 {
		errN := C.errNo()
		if errN != 0 && errN != C.EAGAIN && errN != C.EWOULDBLOCK {
			err = syscall.Errno(C.errNo())
		}
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
	log.Infof("Try set Socket SndBuf to %d KB", bLen/1024)
	if err := SetsockoptInt(fd, C.SOL_SOCKET, C.SO_SNDBUF, bLen); err != nil {
		log.Error("SetsockoptInt, SO_SNDBUF", err)
	}
	if bl, err := GetsockoptInt(fd, C.SOL_SOCKET, C.SO_SNDBUF); err == nil {
		log.Infof("Socket SNDBUF is %d Kb", bl/1024)
	}
}

//加入组播域
func JoinMulticast(fd int, maddr []byte, ifn *net.Interface) (err error) {
	var mreq = [8]byte{}
	copy(mreq[:4], maddr)
	if ifn != nil {
		if adr, err := getIfAddr(ifn); err == nil {
			copy(mreq[4:], adr.To4())
			log.Infof("Use %s for Multicast interface", adr)
		}
	} else {
		// try os.Getenv
		laddr := os.Getenv("LADDR")
		log.Info("Try joinMC env LADDR:", laddr)
		if adr := net.ParseIP(laddr); adr != nil {
			copy(mreq[4:], adr.To4())
			log.Infof("Use %s for Multicast interface", adr)
		}
	}
	optLen := C.int(unsafe.Sizeof(mreq))
	res := C.setsockopt(C.SOCKET(fd), C.IPPROTO_IP, C.IP_ADD_MEMBERSHIP,
		(*C.char)(unsafe.Pointer(&mreq)), optLen)
	if res != 0 {
		err = syscall.Errno(C.errNo())
	}
	return
}

func SetMulticastInterface(fd int, ifn *net.Interface) (err error) {
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
	optLen := C.int(unsafe.Sizeof(sVal))
	res := C.setsockopt(C.SOCKET(fd), C.IPPROTO_IP, C.IP_MULTICAST_IF,
		(*C.char)(unsafe.Pointer(&sVal)), optLen)
	if res != 0 {
		err = syscall.Errno(C.errNo())
	}
	return
}

//退出组播域
func ExitMulticast(fd int, maddr net.IP) {
	var mreq = [8]byte{}
	optLen := C.int(unsafe.Sizeof(mreq))
	copy(mreq[:4], maddr.To4())
	C.setsockopt(C.SOCKET(fd), C.IPPROTO_IP, C.IP_DROP_MEMBERSHIP,
		(*C.char)(unsafe.Pointer(&mreq)), optLen)
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
