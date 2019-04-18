// +build linux

package MoldUDP

import (
	"net"
	"syscall"
	"unsafe"
)

//#cgo LDFLAGS: -ldl
//#include <sys/types.h>          /* See NOTES */
//#include <sys/socket.h>
//#include <netinet/in.h>
//#include <netinet/ip.h>
//#include <net/ethernet.h>
//#include <linux/if_packet.h>
//#include <linux/filter.h>
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
extern int errNo();

struct timespec timeo={0,1000000};
struct	iovec iovec[MAX_BATCH][2];
struct	mmsghdr	dgrams[MAX_BATCH];
struct sock_filter filter[]={
{ 0x28, 0, 0, 0x0000000c },
{ 0x15, 0, 4, 0x000086dd },
{ 0x30, 0, 0, 0x00000014 },
{ 0x15, 0, 11, 0x00000011 },
{ 0x28, 0, 0, 0x00000038 },
{ 0x15, 8, 9, 0x000016e2 },	// dst udp port
{ 0x15, 0, 8, 0x00000800 },
{ 0x30, 0, 0, 0x00000017 },
{ 0x15, 0, 6, 0x00000011 },
{ 0x28, 0, 0, 0x00000014 },
{ 0x45, 4, 0, 0x00001fff },
{ 0xb1, 0, 0, 0x0000000e },
{ 0x48, 0, 0, 0x00000010 },
{ 0x15, 0, 1, 0x000016e2 },	// dst udp port
{ 0x6, 0, 0, 0x00040000 },
{ 0x6, 0, 0, 0x00000000 },
};

inline int setBPF(int fd) {
	struct sock_fprog	fProg;
	fProg.len = sizeof(filter)/sizeof(filter[0]);
	fProg.filter = filter;
	return setsockopt(fd, SOL_SOCKET, SO_ATTACH_FILTER, &fProg, sizeof(fProg));
}

inline int setPacketMultiCast(int fd, int ifIndex, unsigned char *ipAddr) {
	struct packet_mreq mreq;
	mreq.mr_ifindex =  ifIndex;
	mreq.mr_type = PACKET_MR_MULTICAST; // PACKET_MR_ALLMULTI
	mreq.mr_alen = 6;
	mreq.mr_address[0] = 1;
	mreq.mr_address[1] = 0;
	mreq.mr_address[2] = 0x5e;
	mreq.mr_address[3] = ipAddr[1] & 0x7f;
	mreq.mr_address[4] = ipAddr[2];
	mreq.mr_address[5] = ipAddr[3];
	return setsockopt(fd, SOL_PACKET, PACKET_ADD_MEMBERSHIP, &mreq,
				sizeof(mreq));
}

inline void setSockaddrl2(struct sockaddr_ll *sll, void *sbuf, int ifIndex) {
	sll->sll_family = AF_PACKET;
	sll->sll_protocol = htons(ETH_P_IP);
	sll->sll_ifindex = ifIndex;
	sll->sll_halen = ETH_ALEN;
	memcpy(sll->sll_addr, sbuf, ETH_ALEN);
}

static inline void newSockaddrIn(int port, const void *addr, struct sockaddr_in *saddr)
{
	saddr->sin_family = AF_INET;
	saddr->sin_port = htons(port);
	memcpy(& saddr->sin_addr, addr, 4);
}

static inline void copyAddr(struct sockaddr_in *addr, void *dstAddr) {
	memcpy(dstAddr, &addr->sin_addr, 4);
}
*/
import "C"

func setBPF(fd, port int) (err error) {
	if int(C.filter[5].k) == port && int(C.filter[13].k) == port {
		log.Info("already set dst port filter to", port)
	} else {
		C.filter[5].k = C.__u32(port)
		C.filter[13].k = C.__u32(port)
	}
	ret := C.setBPF(C.int(fd))
	if ret != 0 {
		err = syscall.Errno(C.errNo())
	}
	return
}

func GetMulticastHWAddr(adr net.IP) HardwareAddr {
	if ip4 := adr.To4(); ip4 == nil {
		return nil
	} else {
		ret := make([]byte, 6)
		ret[0] = 1
		ret[1] = 0
		ret[2] = 0x5e
		ret[3] = ip4[1] & 0x7f
		ret[4] = ip4[2]
		ret[5] = ip4[3]
		return HardwareAddr(ret)
	}
}

func JoinPacketMulticast(fd int, maddr []byte, ifn *net.Interface) (err error) {
	ret := C.setPacketMultiCast(C.int(fd), C.int(ifn.Index),
		(*C.uchar)(unsafe.Pointer(&maddr[0])))
	if ret != 0 {
		err = syscall.Errno(C.errNo())
	}
	return
}

func Sendmmsg2(fd int, bufs []Packet, pktHdr []byte, ifIndex int) (cnt int, err error) {
	taddr := C.struct_sockaddr_ll{}
	C.setSockaddrl2(&taddr, unsafe.Pointer(&pktHdr[0]), C.int(ifIndex))
	bSize := len(bufs)
	if bSize > C.MAX_BATCH {
		bSize = C.MAX_BATCH
	}
	for i := 0; i < bSize; i++ {
		buf := bufs[i]
		C.iovec[i][0].iov_base = unsafe.Pointer(&pktHdr[0])
		C.iovec[i][0].iov_len = C.size_t(len(pktHdr))
		C.iovec[i][1].iov_base = unsafe.Pointer(&buf[0])
		C.iovec[i][1].iov_len = C.size_t(len(buf))
		C.dgrams[i].msg_len = C.uint(len(buf) + len(pktHdr))
		C.dgrams[i].msg_hdr.msg_iov = &(C.iovec[i][0])
		C.dgrams[i].msg_hdr.msg_iovlen = 2
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
	// timeo set to 1 ms
	res := C.recvmmsg(C.int(fd), &(C.dgrams[0]), C.uint(bSize), 0,
		&(C.timeo))
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
