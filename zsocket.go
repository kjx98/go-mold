// +build linux

package MoldUDP

import (
	"fmt"
	"os"
	"runtime"
	"sync/atomic"
	"syscall"
	"unsafe"
)

/*
#include <sys/types.h>
#include <linux/if_packet.h>  // AF_PACKET, sockaddr_ll
#include <linux/if_ether.h>  // ETH_P_ALL
#include <sys/socket.h>  // socket()
#include <unistd.h>  // close()
#include <string.h>
#include <arpa/inet.h>  // htons()
#include <sys/mman.h>  // mmap(), munmap()
#include <errno.h>
#include <poll.h>  // poll()

struct block_desc {
	uint32_t version;
	uint32_t offset_to_priv;
	struct tpacket_hdr_v1 h1;
};

#define	SOCKADDR_START	TPACKET_ALIGN(sizeof(struct tpacket_hdr))
//#define	TX_START	TPACKET_ALIGN(TPACKET_HDRLEN)
//#define	TX_START	TPACKET_HDRLEN
#define	TX_START		TPACKET_ALIGN(sizeof(struct tpacket_hdr))
void setDstAddr(char *buff, int ifIndex) {
	struct sockaddr_ll *sAddr = (struct sockaddr_ll *)(buff + SOCKADDR_START);
	memset(sAddr, 0, sizeof(*sAddr));
	sAddr->sll_family = AF_PACKET;
	//sAddr->sll_protocol = htons(ETH_P_IP);
	sAddr->sll_ifindex = ifIndex;
	sAddr->sll_halen = 6;
	//memcpy(sAddr->sll_addr, buff+TX_START, 6);
}
*/
import "C"

const (
	TPACKET_ALIGNMENT = uint(C.TPACKET_ALIGNMENT)
	framesPerBlock    = 1024

	MINIMUM_FRAME_SIZE = TPACKET_ALIGNMENT << 6
	MAXIMUM_FRAME_SIZE = TPACKET_ALIGNMENT << 11

	ENABLE_RX       = 1 << 0
	ENABLE_TX       = 1 << 1
	DISABLE_TX_LOSS = 1 << 2
)

func tpAlign(x int) int {
	return int((uint(x) + TPACKET_ALIGNMENT - 1) &^ (TPACKET_ALIGNMENT - 1))
}

const (
	_ETH_ALEN = C.ETH_ALEN //6
	ETH_ALL   = C.ETH_P_ALL
	ETH_IP    = C.ETH_P_IP
	ETH_IPX   = C.ETH_P_IPX
	ETH_IPV6  = C.ETH_P_IPV6

	//_PACKET_VERSION = 0xa
	//_PACKET_RX_RING = 0x5
	//_PACKET_TX_RING = 0xd
	//_PACKET_LOSS    = 0xe

	//_TPACKET_V1 = 0
	// tp_status in <linux/if_packet.h>
	/* rx status */
	/* tx status */
	/* tx and rx status */
	_TP_STATUS_TS_RAW_HARDWARE = 1 << 31
	/* poll events */
)

var (
	_TX_START    int
	_ADDR_START  int
	nPacketSent  int
	nPacketWrong int
)

// OptTPacketVersion is the version of TPacket to use.
// It can be passed into NewTPacket.
type OptTPacketVersion int

// String returns a string representation of the version, generally of the form V#.
func (t OptTPacketVersion) String() string {
	switch t {
	case TPacketVersion1:
		return "V1"
	case TPacketVersion2:
		return "V2"
	case TPacketVersion3:
		return "V3"
	case TPacketVersionHighestAvailable:
		return "HighestAvailable"
	}
	return "InvalidVersion"
}

// TPacket version numbers for use with NewHandle.
const (
	// TPacketVersionHighestAvailable tells NewHandle to use the highest available version of tpacket the kernel has available.
	// This is the default, should a version number not be given in NewHandle's options.
	TPacketVersionHighestAvailable = OptTPacketVersion(-1)
	TPacketVersion1                = OptTPacketVersion(C.TPACKET_V1)
	TPacketVersion2                = OptTPacketVersion(C.TPACKET_V2)
	TPacketVersion3                = OptTPacketVersion(C.TPACKET_V3)
	tpacketVersionMax              = TPacketVersion3
	tpacketVersionMin              = -1
)

