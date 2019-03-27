package MoldUDP

import (
	"errors"
	"net"
	"time"
)

const (
	reqInterval = 5
	maxMessages = 1024
)

// ClientBase struct for MoldUDP client
//	Running		bool
//	LastRecv	int64	last time recv UDP
type ClientBase struct {
	Running          bool
	LastRecv         int64
	seqNo            uint64
	seqMax           uint64
	reqLast          int64
	nRecvs, nRequest int
	nError, nMissed  int
	robinN           int
	session          string
	buff             []byte
	head, tail       map[uint64]*messageCache
}

type messageCache struct {
	seqNo   uint64
	seqNext uint64
	data    []Message
}

// Option	options for Client connectionmap[uint64]*messageCache
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
	//c.head = map[uint64]*messageCache{}
	//c.tail = map[uint64]*messageCache{}
	var ifn *net.Interface
	if opt.IfName != "" {
		if ifn, err = net.InterfaceByName(opt.IfName); err != nil {
			log.Errorf("Ifn(%s) error: %v\n", opt.IfName, err)
			ifn = nil
		}
	}
	c.buff = make([]byte, 2048)
	return ifn
}

var (
	errDecodeHead    = errors.New("DecodeHead error")
	errInvMessageCnt = errors.New("Invalid MessageCnt")
	errSession       = errors.New("Session dismatch")
)

func (c *ClientBase) storeCache(buf []Message, seqNo uint64) {
	// should deep copy buf
	seqNext := seqNo + uint64(len(buf))
	var newCC = messageCache{seqNo: seqNo, seqNext: seqNext, data: buf}
	if cc, ok := c.head[seqNext]; ok {
		// found tail to merge
		delete(c.head, seqNext)
		delete(c.tail, cc.seqNext)
		newCC.data = append(newCC.data, cc.data...)
		seqNext = cc.seqNext
	}
	if cc, ok := c.tail[seqNo]; ok {
		// found head to merge
		delete(c.tail, seqNo)
		delete(c.head, cc.seqNo)
		newCC.data = append(cc.data, newCC.data...)
		seqNo = cc.seqNo
	}
	newCC.seqNo = seqNo
	newCC.seqNext = seqNext
	c.head[seqNo] = &newCC
	c.tail[seqNext] = &newCC
}

func (c *ClientBase) popCache(seqNo uint64) []Message {
	if cc, ok := c.head[seqNo]; ok {
		// found
		delete(c.tail, cc.seqNext)
		ret := cc.data
		delete(c.head, cc.seqNo)
		return ret
	}
	return nil
}

func (c *ClientBase) gotBuff(n int) ([]Message, []byte, error) {
	c.nRecvs++
	var head Header
	if err := DecodeHead(c.buff[:n], &head); err != nil {
		c.nError++
		return nil, nil, errDecodeHead
	}
	if nMsg := head.MessageCnt; nMsg != 0xffff && nMsg >= maxMessages {
		c.nError++
		return nil, nil, errInvMessageCnt
	}
	c.LastRecv = time.Now().Unix()
	if c.session == "" {
		c.session = head.Session
	} else if c.session != head.Session {
		c.nError++
		return nil, nil, errSession
	}
	var res []Message
	if n > headSize {
		if ret, err := Unmarshal(c.buff[headSize:n]); err != nil {
			c.nError++
			return nil, nil, err
		} else {
			res = ret
		}
	}
	if head.MessageCnt == 0xffff {
		log.Info("Got endSession packet")
		if c.seqNo >= c.seqMax {
			c.Running = false
		}
	}
	if head.SeqNo != c.seqNo {
		// should request for retransmit
		seqNo := head.SeqNo // + uint64(head.MessageCnt)
		reqBuf := c.newReq(seqNo)
		// cache or not for MessageCnt not 0, 0xffff
		if msgCnt := head.MessageCnt; msgCnt != 0 && msgCnt != 0xffff {
			c.storeCache(res, head.SeqNo)
		}
		c.nMissed++
		return nil, reqBuf, nil
	}
	switch head.MessageCnt {
	case 0xffff, 0:
		// endSession
		// or heartbeat
		return nil, nil, nil
	default:
		if headSize == n {
			c.nError++
			return nil, nil, errMessageCnt
		}
	}
	c.seqNo += uint64(len(res))
	if bb := c.popCache(c.seqNo); bb != nil {
		res = append(res, bb...)
		c.seqNo += uint64(len(bb))
	}
	// shall we check head cache to merge
	return res, nil, nil
}

func (c *ClientBase) newReq(seqNo uint64) []byte {
	if seqNo > c.seqMax {
		c.seqMax = seqNo
	}
	tt := time.Now().Unix()
	if c.reqLast+reqInterval > tt {
		return nil
	}
	c.reqLast = tt
	cnt := c.seqMax - c.seqNo
	if cnt > 60000 {
		cnt = 60000
	}
	head := Header{Session: c.session, SeqNo: c.seqNo}
	head.MessageCnt = uint16(cnt)
	buff := [headSize]byte{}
	if err := EncodeHead(buff[:], &head); err != nil {
		log.Error("EncodeHead for Req reTrans", err)
		return nil
	}
	return buff[:headSize]
}

func (c *Client) SeqNo() int {
	return int(c.seqNo)
}

func (c *Client) DumpStats() {
	log.Infof("Total Recv: %d seqNo: %d, errors: %d, missed: %d, Request: %d",
		c.nRecvs, c.seqNo, c.nError, c.nMissed, c.nRequest)
}
