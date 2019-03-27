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
	return ifn
}

var (
	errDecodeHead    = errors.New("DecodeHead error")
	errInvMessageCnt = errors.New("Invalid MessageCnt")
	errSession       = errors.New("Session dismatch")
)

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
	if head.SeqNo != c.seqNo {
		// should request for retransmit
		seqNo := head.SeqNo + uint64(head.MessageCnt)
		reqBuf := c.newReq(seqNo)
		// cache or not for MessageCnt not 0, 0xffff
		c.nMissed++
		return nil, reqBuf, nil
	}
	switch head.MessageCnt {
	case 0xffff:
		// should check SeqNo
		log.Info("Got endSession packet")
		if c.seqNo >= c.seqMax {
			c.Running = false
		}
		fallthrough
	case 0:
		// got heartbeat
		return nil, nil, nil
	}
	if headSize == n {
		c.nError++
		return nil, nil, errMessageCnt
	}
	ret, err := Unmarshal(c.buff[headSize:n])
	if err != nil {
		c.nError++
		return nil, nil, err
	}
	c.seqNo += uint64(len(ret))
	return ret, nil, nil
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