// Stats is a set of counters detailing the work TPacket has done so far.
type Stats struct {
	// Packets is the total number of packets returned to the caller.
	Packets int64
	// Polls is the number of blocking syscalls made waiting for packets.
	// This should always be <= Packets, since with TPacket one syscall
	// can (and often does) return many results.
	Polls int64
}

func init() {
	_ADDR_START = int(C.SOCKADDR_START)
	_TX_START = int(C.TX_START)
}

func PacketOffset() int {
	return _TX_START
}

func errnoErr(e syscall.Errno) error {
	switch e {
	case 0:
		return nil
	case syscall.EAGAIN:
		return fmt.Errorf("try again")
	case syscall.EINVAL:
		return fmt.Errorf("invalid argument")
	case syscall.ENOENT:
		return fmt.Errorf("no such file or directory")
	}
	return e
}

func copyFx(dst, src []byte, len int) uint16 {
	copy(dst, src)
	return uint16(len)
}

type CallbackFunc func([]byte, uint16, uint16)

// IZSocket is an interface for interacting with eth-iface like
// objects in the ZSocket code-base. This has basically
// simply enabled the FakeInterface code to work.
type IZSocket interface {
	MaxPackets() int32
	MaxPacketSize() uint16
	WrittenPackets() int32
	Listen(fx CallbackFunc)
	WriteToBuffer(buf []byte, l uint16) (int32, error)
	CopyToBuffer(buf []byte, l uint16, copyFx func(dst, src []byte, l uint16)) (int32, error)
	FlushFrames() (uint, error, []error)
	Close() error
}

// ZSocket opens a zero copy ring-buffer to the specified interface.
// Do not manually initialize ZSocket. Use `NewZSocket`.
type ZSocket struct {
	socket         int
	ifIndex        int
	version        OptTPacketVersion
	stats          Stats
	numBlocks      int
	blockSize      int
	raw            []byte
	nPackets       uint
	nDrops         uint
	listening      int32
	frameNum       int32
	frameSize      uint16
	rxEnabled      bool
	rxFrames       []*ringFrame
	txEnabled      bool
	txLossDisabled bool
	txFrameSize    uint16
	txIndex        int32
	txWritten      int32
	txWrittenIndex int32
	txFrames       []*ringFrame
}

