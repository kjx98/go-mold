package MoldUDP

import (
	"net"
	"sync/atomic"
	"time"
)

const (
	maxUDPsize   = 1472
	heartBeatInt = 2
	maxGoes      = 512
	maxWindow    = 120000
	PPms         = 100 // packets per ms
)

// Server struct for MoldUDP server
//	Running		bool
//	Session		session for all messages
type ServerBase struct {
	Session       string
	dstIP         net.IP
	dstPort       int
	connReq       *net.UDPConn
	PPms          int
	Running       bool
	endSession    bool
	seqNo         uint64
	endTime       int64
	waits         int // wait for 5 seconds end of session
	nRecvs, nSent int
	nError        int
	nResent       int32
	nGoes         int32
	nMaxGoes      int
	nHeartBB      int
	nSleep        int
	msgs          []Message
	buff          []byte
}

func (c *ServerBase) Close() {
	if c.connReq != nil {
		c.connReq.Close()
		c.connReq = nil
	}
}

func (c *ServerBase) EndSession(nWaits int) {
	c.endSession = true
	if nWaits > c.waits {
		c.waits = nWaits
	}
}

func (c *ServerBase) FeedMessages(feeds []Message) {
	c.msgs = append(c.msgs, feeds...)
}

func (server *ServerBase) init(udpAddr string, port int) {
	server.seqNo = 1
	server.waits = 5
	server.PPms = PPms
	// sequence number is 1 based
	server.dstIP = net.ParseIP(udpAddr)
	server.dstPort = port
	if !server.dstIP.IsMulticast() {
		log.Info(server.dstIP, " is not multicast IP")
		server.dstIP = net.IPv4(224, 0, 0, 1)
	}
	laddr := net.UDPAddr{IP: net.IPv4(0, 0, 0, 0), Port: port + 1}
	if conn, err := net.ListenUDP("udp", &laddr); err != nil {
		log.Error("can't listen on request port")
	} else {
		server.connReq = conn
		log.Info("Request Server listen", server.connReq.LocalAddr())
	}
	server.buff = make([]byte, 2048)
	server.Running = true
}

type hostControl struct {
	remote   net.UDPAddr
	bEnd     bool
	seqAcked uint64
	seqNext  uint64 // max nak sequence
	running  int32
}

