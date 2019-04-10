package MoldUDP

import (
	"fmt"
	"net"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

//#cgo LDFLAGS: -ldl
//#include <sys/types.h>          /* See NOTES */
//#include <sys/socket.h>
//#include <netinet/in.h>
//#include <unistd.h>
//#include <string.h>
//#include <errno.h>
/*
#ifndef	_GNU_SOURCE
struct mmsghdr {
    struct msghdr msg_hdr;
    unsigned int msg_len;
};
extern int sendmmsg (int __fd, struct mmsghdr *__vmessages,
			unsigned int __vlen, int __flags);
extern int recvmmsg(int sockfd, struct mmsghdr *msgvec, unsigned int vlen,
			unsigned int flags, struct timespec *timeout);
#endif

#define	MAX_BATCH	64
#define	MAX_PACKET	1472
int inline errNo() { return errno; }

inline void newSockaddrIn(int port, const void *addr, struct sockaddr_in *saddr)
{
	saddr->sin_family = AF_INET;
	saddr->sin_port = htons(port);
	memcpy(& saddr->sin_addr, addr, 4);
}

inline void copyAddr(struct sockaddr_in *addr, void *dstAddr) {
	memcpy(dstAddr, &addr->sin_addr, 4);
}

struct	iovec iovec[MAX_BATCH][1];
struct	mmsghdr	dgrams[MAX_BATCH];
*/
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

func GetsockoptInt(fd, level, opt int) (value int, err error) {
	optLen := C.uint(unsafe.Sizeof(value))
	ret := C.getsockopt(C.int(fd), C.int(level), C.int(opt),
		unsafe.Pointer(&value), &optLen)
	if ret != 0 {
		err = syscall.Errno(C.errNo())
	}
	return
}

func SetsockoptInt(fd, level, opt, val int) (err error) {
	optLen := C.uint(unsafe.Sizeof(val))
	ret := C.setsockopt(C.int(fd), C.int(level), C.int(opt),
		unsafe.Pointer(&val), optLen)
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
	ret := C.close(C.int(fd))
	if ret < 0 {
		err = syscall.Errno(C.errNo())
	}
	return
}

type SockaddrInet4 struct {
	Port int
	Addr [4]byte
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
	if C.getsockname(C.int(fd), (*C.struct_sockaddr)(unsafe.Pointer(&saddr)), &aLen) < 0 {
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
	ret := C.bind(C.int(fd), (*C.struct_sockaddr)(unsafe.Pointer(&saddr)),
		C.socklen_t(unsafe.Sizeof(saddr)))
	if ret < 0 {
		err = syscall.Errno(C.errNo())
	}
	return
}

func Recvfrom(fd int, p []byte, flags int) (n int, from *SockaddrInet4, err error) {
	raddr := C.struct_sockaddr_in{}
	raddrLen := C.socklen_t(unsafe.Sizeof(raddr))
	ret := C.recvfrom(C.int(fd), unsafe.Pointer(&p[0]), C.size_t(len(p)),
		C.int(flags), (*C.struct_sockaddr)(unsafe.Pointer(&raddr)), &raddrLen)
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

func Sendto(fd int, p []byte, flags int, to *SockaddrInet4) (int, error) {
	taddr := C.struct_sockaddr_in{}
	C.newSockaddrIn(C.int(to.Port), unsafe.Pointer(&to.Addr[0]), &taddr)
	ret := C.sendto(C.int(fd), unsafe.Pointer(&p[0]), C.size_t(len(p)),
		C.int(flags), (*C.struct_sockaddr)(unsafe.Pointer(&taddr)),
		C.uint(unsafe.Sizeof(taddr)))
	var err error
	if ret < 0 {
		errN := C.errNo()
		if errN != 0 && errN != C.EAGAIN && errN != C.EWOULDBLOCK {
			err = syscall.Errno(C.errNo())
		}
	}
	return int(ret), err
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
	}
	optLen := C.uint(unsafe.Sizeof(mreq))
	res := C.setsockopt(C.int(fd), C.IPPROTO_IP, C.IP_ADD_MEMBERSHIP,
		unsafe.Pointer(&mreq), optLen)
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
	optLen := C.uint(unsafe.Sizeof(sVal))
	res := C.setsockopt(C.int(fd), C.IPPROTO_IP, C.IP_MULTICAST_IF,
		unsafe.Pointer(&sVal), optLen)
	if res != 0 {
		err = syscall.Errno(C.errNo())
	}
	return
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

func Sendmmsg(fd int, bufs []Packet, to *SockaddrInet4) (cnt int, err error) {
	taddr := C.struct_sockaddr_in{}
	C.newSockaddrIn(C.int(to.Port), unsafe.Pointer(&to.Addr[0]), &taddr)
	bSize := len(bufs)
	if bSize > C.MAX_BATCH {
		bSize = C.MAX_BATCH
	}
	for i := 0; i < bSize; i++ {
		buf := bufs[i]
		C.iovec[i][0].iov_base = unsafe.Pointer(&buf[0])
		C.iovec[i][0].iov_len = C.size_t(len(buf))
		C.dgrams[i].msg_len = C.uint(len(buf))
		C.dgrams[i].msg_hdr.msg_iov = &(C.iovec[i][0])
		C.dgrams[i].msg_hdr.msg_iovlen = 1
		C.dgrams[i].msg_hdr.msg_name = unsafe.Pointer(&taddr)
		C.dgrams[i].msg_hdr.msg_namelen = C.socklen_t(unsafe.Sizeof(taddr))
	}
	res := C.sendmmsg(C.int(fd), &(C.dgrams[0]), C.uint(bSize), 0)
	if res < 0 {
		err = syscall.Errno(C.errNo())
	} else {
		cnt = int(res)
	}
	return
}

func Recvmmsg(fd int, bufs []Packet, flags int) (cnt int, from *SockaddrInet4, err error) {
	raddr := C.struct_sockaddr_in{}
	raddrLen := C.socklen_t(unsafe.Sizeof(raddr))
	bSize := len(bufs)
	if bSize > C.MAX_BATCH {
		bSize = C.MAX_BATCH
	}
	C.dgrams[0].msg_hdr.msg_name = unsafe.Pointer(&raddr)
	C.dgrams[0].msg_hdr.msg_namelen = raddrLen
	for i := 0; i < bSize; i++ {
		buf := bufs[i]
		C.iovec[i][0].iov_base = unsafe.Pointer(&buf[0])
		C.iovec[i][0].iov_len = C.size_t(len(buf))
		C.dgrams[i].msg_hdr.msg_iov = &(C.iovec[i][0])
		C.dgrams[i].msg_hdr.msg_iovlen = 1
		C.dgrams[i].msg_len = C.uint(len(buf))
		if i == 0 {
			continue
		}
		C.dgrams[i].msg_hdr.msg_name = C.NULL
		C.dgrams[i].msg_hdr.msg_namelen = 0
	}
	res := C.recvmmsg(C.int(fd), &(C.dgrams[0]), C.uint(bSize), 0,
		(*C.struct_timespec)(C.NULL))
	if res < 0 {
		errN := C.errNo()
		if errN != 0 && errN != C.EAGAIN && errN != C.EWOULDBLOCK {
			err = syscall.Errno(C.errNo())
		}
	} else {
		cnt = int(res)
		for i := 0; i < cnt; i++ {
			buf := bufs[i]
			bLen := int(C.dgrams[i].msg_len)
			bufs[i] = buf[:bLen]
		}
	}
	from = &SockaddrInet4{Port: int(C.ntohs(raddr.sin_port))}
	C.copyAddr(&raddr, unsafe.Pointer(&from.Addr[0]))
	return
}