// NewZSocket opens a "ZSocket" on the specificed interface
// (by interfaceIndex). Whether the TX ring, RX ring, or both
// are enabled are options that can be passed. Additionally,
// an option can be passed that will tell the kernel to pay
// attention to packet faults, called DISABLE_TX_LOSS.
func NewZSocket(ethIndex, options int, maxFrameSize, maxTotalFrames uint, ethType int) (*ZSocket, error) {
	if maxFrameSize < MINIMUM_FRAME_SIZE ||
		maxFrameSize > MAXIMUM_FRAME_SIZE ||
		(maxFrameSize&(maxFrameSize-1)) > 0 {
		return nil, fmt.Errorf("maxFrameSize must be at least %d (MINIMUM_FRAME_SIZE), be at most %d (MAXIMUM_FRAME_SIZE), and be a power of 2",
			MINIMUM_FRAME_SIZE, MAXIMUM_FRAME_SIZE)
	}
	if maxTotalFrames < 16 && maxTotalFrames%8 == 0 {
		return nil, fmt.Errorf("maxTotalFrames must be at least 16, and be a multiple of 8")
	}

	log.Info("AF_PACKET Ring TX_START", _TX_START, "ADDR_START", _ADDR_START)

	zs := new(ZSocket)
	zs.rxEnabled = options&ENABLE_RX == ENABLE_RX
	zs.txEnabled = options&ENABLE_TX == ENABLE_TX
	zs.txLossDisabled = options&DISABLE_TX_LOSS == DISABLE_TX_LOSS
	eT := int(C.htons(C.ushort(ethType)))
	if zs.txEnabled {
		// disable recv for transmit mode
		zs.rxEnabled = false
		eT = 0
	}
	// in Linux PF_PACKET is actually defined by AF_PACKET.
	// SOCK_DGRAM not work, no packet listened
	//sock, err := Socket(C.AF_PACKET, C.SOCK_DGRAM, eT)
	sock, err := Socket(C.AF_PACKET, C.SOCK_RAW, eT)
	if err != nil {
		log.Error("socket AF_PACKET", err)
		return nil, err
	}
	zs.socket = sock
	zs.ifIndex = ethIndex
	sll := syscall.SockaddrLinklayer{}
	sll.Protocol = uint16(eT)
	sll.Ifindex = ethIndex
	sll.Halen = C.ETH_ALEN
	if err := syscall.Bind(sock, &sll); err != nil {
		log.Error("bind AF_PACKET", err)
		return nil, err
	}
	zs.version = TPacketVersion1

	if vv, err := GetsockoptInt(sock, C.SOL_PACKET, C.PACKET_VERSION); err == nil {
		log.Info("PACKET_VERSION is", vv)
		if zs.rxEnabled && vv != int(TPacketVersion3) {
			if err := SetsockoptInt(sock, C.SOL_PACKET, C.PACKET_VERSION, C.TPACKET_V3); err != nil {
				log.Error("Set PACKET_VERSION", err)
				SetsockoptInt(sock, C.SOL_PACKET, C.PACKET_VERSION, C.TPACKET_V1)
				// set to Packet_version 1
			} else {
				log.Info("Using PACKET_VERSION3 for recv")
				zs.version = TPacketVersion3
			}
		}
	} else {
		log.Error("Get PACKET_VERSION", err)
		return nil, err
	}

	req := &tpacketReq{}
	pageSize := uint(os.Getpagesize())
	var frameSize uint
	if maxFrameSize < pageSize {
		frameSize = calculateLargestFrame(maxFrameSize)
	} else {
		frameSize = (maxFrameSize / pageSize) * pageSize
	}
	req.frameSize = frameSize
	if zs.version == TPacketVersion3 {
		req.blockSize = req.frameSize * framesPerBlock
		req.blockNum = maxTotalFrames / framesPerBlock
		req.frameNum = (req.blockSize / req.frameSize) * req.blockNum
	} else {
		req.blockSize = req.frameSize * 8
		req.blockNum = maxTotalFrames / 8
		req.frameNum = (req.blockSize / req.frameSize) * req.blockNum
	}
	reqP := req.getPointer(zs.version == TPacketVersion3) // for V1, true for V3
	log.Infof("ZSocket %s, blockSize: %d KB, frameSize: %d, numBlock: %d",
		zs.version, req.blockSize/1024, req.frameSize, req.blockNum)

	zs.numBlocks = int(req.blockNum)
	zs.blockSize = int(req.blockSize)
	if zs.rxEnabled {
		_, _, e1 := syscall.Syscall6(uintptr(syscall.SYS_SETSOCKOPT),
			uintptr(sock), uintptr(C.SOL_PACKET), uintptr(C.PACKET_RX_RING),
			uintptr(reqP), uintptr(req.size(zs.version == TPacketVersion3)), 0)
		if e1 != 0 {
			return nil, errnoErr(e1)
		}
	}
	if zs.txEnabled {
		_, _, e1 := syscall.Syscall6(uintptr(syscall.SYS_SETSOCKOPT),
			uintptr(sock), uintptr(C.SOL_PACKET), uintptr(C.PACKET_TX_RING),
			uintptr(reqP), uintptr(req.size(false)), 0)
		if e1 != 0 {
			return nil, errnoErr(e1)
		}
		// Can't get this to work for some reason
		if !zs.txLossDisabled {
			if err := SetsockoptInt(sock, C.SOL_PACKET, C.PACKET_LOSS, 1); err != nil {
				log.Error("setsockopt PACKET_LOSS", err)
				//return nil, err
			}
		}
	}

	size := req.blockSize * req.blockNum
	// never enable both TX and RX
	/*
		if zs.txEnabled && zs.rxEnabled {
			size *= 2
		}
	*/

	bs, err := syscall.Mmap(sock, int64(0), int(size), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED|syscall.MAP_LOCKED|syscall.MAP_POPULATE)
	if err != nil {
		return nil, err
	}
	zs.raw = bs
	zs.frameNum = int32(req.frameNum)
	zs.frameSize = uint16(req.frameSize)
	i := 0
	frLoc := 0
	if zs.rxEnabled {
		switch zs.version {
		case TPacketVersion1, TPacketVersion2:
			for i = 0; i < int(zs.frameNum); i++ {
				frLoc = i * int(zs.frameSize)
				rf := &ringFrame{}
				rf.raw = zs.raw[frLoc : frLoc+int(zs.frameSize)]
				zs.rxFrames = append(zs.rxFrames, rf)
			}
		case TPacketVersion3:
			// no need prepare block header
		}
	}
	if zs.txEnabled {
		zs.txFrameSize = zs.frameSize - uint16(_TX_START)
		zs.txWritten = 0
		zs.txWrittenIndex = -1
		for t := 0; t < int(zs.frameNum); t, i = t+1, i+1 {
			frLoc = i * int(zs.frameSize)
			tx := &ringFrame{}
			tx.raw = zs.raw[frLoc : frLoc+int(zs.frameSize)]
			//tx.txStart = tx.raw[int(C.TPACKET_HDRLEN):]
			tx.txStart = tx.raw[int(C.TX_START):]
			zs.txFrames = append(zs.txFrames, tx)
		}
	}
	zs.updateSocketStats()
	return zs, nil
}

