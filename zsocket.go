package MoldUDP

import (
	"fmt"
	//"github.com/kjx98/go-mold/nettypes"
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
#include <arpa/inet.h>  // htons()
#include <sys/mman.h>  // mmap(), munmap()
#include <poll.h>  // poll()
*/
import "C"

const (
	TPACKET_ALIGNMENT = 16

	MINIMUM_FRAME_SIZE = TPACKET_ALIGNMENT << 7
	MAXIMUM_FRAME_SIZE = TPACKET_ALIGNMENT << 11

	ENABLE_RX       = 1 << 0
	ENABLE_TX       = 1 << 1
	DISABLE_TX_LOSS = 1 << 2
)

const (
	_ETH_ALEN = C.ETH_ALEN //6
	ETH_ALL   = C.ETH_P_ALL
	ETH_IP    = C.ETH_P_IP
	ETH_IPX   = C.ETH_P_IPX
	ETH_IPV6  = C.ETH_P_IPV6

	_PACKET_VERSION = 0xa
	_PACKET_RX_RING = 0x5
	_PACKET_TX_RING = 0xd
	_PACKET_LOSS    = 0xe

	_TPACKET_V1 = 0
	// tp_status in <linux/if_packet.h>
	/* rx status */
	/* tx status */
	/* tx and rx status */
	_TP_STATUS_TS_RAW_HARDWARE = 1 << 31
	/* poll events */
)

var (
	_TP_MAC_START     int
	_TP_MAC_STOP      int
	_TP_LEN_START     int
	_TP_LEN_STOP      int
	_TP_SNAPLEN_START int
	_TP_SNAPLEN_STOP  int

	_TX_START int
)

// the top of every frame in the ring buffer looks like this:
//struct tpacket_hdr {
//         unsigned long   tp_status;
//         unsigned int    tp_len;
//         unsigned int    tp_snaplen;
//         unsigned short  tp_mac;
//         unsigned short  tp_net;
//         unsigned int    tp_sec;
//         unsigned int    tp_usec;
//};
func init() {
	tpHdr := C.struct_tpacket_hdr{}
	_TX_START = int(unsafe.Sizeof(tpHdr))
	r := _TX_START % TPACKET_ALIGNMENT
	if r > 0 {
		_TX_START += (TPACKET_ALIGNMENT - r)
	}
	log.Info("AF_PACKET Ring TX_START", _TX_START)
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

func copyFx(dst, src []byte, len uint16) uint16 {
	copy(dst, src)
	return len
}

// IZSocket is an interface for interacting with eth-iface like
// objects in the ZSocket code-base. This has basically
// simply enabled the FakeInterface code to work.
type IZSocket interface {
	MaxPackets() int32
	MaxPacketSize() uint16
	WrittenPackets() int32
	Listen(fx func([]byte, uint16, uint16))
	WriteToBuffer(buf []byte, l uint16) (int32, error)
	CopyToBuffer(buf []byte, l uint16, copyFx func(dst, src []byte, l uint16)) (int32, error)
	FlushFrames() (uint, error, []error)
	Close() error
}

// ZSocket opens a zero copy ring-buffer to the specified interface.
// Do not manually initialize ZSocket. Use `NewZSocket`.
type ZSocket struct {
	socket         int
	raw            []byte
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

	zs := new(ZSocket)
	eT := C.htons(C.ushort(ethType))
	// in Linux PF_PACKET is actually defined by AF_PACKET.
	sock, err := syscall.Socket(syscall.AF_PACKET, syscall.SOCK_RAW, int(eT))
	if err != nil {
		return nil, err
	}
	zs.socket = sock
	sll := syscall.SockaddrLinklayer{}
	sll.Protocol = uint16(eT)
	sll.Ifindex = ethIndex
	sll.Halen = _ETH_ALEN
	if err := syscall.Bind(sock, &sll); err != nil {
		return nil, err
	}

	zs.rxEnabled = options&ENABLE_RX == ENABLE_RX
	zs.txEnabled = options&ENABLE_TX == ENABLE_TX
	zs.txLossDisabled = options&DISABLE_TX_LOSS == DISABLE_TX_LOSS

	if err := syscall.SetsockoptInt(sock, syscall.SOL_PACKET, _PACKET_VERSION, _TPACKET_V1); err != nil {
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
	req.blockSize = req.frameSize * 8
	req.blockNum = maxTotalFrames / 8
	req.frameNum = (req.blockSize / req.frameSize) * req.blockNum
	reqP := req.getPointer()
	if zs.rxEnabled {
		_, _, e1 := syscall.Syscall6(uintptr(syscall.SYS_SETSOCKOPT), uintptr(sock), uintptr(syscall.SOL_PACKET), uintptr(_PACKET_RX_RING), uintptr(reqP), uintptr(req.size()), 0)
		if e1 != 0 {
			return nil, errnoErr(e1)
		}
	}
	if zs.txEnabled {
		_, _, e1 := syscall.Syscall6(uintptr(syscall.SYS_SETSOCKOPT), uintptr(sock), uintptr(syscall.SOL_PACKET), uintptr(_PACKET_TX_RING), uintptr(reqP), uintptr(req.size()), 0)
		if e1 != 0 {
			return nil, errnoErr(e1)
		}
		/*
			Can't get this to work for some reason
			if !zs.txLossDisabled {
				if err := syscall.SetsockoptInt(sock, syscall.SOL_PACKET, _PACKET_LOSS, 1); err != nil {
					return nil, err
				}
			}*/
	}

	size := req.blockSize * req.blockNum
	if zs.txEnabled && zs.rxEnabled {
		size *= 2
	}

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
		for i = 0; i < int(zs.frameNum); i++ {
			frLoc = i * int(zs.frameSize)
			rf := &ringFrame{}
			rf.raw = zs.raw[frLoc : frLoc+int(zs.frameSize)]
			zs.rxFrames = append(zs.rxFrames, rf)
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
			tx.txStart = tx.raw[_TX_START:]
			zs.txFrames = append(zs.txFrames, tx)
		}
	}
	return zs, nil
}

func calculateLargestFrame(ceil uint) uint {
	i := uint(MINIMUM_FRAME_SIZE)
	if i == ceil {
		return i
	}
	for i < ceil {
		i <<= 1
	}
	return (i >> 1)
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
	return zs.frameSize - uint16(_TX_START)
}

// Returns the amount of packets, written to the tx ring, that
// haven't been flushed.
func (zs *ZSocket) WrittenPackets() int32 {
	return atomic.LoadInt32(&zs.txWritten)
}

// Listen to all specified packets in the RX ring-buffer
func (zs *ZSocket) Listen(fx func([]byte, uint16, uint16)) error {
	if !zs.rxEnabled {
		return fmt.Errorf("the RX ring is disabled on this socket")
	}
	if !atomic.CompareAndSwapInt32(&zs.listening, 0, 1) {
		return fmt.Errorf("there is already a listener on this socket")
	}
	pfd := &pollfd{}
	pfd.fd = zs.socket
	pfd.events = C.POLLERR | C.POLLIN
	pfd.revents = 0
	pfdP := uintptr(pfd.getPointer())
	rxIndex := int32(0)
	rf := zs.rxFrames[rxIndex]
	pollTimeout := -1
	pTOPointer := uintptr(unsafe.Pointer(&pollTimeout))
	for {
		for ; rf.rxReady(); rf = zs.rxFrames[rxIndex] {
			//f := nettypes.Frame(rf.raw[rf.macStart():])
			f := rf.raw[rf.macStart():]
			fx(f, rf.tpLen(), rf.tpSnapLen())
			rf.rxSet()
			rxIndex = (rxIndex + 1) % zs.frameNum
		}
		_, _, e1 := syscall.Syscall(syscall.SYS_POLL, pfdP, uintptr(1), pTOPointer)
		if e1 != 0 {
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
	if l < 0 {
		return zs.CopyToBuffer(buf, uint16(len(buf)), copyFx)
	}
	return zs.CopyToBuffer(buf[:l], l, copyFx)
}

// CopyToBuffer is like WriteToBuffer, it writes a frame to the TX
// ring buffer. However, it can take a function argument, that will
// be passed the raw TX byes so that custom logic can be applied
// to copying the frame (for example, encrypting the frame).
func (zs *ZSocket) CopyToBuffer(buf []byte, l uint16, copyFx func(dst, src []byte, l uint16) uint16) (int32, error) {
	if !zs.txEnabled {
		return -1, fmt.Errorf("the TX ring is not enabled on this socket")
	}
	tx, txIndex, err := zs.getFreeTx()
	if err != nil {
		return -1, err
	}
	cL := copyFx(tx.txStart, buf, l)
	tx.setTpLen(cL)
	tx.setTpSnapLen(cL)
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
	z := uintptr(0)
	for t, w := index, written; w > 0; w-- {
		zs.txFrames[t].txSet()
		t = (t + 1) % frameNum
	}
	if _, _, e1 := syscall.Syscall6(syscall.SYS_SENDTO, uintptr(zs.socket), z, z, z, z, z); e1 != 0 {
		return framesFlushed, e1, nil
	}
	var errs []error = nil
	for t, w := index, written; w > 0; w-- {
		tx := zs.txFrames[t]
		if zs.txLossDisabled && tx.txWrongFormat() {
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
		pfd := &pollfd{}
		pfd.fd = zs.socket
		pfd.events = C.POLLERR | C.POLLOUT
		pfd.revents = 0
		timeout := -1
		_, _, e1 := syscall.Syscall(syscall.SYS_POLL, uintptr(pfd.getPointer()), uintptr(1), uintptr(unsafe.Pointer(&timeout)))
		if e1 != 0 {
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

func (tr *tpacketReq) getPointer() unsafe.Pointer {
	req := C.struct_tpacket_req{C.uint(tr.blockSize),
		C.uint(tr.blockNum), C.uint(tr.frameSize), C.uint(tr.frameNum)}
	return unsafe.Pointer(&req)
}

func (req *tpacketReq) size() int {
	return int(unsafe.Sizeof(C.struct_tpacket_req{}))
}

type pollfd struct {
	fd      int
	events  int16
	revents int16
}

func (pfd *pollfd) getPointer() unsafe.Pointer {
	cPfd := C.struct_pollfd{C.int(pfd.fd), C.short(pfd.events),
		C.short(pfd.revents)}
	return unsafe.Pointer(&cPfd)
}

func (req *pollfd) size() int {
	return int(unsafe.Sizeof(C.struct_pollfd{}))
}

type ringFrame struct {
	raw     []byte
	txStart []byte
	mb      uint32
}

func (rf *ringFrame) macStart() uint16 {
	tpHdr := (*C.struct_tpacket_hdr)(unsafe.Pointer(&rf.raw[0]))
	return uint16(tpHdr.tp_mac)
	//return nativeShort(rf.raw[_TP_MAC_START:_TP_MAC_STOP])
}

func (rf *ringFrame) tpLen() uint16 {
	tpHdr := (*C.struct_tpacket_hdr)(unsafe.Pointer(&rf.raw[0]))
	return uint16(tpHdr.tp_len)
	//return uint16(nativeInt(rf.raw[_TP_LEN_START:_TP_LEN_STOP]))
}

func (rf *ringFrame) setTpLen(v uint16) {
	tpHdr := (*C.struct_tpacket_hdr)(unsafe.Pointer(&rf.raw[0]))
	tpHdr.tp_len = C.uint(v)
	//nativePutInt(rf.raw[_TP_LEN_START:_TP_LEN_STOP], uint32(v))
}

func (rf *ringFrame) tpSnapLen() uint16 {
	tpHdr := (*C.struct_tpacket_hdr)(unsafe.Pointer(&rf.raw[0]))
	return uint16(tpHdr.tp_snaplen)
	//return uint16(nativeInt(rf.raw[_TP_SNAPLEN_START:_TP_SNAPLEN_STOP]))
}

func (rf *ringFrame) setTpSnapLen(v uint16) {
	tpHdr := (*C.struct_tpacket_hdr)(unsafe.Pointer(&rf.raw[0]))
	tpHdr.tp_snaplen = C.uint(v)
	//nativePutInt(rf.raw[_TP_SNAPLEN_START:_TP_SNAPLEN_STOP], uint32(v))
}

func (rf *ringFrame) rxReady() bool {
	tpHdr := (*C.struct_tpacket_hdr)(unsafe.Pointer(&rf.raw[0]))
	return tpHdr.tp_status&C.TP_STATUS_USER == C.TP_STATUS_USER
	//return nativeLong(rf.raw[0:HOST_LONG_SIZE])&_TP_STATUS_USER == _TP_STATUS_USER && atomic.CompareAndSwapUint32(&rf.mb, 0, 1)
}

func (rf *ringFrame) rxSet() {
	tpHdr := (*C.struct_tpacket_hdr)(unsafe.Pointer(&rf.raw[0]))
	tpHdr.tp_status = C.TP_STATUS_KERNEL
	//nativePutLong(rf.raw[0:HOST_LONG_SIZE], uint64(_TP_STATUS_KERNEL))
	// this acts as a memory barrier
	atomic.StoreUint32(&rf.mb, 0)
}

func (rf *ringFrame) txWrongFormat() bool {
	tpHdr := (*C.struct_tpacket_hdr)(unsafe.Pointer(&rf.raw[0]))
	return tpHdr.tp_status&C.TP_STATUS_WRONG_FORMAT == C.TP_STATUS_WRONG_FORMAT
	//return nativeLong(rf.raw[0:HOST_LONG_SIZE])&_TP_STATUS_WRONG_FORMAT == _TP_STATUS_WRONG_FORMAT
}

func (rf *ringFrame) txReady() bool {
	tpHdr := (*C.struct_tpacket_hdr)(unsafe.Pointer(&rf.raw[0]))
	return tpHdr.tp_status&(C.TP_STATUS_SEND_REQUEST|C.TP_STATUS_SENDING) == 0
	//return nativeLong(rf.raw[0:HOST_LONG_SIZE])&(_TP_STATUS_SEND_REQUEST|_TP_STATUS_SENDING) == 0
}

func (rf *ringFrame) txMBReady() bool {
	return atomic.CompareAndSwapUint32(&rf.mb, 0, 1)
}

func (rf *ringFrame) txSet() {
	tpHdr := (*C.struct_tpacket_hdr)(unsafe.Pointer(&rf.raw[0]))
	tpHdr.tp_status = C.TP_STATUS_SEND_REQUEST
	//nativePutLong(rf.raw[0:HOST_LONG_SIZE], uint64(_TP_STATUS_SEND_REQUEST))
}

func (rf *ringFrame) txSetMB() {
	atomic.StoreUint32(&rf.mb, 0)
}

func (rf *ringFrame) printRxStatus() {
	tpHdr := (*C.struct_tpacket_hdr)(unsafe.Pointer(&rf.raw[0]))
	s := tpHdr.tp_status
	//s := nativeLong(rf.raw[0:HOST_LONG_SIZE])
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
	//s := nativeLong(rf.raw[0:HOST_LONG_SIZE])
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
