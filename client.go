package MoldUDP

import (
	"errors"
	"net"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kjx98/golib/to"
)

const (
	reqInterval = 100 * time.Millisecond
	maxMessages = 1024
	nakWindow   = 65400
)

// Client struct for MoldUDP client
//	Running		bool
//	LastRecv	int64	last time recv UDP
type Client struct {
	dstIP            net.IP // Multicast dst IP
	dstPort          int    // Multicast dst Port
	connReq          *net.UDPConn
	conn             McastConn
	reqSrv           []*net.UDPAddr
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
	cache            msgCache
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

func (c *Client) Close() error {
	if c.conn == nil {
		return errClosed
	}
	err := c.conn.Close()
	c.conn = nil
	if c.connReq != nil {
		c.connReq.Close()
		c.connReq = nil
	}
	return err
}

var (
	errDecodeHead    = errors.New("DecodeHead error")
	errInvMessageCnt = errors.New("Invalid MessageCnt")
	errSession       = errors.New("Session dismatch")
)

func (c *Client) storeCache(buf []Message, seqNo uint64) uint64 {
	bLen := len(buf)
	bMerge := false
	ret := seqNo
	for i := 0; i < bLen; i++ {
		if c.cache.Upset(seqNo, &buf[i]) {
			bMerge = true
		}
		seqNo++
	}
	if bMerge {
		c.nMerges++
		return 0
	}
	return ret
}

func (c *Client) popCache(seqNo uint64) []Message {
	return c.cache.Merge(seqNo)
}

func (c *Client) gotBuff(n int) error {
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
	} else {
		// newBuf is nil for endSession or Heartbeat
	}
	msgBB := msgBuf{seqNo: head.SeqNo, msgCnt: nMsg, dataBuf: newBuf}
	c.ch <- msgBB
	return nil
}

func (c *Client) doMsgBuf(msgBB *msgBuf) ([]byte, error) {
	var res []Message
	if len(msgBB.dataBuf) > 0 {
		if ret, err := Unmarshal(msgBB.dataBuf, int(msgBB.msgCnt)); err != nil {
			c.nError++
			//log.Error("Unmarshal msgBB", err)
			return nil, err
		} else {
			res = ret
		}
	}
	if msgBB.msgCnt == 0xffff {
		if !c.endSession {
			log.Info("Got endSession packet")
			c.endSession = true
		}
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
		if seqF := c.seqNo; seqNext < seqF {
			// already got
			c.nRepeats++
			return nil, nil
		} else if seqNo > seqF {
			// cache or not for MessageCnt not 0, 0xffff
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

func (c *Client) newReq(seqNo uint64) []byte {
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
func (c *Client) Read() ([]Message, uint64, error) {
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

func (c *Client) SeqNo() int {
	return int(c.seqNo)
}

func (c *Client) LastSeq() (uint64, int) {
	seqNo := atomic.LoadUint64(&c.lastSeq)
	ret := atomic.LoadInt32(&c.lastN)
	return seqNo, int(ret)
}

func (c *Client) DumpStats() {
	log.Infof("Total Recv:%d seqNo: %d/%d,error: %d,missed: %d, Request: %d/%d"+
		"\nmaxCache: %d, cache merge: %d", c.nRecvs, c.seqNo, c.seqMax, c.nError,
		c.nMissed, c.nRequest, c.nRepeats, c.cache.maxPageNo, c.nMerges)
}

func NewClient(udpAddr string, port int, opt *Option, conn McastConn) (*Client, error) {
	var err error
	client := Client{conn: conn, seqNo: opt.NextSeq}
	if client.seqNo == 0 {
		client.seqNo++
	}
	client.dstIP = net.ParseIP(udpAddr)
	client.dstPort = port
	if !client.dstIP.IsMulticast() {
		log.Info(client.dstIP, "is not multicast IP")
		client.dstIP = net.IPv4(224, 0, 0, 1)
	}
	var ifn *net.Interface
	if opt.IfName != "" {
		if ifn, err = net.InterfaceByName(opt.IfName); err != nil {
			log.Errorf("Ifn(%s) error: %v\n", opt.IfName, err)
			ifn = nil
		}
	}
	if err := client.conn.Open(client.dstIP, port, ifn); err != nil {
		log.Error("Open Multicast", err)
		return nil, err
	}
	for _, daddr := range opt.Srvs {
		ss := strings.Split(daddr, ":")
		udpA := net.UDPAddr{IP: net.ParseIP(ss[0])}
		if len(ss) == 1 {
			udpA.Port = port
		} else {
			udpA.Port = to.Int(ss[1])
		}
		client.reqSrv = append(client.reqSrv, &udpA)
	}
	client.buff = make([]byte, 2048)
	client.ch = make(chan msgBuf, 10000)
	client.cache.Init()
	client.Running = true
	client.LastRecv = time.Now().Unix()
	go client.requestLoop()
	go client.doMsgLoop()
	return &client, nil
}

func (c *Client) requestLoop() {
	ticker := time.NewTicker(time.Millisecond * 100)
	nextReqT := int64(0)
	for c.Running {
		select {
		case <-ticker.C:
			if c.seqNo < c.seqMax {
				req := c.newReq(c.seqMax)
				if req != nil {
					// need send Request
					c.request(req)
				}
			}
			tt := time.Now().Unix()
			if nextReqT != 0 {
				if c.LastRecv+1 >= tt {
					nextReqT = 0
				} else {
					nextReqT = tt + 1
					req := c.newReq(c.seqNo + 200)
					if req != nil {
						c.request(req)
					}
				}
			} else if c.LastRecv+1 < tt {
				nextReqT = tt + 1
			}
		case msgBB, ok := <-c.ch:
			if ok {
				if req, err := c.doMsgBuf(&msgBB); err != nil {
					log.Error("doMsgBuf", err)
				} else {
					if req != nil {
						// need send Request
						c.request(req)
					}

				}
			}
		}
	}
}

func (c *Client) doMsgLoop() {
	for c.Running {
		n, remoteAddr, err := c.conn.Recv(c.buff)
		if err != nil {
			log.Error("ReadFromUDP from", remoteAddr, " ", err)
			continue
		}
		if err := c.gotBuff(n); err != nil {
			log.Error("Packet from", remoteAddr, " error:", err)
			continue
		} else {
			if len(c.reqSrv) == 0 {
				// request port diff from sending source port
				remoteAddr.Port = c.dstPort + 1
				c.reqSrv = append(c.reqSrv, remoteAddr)
			}
		}
	}
}

func (c *Client) request(buff []byte) {
	if len(c.reqSrv) == 0 {
		return
	}
	c.nRequest++
	if c.nRequest < 5 {
		log.Info("Send reTrans seq:", c.seqNo, " req to", c.reqSrv[c.robinN])
	}
	if c.connReq == nil {
		if conn, err := net.DialUDP("udp", nil, c.reqSrv[c.robinN]); err != nil {
			log.Error("DialUDP requestSrv", err)
			return
		} else {
			c.connReq = conn
		}
	}
	if _, err := c.connReq.Write(buff[:]); err != nil {
		log.Error("Req WriteToUDP", err)
	}
	c.robinN++
	if c.robinN >= len(c.reqSrv) {
		c.robinN = 0
	}
}