func calculateLargestFrame(ceil uint) uint {
	i := uint(MINIMUM_FRAME_SIZE)
	for i < ceil {
		i <<= 1
	}
	//return (i >> 1)
	return i
}

// Returns fd handle of socket for using lower call
func (zs *ZSocket) Fd() int {
	return zs.socket
}

// Returns the maximum amount of frame packets that can be written
func (zs *ZSocket) MaxPackets() int32 {
	return zs.frameNum
}

// Returns the frame size in bytes
func (zs *ZSocket) MaxPacketSize() uint16 {
	return zs.frameSize - uint16(C.TPACKET_HDRLEN)
}

// Stats returns statistics on the packets the TPacket has seen so far.
func (zs *ZSocket) Stats() (Stats, error) {
	return Stats{
		Polls:   atomic.LoadInt64(&zs.stats.Polls),
		Packets: atomic.LoadInt64(&zs.stats.Packets),
	}, nil
}

// Returns the amount of packets, written to the tx ring, that
// haven't been flushed.
func (zs *ZSocket) WrittenPackets() int32 {
	return atomic.LoadInt32(&zs.txWritten)
}

// updateSocketStats clears socket counters and update ZSocket stats.
func (h *ZSocket) updateSocketStats() error {
	if h.version == TPacketVersion3 {
		var ssv3 C.struct_tpacket_stats_v3
		socklen := unsafe.Sizeof(ssv3)
		slt := uint(socklen)

		err := Getsockopt(h.socket, syscall.SOL_PACKET, syscall.PACKET_STATISTICS, unsafe.Pointer(&ssv3), &slt)
		if err != nil {
			return err
		}
		h.nPackets += uint(ssv3.tp_packets)
		h.nDrops += uint(ssv3.tp_drops)
	} else {
		var ss C.struct_tpacket_stats
		socklen := unsafe.Sizeof(ss)
		slt := uint(socklen)

		err := Getsockopt(h.socket, syscall.SOL_PACKET, syscall.PACKET_STATISTICS, unsafe.Pointer(&ss), &slt)
		if err != nil {
			return err
		}
		h.nPackets += uint(ss.tp_packets)
		h.nDrops += uint(ss.tp_drops)
	}
	return nil
}

// Listen to all specified packets in the RX ring-buffer
func (zs *ZSocket) Listen(fx CallbackFunc) error {
	if !zs.rxEnabled {
		return fmt.Errorf("the RX ring is disabled on this socket")
	}
	if !atomic.CompareAndSwapInt32(&zs.listening, 0, 1) {
		return fmt.Errorf("there is already a listener on this socket")
	}
	if zs.version == TPacketVersion3 {
		return zs.listenV3(fx)
	}
	return zs.listenV1(fx)
}

