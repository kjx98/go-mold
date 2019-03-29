package MoldUDP

import (
	"bytes"
	"errors"
	"net"
	"runtime"
	"sort"
	"sync"
	"time"
)

const (
	reqInterval    = 20 * time.Millisecond
	maxMessages    = 1024
	cacheThreshold = 256 // start combine retrans
)

// ClientBase struct for MoldUDP client
//	Running		bool
//	LastRecv	int64	last time recv UDP
type ClientBase struct {
	Running          bool
	endSession       bool
	LastRecv         int64
	seqNo            uint64
	seqMax           uint64
	reqLast          time.Time
	nRecvs, nRequest int
	nError, nMissed  int
	nRepeats         int
	robinN           int
	session          string
	buff             []byte
	maxCache         int
	nMerges          int
	readLock         sync.RWMutex
	ch               chan msgBuf
	ready            []Message
	cache            []*messageCache
}

type msgBuf struct {
	seqNo   uint64
	msgCnt  uint16
	dataBuf []byte
}

type messageCache struct {
	seqNo   uint64
	seqNext uint64
	data    []Message
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
	c.cache = []*messageCache{}
	var ifn *net.Interface
	if opt.IfName != "" {
		if ifn, err = net.InterfaceByName(opt.IfName); err != nil {
			log.Errorf("Ifn(%s) error: %v\n", opt.IfName, err)
			ifn = nil
		}
	}
	c.buff = make([]byte, 2048)
	c.ch = make(chan msgBuf, 10000)
	return ifn
}

var (
	errDecodeHead    = errors.New("DecodeHead error")
	errInvMessageCnt = errors.New("Invalid MessageCnt")
	errSession       = errors.New("Session dismatch")
)

func (c *ClientBase) storeCache(buf []Message, seqNo uint64) uint64 {
	// should deep copy buf
	seqNext := seqNo + uint64(len(buf))
	var newCC = messageCache{seqNo: seqNo, seqNext: seqNext, data: buf}
	if len(c.cache) == 0 {
		c.cache = []*messageCache{&newCC}
	} else if cnt := len(c.cache); c.cache[cnt-1].seqNo < seqNo {
		// append or merge
		if sNext := c.cache[cnt-1].seqNext; seqNo > sNext {
			// append
			c.cache = append(c.cache, &newCC)
			return seqNo
		} else if seqNext < sNext {
			// merge, seqNo overlap, no Retrans need
			c.cache[cnt-1].data = append(c.cache[cnt-1].data,
				buf[seqNext-seqNo:]...)
			//log.Info("storeCache merge tail overlap")
		}
		c.nMerges++
		return 0
	} else {
		// insert or  merge
		off := sort.Search(cnt, func(i int) bool {
			return c.cache[i].seqNo >= seqNo
		})
		prev := off - 1
		cc := c.cache[off]
		if nn := cc.seqNo; seqNext >= nn {
			// merge next
			if nNext := cc.seqNext; nNext > seqNext {
				newCC.data = append(newCC.data, cc.data[seqNext-nn:]...)
				newCC.seqNext = nNext
				//log.Info("storeCache insert merge tail")
			}
			c.nMerges++
			off++
		}
		// seqNext may change
		seqNext = newCC.seqNext
		cct := c.cache[off:]
		if prev < 0 {
			c.cache = append([]*messageCache{&newCC}, cct...)
		} else if cc := c.cache[prev]; cc.seqNext >= seqNo {
			// merge head, no Retrans need
			if seqNext > cc.seqNext {
				cc.data = append(cc.data, buf[cc.seqNext-seqNo:]...)
			}
			c.cache = append(c.cache[:prev+1], cct...)
			c.nMerges++
			return 0
		} else {
			// insert
			ccs := append([]*messageCache{&newCC}, cct...)
			c.cache = append(c.cache[:prev+1], ccs...)
			return seqNo
		}
	}
	return seqNo
}

func (c *ClientBase) popCache(seqNo uint64) []Message {
	if len(c.cache) == 0 {
		return nil
	}
	var i int
	for i = 0; i < len(c.cache); i++ {
		if c.cache[i].seqNext > seqNo {
			break
		}
	}
	if i >= len(c.cache) {
		// all cache got
		c.cache = []*messageCache{}
		log.Info("popCache expire all cache")
		return nil
	}
	c.cache = c.cache[i:]
	if cc := c.cache[0]; cc.seqNo <= seqNo {
		ret := cc.data
		off := int(seqNo - cc.seqNo)
		c.cache = c.cache[1:]
		if off > 0 {
			log.Info("popCache merge", off, "overlap")
		}
		return ret[off:]
	}
	return nil
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
		newBuf = bytes.Repeat(c.buff[headSize:n], 1)
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
			return nil, err
		} else {
			res = ret
		}
	}
	if msgBB.msgCnt == 0xffff {
		log.Info("Got endSession packet")
		c.endSession = true
		if c.seqNo >= c.seqMax {
			log.Info("Got all messages seqNo:", c.seqNo, " stop running")
			c.Running = false
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
			if len(c.cache) > c.maxCache {
				c.maxCache = len(c.cache)
			}
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
			if reqBuf != nil {
				c.nMissed++
			}
			return reqBuf, nil
		}
		return nil, nil
	}
	seqNo = msgBB.seqNo
	if c.seqNo > seqNo {
		res = res[int(c.seqNo-seqNo):]
	}
	c.seqNo += uint64(len(res))
	// shall we check head cache to merge
	if bb := c.popCache(c.seqNo); bb != nil {
		res = append(res, bb...)
		c.seqNo += uint64(len(bb))
	}
	if c.endSession && c.seqNo >= c.seqMax {
		log.Info("Got all messages via retrans seqNo:", c.seqNo, " stop running")
		c.Running = false
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
	if len(c.cache) > 0 && len(c.cache) < cacheThreshold {
		// near window
		seqNo = c.cache[0].seqNo
	}
	cnt := seqNo - seqF
	if cnt > 60000 {
		cnt = 60000
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
func (c *ClientBase) Read() ([]Message, error) {
	for c.Running {
		c.readLock.Lock()
		res := c.ready
		c.ready = nil
		c.readLock.Unlock()
		if res != nil {
			return res, nil
		}
		runtime.Gosched()
	}
	return nil, nil
}

func (c *Client) SeqNo() int {
	return int(c.seqNo)
}

func (c *ClientBase) DumpStats() {
	log.Infof("Total Recv:%d seqNo: %d, error: %d, missed: %d, Request: %d/%d"+
		"\nmaxCache: %d, cache merge: %d", c.nRecvs, c.seqNo, c.nError,
		c.nMissed, c.nRequest, c.nRepeats, c.maxCache, c.nMerges)
}