// RequestLoop		go routine process request retrans
func (c *ServerBase) RequestLoop() {
	hostMap := map[string]*hostControl{}
	var nResends int

	doReq := func(hc *hostControl) {
		//seqNo uint64, cnt uint16, remoteA net.UDPAddr
		// only retrans one UDP packet
		// proce reTrans
		var buff [maxUDPsize]byte
		seqNo := hc.seqAcked

		defer atomic.AddInt32(&c.nGoes, -1)
		nResends++
		if (nResends & 127) == 0 {
			log.Infof("Resend packets to %s Seq: %d -- %d", hc.remote.IP,
				seqNo, hc.seqNext)
		}
		sHead := Header{Session: c.Session, SeqNo: seqNo}
		for seqNo < atomic.LoadUint64(&hc.seqNext) {
			lastS := int(atomic.LoadUint64(&hc.seqNext))
			msgCnt, bLen := Marshal(buff[headSize:], c.msgs[seqNo-1:lastS-1])
			if msgCnt == 0 && c.endSession {
				// end of Session
				break
			} else {
				sHead.MessageCnt = uint16(msgCnt)
			}
			if err := EncodeHead(buff[:headSize], &sHead); err != nil {
				log.Error("EncodeHead for proccess reTrans", err)
				continue
			}
			atomic.AddInt32(&c.nResent, 1)
			if _, err := c.connReq.WriteToUDP(buff[:headSize+bLen],
				&hc.remote); err != nil {
				log.Error("Res WriteToUDP", hc.remote, err)
				break
			}
			seqNo += uint64(msgCnt)
			if seqNo >= uint64(len(c.msgs)) {
				break
			}
			sHead.SeqNo = seqNo
			// system Sleep delay 50us about, so about 100us sleep
			// or changed to Sleep 50us
			time.Sleep(time.Microsecond * 50)
		}
		atomic.StoreInt32(&hc.running, 0)
		// ony recover lost last packet and endSession
		// using endSession Hearbeat packet to indicate endSession
		if c.endSession && int(c.seqNo) >= len(c.msgs) && !hc.bEnd {
			hc.bEnd = true
			sHead.SeqNo = seqNo
			// send endSession as well
			sHead.MessageCnt = 0xffff
			//log.Info("Retrans endSession, seqNo:", seqNo)
			if err := EncodeHead(buff[:headSize], &sHead); err == nil {
				_, err = c.connReq.WriteToUDP(buff[:headSize], &hc.remote)
			} else {
				log.Error("EncodeHead for proccess reTrans endSession", err)
			}
		}
	}
	if c.connReq == nil {
		return
	}
	lastLog := time.Now()
	for c.Running {
		n, remoteAddr, err := c.connReq.ReadFromUDP(c.buff)
		if err != nil {
			log.Error("ReadFromUDP from", remoteAddr, " ", err)
			continue
		}
		c.nRecvs++
		//c.LastRecv = time.Now().Unix()
		if n != headSize {
			c.nError++
			continue
		}
		var head Header
		if err := DecodeHead(c.buff[:n], &head); err != nil {
			log.Error("DecodeHead from", remoteAddr, " ", err)
			c.nError++
			continue
		}
		if head.SeqNo >= c.seqNo {
			log.Errorf("Invalid seq %d, server seqNo: %d", head.SeqNo, c.seqNo)
			c.nError++
			continue
		}
		if nMsg := head.MessageCnt; nMsg == 0xffff || nMsg == 0 {
			log.Errorf("Seems msg from server MessageCnt(%d) from %s", nMsg, remoteAddr)
			c.nError++
			continue
		}
		rAddr := remoteAddr.IP.String()
		var hc *hostControl
		seqNext := head.SeqNo + uint64(head.MessageCnt)
		if hh, ok := hostMap[rAddr]; ok {
			hc = hh
			if atomic.LoadInt32(&hc.running) == 0 {
				hc.seqAcked = head.SeqNo
			}
			if time.Now().Sub(lastLog) >= time.Second {
				log.Info(rAddr, "in process retrans for", hc.seqAcked, seqNext)
				lastLog = time.Now()
			}
		} else {
			hc = new(hostControl)
			hc.seqAcked = head.SeqNo
			hc.remote = *remoteAddr
			hc.remote.Port = c.dstPort
			hostMap[rAddr] = hc
		}
		if head.SeqNo > hc.seqAcked {
			hc.seqAcked = head.SeqNo
		}
		if seqNext > hc.seqAcked+maxWindow {
			log.Info("%d exceed maxWindow seqNo: %d", seqNext, hc.seqAcked)
			continue
		}
		if atomic.LoadUint64(&hc.seqNext) < seqNext {
			atomic.StoreUint64(&hc.seqNext, seqNext)
		}

		if atomic.LoadInt32(&hc.running) == 0 {
			if atomic.LoadInt32(&c.nGoes) > maxGoes {
				continue
			}
			nGoes := int(atomic.AddInt32(&c.nGoes, 1))
			if nGoes > c.nMaxGoes {
				c.nMaxGoes = nGoes
			}
			atomic.StoreInt32(&hc.running, 1)
			go doReq(hc)
		}
	}
}

func (c *ServerBase) SeqNo() int {
	return int(c.seqNo)
}

func (c *ServerBase) DumpStats() {
	log.Infof("Total Sent: %d HeartBeat: %d seqNo: %d, sleep: %d\n"+
		"Recv: %d, errors: %d, reSent: %d, maxGoes: %d",
		c.nSent, c.nHeartBB, c.seqNo, c.nSleep, c.nRecvs, c.nError,
		c.nResent, c.nMaxGoes)
}