func (zs *ZSocket) listenV3(fx CallbackFunc) error {
	pfd := [1]PollFd{}
	pfd[0].fd = zs.socket
	pfd[0].events = C.POLLERR | C.POLLIN // | C.POLLRDNORM
	pfd[0].revents = 0
	//pfdP := uintptr(pfd.getPointer())
	bdIndex := 0
	pollTimeout := 50
	for {
		offs := bdIndex * zs.blockSize
		bd := zs.raw[offs : offs+zs.blockSize]
		pbd := (*blockDesc)(unsafe.Pointer(&bd[0]))
		if (pbd.h1.block_status & C.TP_STATUS_USER) == 0 {
			atomic.AddInt64(&zs.stats.Polls, 1)
			n, e1 := Poll(pfd[:], pollTimeout)
			if n == 0 {
				// should be timeout
				continue
			}
			if pfd[0].revents&C.POLLERR > 0 {
				// get error poll
				continue
			}
			if e1 != nil {
				return e1
			}
		} else {
			zs.walkBlock(bd, fx)
			bdIndex = (bdIndex + 1) % zs.numBlocks
		}
	}
}

func (zs *ZSocket) listenV1(fx CallbackFunc) error {
	pfd := [1]PollFd{}
	pfd[0].fd = zs.socket
	pfd[0].events = C.POLLERR | C.POLLIN // | C.POLLRDNORM
	pfd[0].revents = 0
	rxIndex := int32(0)
	rf := zs.rxFrames[rxIndex]
	pollTimeout := -1
	for {
		for ; rf.rxReady(); rf = zs.rxFrames[rxIndex] {
			//f := nettypes.Frame(rf.raw[rf.macStart():])
			f := rf.raw[rf.macStart():]
			fx(f, rf.tpLen(), rf.tpSnapLen())
			atomic.AddInt64(&zs.stats.Packets, 1)
			rf.rxSet()
			rxIndex = (rxIndex + 1) % zs.frameNum
		}
		atomic.AddInt64(&zs.stats.Polls, 1)
		_, e1 := Poll(pfd[:], pollTimeout)
		if e1 != nil {
			return e1
		}
	}
}

// WriteToBuffer writes a raw frame in bytes to the TX ring buffer.
// The length of the frame must be specified.
func (zs *ZSocket) WriteToBuffer(buf []byte, l uint16) (int32, error) {
	if l > zs.txFrameSize {
		return -1, fmt.Errorf("the length of the write exceeds the size of the TX frame")
	}
	if l <= 0 {
		return zs.CopyToBuffer(buf, uint16(len(buf)), copyFx)
	}
	return zs.CopyToBuffer(buf[:l], l, copyFx)
}

// CopyToBuffer is like WriteToBuffer, it writes a frame to the TX
// ring buffer. However, it can take a function argument, that will
// be passed the raw TX byes so that custom logic can be applied
// to copying the frame (for example, encrypting the frame).
func (zs *ZSocket) CopyToBuffer(buf []byte, l uint16, copyFx func(dst, src []byte, l int) uint16) (int32, error) {
	if !zs.txEnabled {
		return -1, fmt.Errorf("the TX ring is not enabled on this socket")
	}
	tx, txIndex, err := zs.getFreeTx()
	if err != nil {
		return -1, err
	}
	cL := copyFx(tx.txStart, buf, int(l))
	//C.setDstAddr((*C.char)(unsafe.Pointer(&tx.raw[0])), C.int(zs.ifIndex))
	tx.setTpLen(cL)
	tx.setTpSnapLen(cL)
	nPacketSent++
	written := atomic.AddInt32(&zs.txWritten, 1)
	if written == 1 {
		atomic.SwapInt32(&zs.txWrittenIndex, txIndex)
	}
	return txIndex, nil
}

