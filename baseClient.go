package MoldUDP

import (
	"errors"
	"net"
	"sort"
	"time"
)

const (
	reqInterval = 2
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
	cache            []*messageCache
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
	return ifn
}

var (
	errDecodeHead    = errors.New("DecodeHead error")
	errInvMessageCnt = errors.New("Invalid MessageCnt")
	errSession       = errors.New("Session dismatch")
)

func (c *ClientBase) storeCache(buf []Message, seqNo uint64) (uint64, uint64) {
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
			return sNext, seqNo
		} else {
			// merge, seqNo overlap, no Retrans need
			c.cache[cnt-1].data = append(c.cache[cnt-1].data,
				buf[seqNext-seqNo:]...)
		}
		return 0, 0
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
			}
			off++
		}
		if prev < 0 {
			c.cache = append([]*messageCache{&newCC}, c.cache[off:]...)
		} else if cc := c.cache[prev]; cc.seqNext >= seqNo {
			// merge head, no Retrans need
			if seqNext > cc.seqNext {
				cc.data = append(cc.data, buf[cc.seqNext-seqNo:]...)
			}
			return 0, 0
		} else {
			// insert
			ccs := append([]*messageCache{&newCC}, c.cache[off:]...)
			c.cache = append(c.cache[:prev], ccs...)
			return cc.seqNext, seqNo
		}
	}
	return c.seqNo, seqNo
}

func (c *ClientBase) popCache(seqNo uint64) []Message {
	if len(c.cache) > 0 && c.cache[0].seqNo == seqNo {
		ret := c.cache[0].data
		c.cache = c.cache[1:]
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
		// cache or not for MessageCnt not 0, 0xffff
		seqF := c.seqNo
		if msgCnt := head.MessageCnt; msgCnt != 0 && msgCnt != 0xffff {
			seqF, seqNo = c.storeCache(res, head.SeqNo)
		}
		reqBuf := c.newReq(seqF, seqNo)
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

func (c *ClientBase) newReq(seqF, seqNo uint64) []byte {
	tt := time.Now().Unix()
	if seqNo > c.seqMax {
		c.seqMax = seqNo
	} else {
		if c.reqLast+reqInterval > tt {
			//return nil
		}
	}
	c.reqLast = tt
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

func (c *Client) SeqNo() int {
	return int(c.seqNo)
}

func (c *Client) DumpStats() {
	log.Infof("Total Recv: %d seqNo: %d, errors: %d, missed: %d, Request: %d",
		c.nRecvs, c.seqNo, c.nError, c.nMissed, c.nRequest)
}
