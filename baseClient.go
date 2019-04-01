package MoldUDP

import (
	"errors"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

const (
	reqInterval = 20 * time.Millisecond
	maxMessages = 1024
	maxCache    = 0x20000
	nakWindow   = 65400
)

// ClientBase struct for MoldUDP client
//	Running		bool
//	LastRecv	int64	last time recv UDP
type ClientBase struct {
	Running          bool
	endSession       bool
	bDone            bool
	LastRecv         int64
	seqNo            uint64
	seqMax           uint64
	reqLast          time.Time
	nRecvs, nRequest int
	nError, nMissed  int
	nRepeats         int
	lastSeq          uint64
	lastN            int32
	robinN           int
	session          string
	buff             []byte
	nMerges          int
	readLock         sync.RWMutex
	ch               chan msgBuf
	ready            []Message
	cache            []*Message
	//cacheS           []*Message
}

type msgBuf struct {
	seqNo   uint64
	msgCnt  uint16
	dataBuf []byte
}

// Option	options for Client connection
//	Srvs	request servers, host[:port]
//	IfName	if nor blank, if interface for Multicast
//	NextSeq	next sequence number for listen packet, 1 based
type Option struct {
	Srvs    []string
	IfName  string
	NextSeq uint64
}

func (c *ClientBase) initClientBase(opt *Option) *net.Interface {
	var err error
	// sequence number is 1 based
	if c.seqNo == 0 {
		c.seqNo++
	}
	var ifn *net.Interface
	if opt.IfName != "" {
		if ifn, err = net.InterfaceByName(opt.IfName); err != nil {
			log.Errorf("Ifn(%s) error: %v\n", opt.IfName, err)
			ifn = nil
		}
	}
	c.buff = make([]byte, 2048)
	c.ch = make(chan msgBuf, 10000)
	c.cache = make([]*Message, maxCache)
	//c.cacheS = make([]*Message, maxCache)
	return ifn
}

var (
	errDecodeHead    = errors.New("DecodeHead error")
	errInvMessageCnt = errors.New("Invalid MessageCnt")
	errSession       = errors.New("Session dismatch")
)

func (c *ClientBase) storeCache(buf []Message, seqNo uint64) uint64 {
	bLen := len(buf)
	off := int(seqNo - c.seqNo)
	bMerge := false
	for i := 0; i < bLen; i++ {
		if off+i >= maxCache {
			break
		}
		if c.cache[off+i] != nil {
			bMerge = true
		}
		c.cache[off+i] = &buf[i]
	}
	if bMerge {
		c.nMerges++
		return 0
	}
	return seqNo
}

func (c *ClientBase) popCache(seqNo uint64) []Message {
	if len(c.cache) == 0 {
		return nil
	}
	var i int
	off := int(seqNo - c.seqNo)
	res := []Message{}
	for i = off; i < len(c.cache); i++ {
		if c.cache[i] == nil {
			break
		}
	}
	if i == off {
		res = nil
	} else {
		for j := off; j < i; j++ {
			res = append(res, *c.cache[j])
		}
	}
	if i < len(c.cache) {
		copy(c.cache, c.cache[i:])
		//copy(c.cacheS, c.cache[i:])
		// swap cache
		//c.cache, c.cacheS = c.cacheS, c.cache
	}
	for j := len(c.cache) - i; j < len(c.cache); j++ {
		c.cache[j] = nil
	}
	return res
}

func (c *ClientBase) gotBuff(n int) error {
	c.nRecvs++
	var head Header
	if err := DecodeHead(c.buff[:n], &head); err != nil {
		c.nError++
		return errDecodeHead
	}
	nMsg := head.MessageCnt
	if nMsg != 0xffff && nMsg >= maxMessages {
		c.nError++
		return errInvMessageCnt
	}
	c.LastRecv = time.Now().Unix()
	if c.session == "" {
		c.session = head.Session
	} else if c.session != head.Session {
		c.nError++
		return errSession
	}

	var newBuf []byte
	if nMsg != 0xffff && nMsg != 0 {
		if n == headSize {
			return errMessageCnt
		}
		newBuf = make([]byte, n-headSize)
		copy(newBuf, c.buff[headSize:n])
	}
	msgBB := msgBuf{seqNo: head.SeqNo, msgCnt: nMsg, dataBuf: newBuf}
	c.ch <- msgBB
	return nil
}

func (c *ClientBase) doMsgBuf(msgBB *msgBuf) ([]byte, error) {
	var res []Message
	if len(msgBB.dataBuf) > 0 {
		if ret, err := Unmarshal(msgBB.dataBuf); err != nil {
			c.nError++
			//log.Error("Unmarshal msgBB", err)
			return nil, err
		} else {
			res = ret
		}
	}
	if msgBB.msgCnt == 0xffff {
		log.Info("Got endSession packet")
		c.endSession = true
		if msgBB.seqNo == c.seqNo && c.seqNo >= c.seqMax {
			log.Info("Got all messages seqNo:", c.seqNo, " to stop running")
			//c.Running = false
			c.bDone = true
			if c.ready == nil {
				c.Running = false
			}
		}
	}
	seqNo := msgBB.seqNo
	if msgCnt := msgBB.msgCnt; msgCnt != 0 && msgCnt != 0xffff {
		// should request for retransmit
		if len(res) != int(msgCnt) {
			c.nError++
			return nil, errMessageCnt
		}
		seqNext := seqNo + uint64(msgCnt)
		if seqNext < c.seqNo {
			// already got
			c.nRepeats++
			return nil, nil
		}
		// cache or not for MessageCnt not 0, 0xffff
		if seqF := c.seqNo; seqNo > seqF {
			seqNo = c.storeCache(res, seqNo)
			if seqNo <= seqF {
				return nil, nil
			}
			reqBuf := c.newReq(seqNo)
			c.nMissed++
			return reqBuf, nil
		}
	} else {
		// endSession
		// or heartbeat
		if c.seqNo < seqNo {
			reqBuf := c.newReq(seqNo)
			c.nMissed++
			return reqBuf, nil
		}
		return nil, nil
	}
	seqNo = msgBB.seqNo
	if c.seqNo > seqNo {
		res = res[int(c.seqNo-seqNo):]
	}
	seqNo = c.seqNo + uint64(len(res))
	//c.seqNo += uint64(len(res))
	// popCache used c.seqNo as base
	// shall we check head cache to merge
	if bb := c.popCache(seqNo); bb != nil {
		res = append(res, bb...)
		seqNo += uint64(len(bb))
	}
	atomic.StoreUint64(&c.lastSeq, c.seqNo)
	c.seqNo = seqNo
	atomic.StoreInt32(&c.lastN, int32(c.seqNo-c.lastSeq))
	if c.endSession && c.seqNo >= c.seqMax {
		log.Info("Got all messages via retrans seqNo:", c.seqNo, " to stop running")
		//c.Running = false
		c.bDone = true
	}
	c.readLock.Lock()
	c.ready = append(c.ready, res...)
	c.readLock.Unlock()
	return nil, nil
}

func (c *ClientBase) newReq(seqNo uint64) []byte {
	if c.seqNo >= seqNo {
		return nil
	}
	tt := time.Now()
	if seqNo > c.seqMax {
		c.seqMax = seqNo
	}
	if tt.Sub(c.reqLast) < reqInterval {
		return nil
	}
	c.reqLast = tt
	seqF := c.seqNo
	cnt := seqNo - seqF
	if cnt > nakWindow {
		cnt = nakWindow
	}
	head := Header{Session: c.session, SeqNo: seqF}
	head.MessageCnt = uint16(cnt)
	buff := [headSize]byte{}
	if err := EncodeHead(buff[:], &head); err != nil {
		log.Error("EncodeHead for Req reTrans", err)
		return nil
	}
	return buff[:headSize]
}

// Read			Get []Message in order
//	[]Message	messages received in order
//	return   	nil,nil   for end of session or finished
func (c *ClientBase) Read() ([]Message, uint64, error) {
	for c.Running {
		c.readLock.Lock()
		res := c.ready
		c.ready = nil
		seqNo := atomic.LoadUint64(&c.lastSeq)
		c.readLock.Unlock()
		if c.bDone && seqNo+uint64(len(res)) >= c.seqNo {
			log.Info("Read all seqNo:", c.seqNo, " really stop running")
			c.Running = false
			//c.bDone = true
		}
		if res != nil {
			return res, seqNo, nil
		}
		runtime.Gosched()
	}
	return nil, 0, nil
}

func (c *ClientBase) SeqNo() int {
	return int(c.seqNo)
}

func (c *ClientBase) LastSeq() (uint64, int) {
	seqNo := atomic.LoadUint64(&c.lastSeq)
	ret := atomic.LoadInt32(&c.lastN)
	return seqNo, int(ret)
}

func (c *ClientBase) DumpStats() {
	log.Infof("Total Recv:%d seqNo: %d, error: %d, missed: %d, Request: %d/%d"+
		"\nmaxCache: %d, cache merge: %d", c.nRecvs, c.seqNo, c.nError,
		c.nMissed, c.nRequest, c.nRepeats, maxCache, c.nMerges)
}