// FlushFrames tells the kernel to flush all packets written
// to the TX ring buffer.n
func (zs *ZSocket) FlushFrames() (uint, error, []error) {
	if !zs.txEnabled {
		return 0, fmt.Errorf("the TX ring is not enabled on this socket, there is nothing to flush"), nil
	}
	var index int32
	for {
		index = atomic.LoadInt32(&zs.txWrittenIndex)
		if index == -1 {
			return 0, nil, nil
		}
		if atomic.CompareAndSwapInt32(&zs.txWrittenIndex, index, -1) {
			break
		}
	}
	written := atomic.SwapInt32(&zs.txWritten, 0)
	framesFlushed := uint(0)
	frameNum := int32(zs.frameNum)
	for t, w := index, written; w > 0; w-- {
		zs.txFrames[t].txSet()
		t = (t + 1) % frameNum
	}
	/*
		if _, _, e1 := syscall.Syscall6(syscall.SYS_SENDTO, uintptr(zs.socket), z, z, z, z, z); e1 != 0 {
			return framesFlushed, e1, nil
		}
	*/
	if _, e1 := Sendto(zs.socket, nil, 0, nil); e1 != nil {
		return framesFlushed, e1, nil
	}
	zs.updateSocketStats()
	var errs []error = nil
	for t, w := index, written; w > 0; w-- {
		tx := zs.txFrames[t]
		if zs.txLossDisabled && tx.txWrongFormat() {
			//log.Error("tx packet_loss")
			nPacketWrong++
			errs = append(errs, txIndexError(t))
		} else {
			framesFlushed++
		}
		tx.txSetMB()
		t = (t + 1) % frameNum
	}
	return framesFlushed, nil, errs
}

// Close socket
func (zs *ZSocket) Close() error {
	zs.updateSocketStats()
	if zs.rxEnabled {
		log.Infof("zsocket recv: %d/%d, drops: %d, Polls: %d", zs.stats.Packets,
			zs.nPackets, zs.nDrops, zs.stats.Polls)
	} else {
		log.Infof("zsocket sent: %d, sentError: %d", nPacketSent, nPacketWrong)
	}
	return syscall.Close(zs.socket)
}

func (zs *ZSocket) getFreeTx() (*ringFrame, int32, error) {
	if atomic.LoadInt32(&zs.txWritten) == zs.frameNum {
		return nil, -1, fmt.Errorf("the tx ring buffer is full")
	}
	var txIndex int32
	for txIndex = atomic.LoadInt32(&zs.txIndex); !atomic.CompareAndSwapInt32(&zs.txIndex, txIndex, (txIndex+1)%int32(zs.frameNum)); txIndex = atomic.LoadInt32(&zs.txIndex) {
	}
	tx := zs.txFrames[txIndex]
	for !tx.txReady() {
		pfd := [1]PollFd{}
		pfd[0].fd = zs.socket
		pfd[0].events = C.POLLERR | C.POLLOUT
		pfd[0].revents = 0
		timeout := -1
		/*
			_, _, e1 := syscall.Syscall(syscall.SYS_POLL, uintptr(pfd.getPointer()), uintptr(1), uintptr(unsafe.Pointer(&timeout)))
			if e1 != 0 {
				return nil, -1, e1
			}
		*/
		atomic.AddInt64(&zs.stats.Polls, 1)
		_, e1 := Poll(pfd[:], timeout)
		if e1 != nil {
			return nil, -1, e1
		}
	}
	for !tx.txMBReady() {
		runtime.Gosched()
	}
	return tx, txIndex, nil
}

type txIndexError int32

func (ie txIndexError) Error() string {
	return fmt.Sprintf("bad format in tx frame %d", ie)
}

type tpacketReq struct {
	blockSize, /* Minimal size of contiguous block */
	blockNum, /* Number of blocks */
	frameSize, /* Size of frame */
	frameNum uint
}

func (tr *tpacketReq) getPointer(isV3 bool) unsafe.Pointer {
	if isV3 {
		req := C.struct_tpacket_req3{C.uint(tr.blockSize), C.uint(tr.blockNum),
			C.uint(tr.frameSize), C.uint(tr.frameNum), C.uint(50), 0, 0}
		return unsafe.Pointer(&req)
	} else {
		req := C.struct_tpacket_req{C.uint(tr.blockSize),
			C.uint(tr.blockNum), C.uint(tr.frameSize), C.uint(tr.frameNum)}
		return unsafe.Pointer(&req)
	}
}

func (req *tpacketReq) size(isV3 bool) int {
	if isV3 {
		return int(unsafe.Sizeof(C.struct_tpacket_req3{}))
	}
	return int(unsafe.Sizeof(C.struct_tpacket_req{}))
}

type ringFrame struct {
	raw     []byte
	txStart []byte
	mb      uint32
}

func (rf *ringFrame) macStart() uint16 {
	tpHdr := (*C.struct_tpacket_hdr)(unsafe.Pointer(&rf.raw[0]))
	//macStart = int(tpHdr.tp_mac)
	return uint16(tpHdr.tp_mac)
}

func (rf *ringFrame) tpLen() uint16 {
	tpHdr := (*C.struct_tpacket_hdr)(unsafe.Pointer(&rf.raw[0]))
	return uint16(tpHdr.tp_len)
}

func (rf *ringFrame) setTpLen(v uint16) {
	tpHdr := (*C.struct_tpacket_hdr)(unsafe.Pointer(&rf.raw[0]))
	tpHdr.tp_len = C.uint(v)
}

func (rf *ringFrame) tpSnapLen() uint16 {
	tpHdr := (*C.struct_tpacket_hdr)(unsafe.Pointer(&rf.raw[0]))
	return uint16(tpHdr.tp_snaplen)
}

func (rf *ringFrame) setTpSnapLen(v uint16) {
	tpHdr := (*C.struct_tpacket_hdr)(unsafe.Pointer(&rf.raw[0]))
	tpHdr.tp_snaplen = C.uint(v)
}

func (rf *ringFrame) rxReady() bool {
	tpHdr := (*C.struct_tpacket_hdr)(unsafe.Pointer(&rf.raw[0]))
	return (tpHdr.tp_status&C.TP_STATUS_USER) == C.TP_STATUS_USER && atomic.CompareAndSwapUint32(&rf.mb, 0, 1)
}

func (rf *ringFrame) rxSet() {
	tpHdr := (*C.struct_tpacket_hdr)(unsafe.Pointer(&rf.raw[0]))
	tpHdr.tp_status = C.TP_STATUS_KERNEL
	// this acts as a memory barrier
	atomic.StoreUint32(&rf.mb, 0)
}

func (rf *ringFrame) txWrongFormat() bool {
	tpHdr := (*C.struct_tpacket_hdr)(unsafe.Pointer(&rf.raw[0]))
	return (tpHdr.tp_status & C.TP_STATUS_WRONG_FORMAT) == C.TP_STATUS_WRONG_FORMAT
}

func (rf *ringFrame) txReady() bool {
	tpHdr := (*C.struct_tpacket_hdr)(unsafe.Pointer(&rf.raw[0]))
	return (tpHdr.tp_status & (C.TP_STATUS_SEND_REQUEST | C.TP_STATUS_SENDING)) == 0
}

func (rf *ringFrame) txMBReady() bool {
	return atomic.CompareAndSwapUint32(&rf.mb, 0, 1)
}

func (rf *ringFrame) txSet() {
	tpHdr := (*C.struct_tpacket_hdr)(unsafe.Pointer(&rf.raw[0]))
	tpHdr.tp_status = C.TP_STATUS_SEND_REQUEST
}

func (rf *ringFrame) txSetMB() {
	atomic.StoreUint32(&rf.mb, 0)
}

func (rf *ringFrame) printRxStatus() {
	tpHdr := (*C.struct_tpacket_hdr)(unsafe.Pointer(&rf.raw[0]))
	s := tpHdr.tp_status
	fmt.Printf("RX STATUS :")
	if s == 0 {
		fmt.Printf(" Kernel")
	}
	if C.TP_STATUS_USER&s > 0 {
		fmt.Printf(" User")
	}
	if C.TP_STATUS_COPY&s > 0 {
		fmt.Printf(" Copy")
	}
	if C.TP_STATUS_LOSING&s > 0 {
		fmt.Printf(" Losing")
	}
	if C.TP_STATUS_CSUMNOTREADY&s > 0 {
		fmt.Printf(" CSUM-NotReady")
	}
	if C.TP_STATUS_VLAN_VALID&s > 0 {
		fmt.Printf(" VlanValid")
	}
	if C.TP_STATUS_BLK_TMO&s > 0 {
		fmt.Printf(" BlkTMO")
	}
	if C.TP_STATUS_VLAN_TPID_VALID&s > 0 {
		fmt.Printf(" VlanTPIDValid")
	}
	if C.TP_STATUS_CSUMNOTREADY&s > 0 {
		fmt.Printf(" CSUM-Valid")
	}
	rf.printRxTxStatus(uint64(s))
	fmt.Printf("\n")
}

func (rf *ringFrame) printTxStatus() {
	tpHdr := (*C.struct_tpacket_hdr)(unsafe.Pointer(&rf.raw[0]))
	s := tpHdr.tp_status
	fmt.Printf("TX STATUS :")
	if s == 0 {
		fmt.Printf(" Available")
	}
	if s&C.TP_STATUS_SEND_REQUEST > 0 {
		fmt.Printf(" SendRequest")
	}
	if s&C.TP_STATUS_SENDING > 0 {
		fmt.Printf(" Sending")
	}
	if s&C.TP_STATUS_WRONG_FORMAT > 0 {
		fmt.Printf(" WrongFormat")
	}
	rf.printRxTxStatus(uint64(s))
	fmt.Printf("\n")
}

func (rf *ringFrame) printRxTxStatus(s uint64) {
	if s&C.TP_STATUS_TS_SOFTWARE > 0 {
		fmt.Printf(" Software")
	}
	if s&_TP_STATUS_TS_RAW_HARDWARE > 0 {
		fmt.Printf(" Hardware")
	}
}

type ringFrameV3 struct {
	raw []byte
}

func (rf *ringFrameV3) macStart() uint16 {
	tpHdr := (*C.struct_tpacket3_hdr)(unsafe.Pointer(&rf.raw[0]))
	//macStart = int(tpHdr.tp_mac)
	return uint16(tpHdr.tp_mac)
}

func (rf *ringFrameV3) tpLen() uint16 {
	tpHdr := (*C.struct_tpacket3_hdr)(unsafe.Pointer(&rf.raw[0]))
	return uint16(tpHdr.tp_len)
}

func (rf *ringFrameV3) tpSnapLen() uint16 {
	tpHdr := (*C.struct_tpacket3_hdr)(unsafe.Pointer(&rf.raw[0]))
	return uint16(tpHdr.tp_snaplen)
}

func (rf *ringFrameV3) rxReady() bool {
	tpHdr := (*C.struct_tpacket3_hdr)(unsafe.Pointer(&rf.raw[0]))
	return (tpHdr.tp_status & C.TP_STATUS_USER) == C.TP_STATUS_USER
}

func (rf *ringFrameV3) rxSet() {
	tpHdr := (*C.struct_tpacket3_hdr)(unsafe.Pointer(&rf.raw[0]))
	tpHdr.tp_status = C.TP_STATUS_KERNEL
}

type blockDesc C.struct_block_desc

func (zs *ZSocket) walkBlock(bd []byte, fx CallbackFunc) {
	pbd := (*blockDesc)(unsafe.Pointer(&bd[0]))
	numPkts := int(pbd.h1.num_pkts)
	var ppd *C.struct_tpacket3_hdr
	offs := int(pbd.h1.offset_to_first_pkt)
	for i := 0; i < numPkts; i++ {
		ppd = (*C.struct_tpacket3_hdr)(unsafe.Pointer(&bd[offs]))
		//if ppd.tp_status & C.TP_STATUS_USER == 0 { break }
		tpLen := int(ppd.tp_len)
		if tpLen == 0 {
			log.Info("Got tpLen == 0")
			break
		}
		rf := ringFrameV3{bd[offs:]}
		macSt := int(rf.macStart())
		f := rf.raw[macSt:]
		fx(f, rf.tpLen(), rf.tpSnapLen())
		atomic.AddInt64(&zs.stats.Packets, 1)
		if ppd.tp_next_offset == 0 {
			// should spec process
			offs += int(tpAlign(int(ppd.tp_snaplen) + int(ppd.tp_mac)))
			break
		} else {
			offs += int(ppd.tp_next_offset)
		}
		//rf.rxSet()
		if offs >= zs.blockSize {
			// error
			log.Error("bad offs", offs)
			break
		}
	}
	pbd.h1.block_status = C.TP_STATUS_KERNEL
}
