package MoldUDP

import (
	"net"
	"runtime"
	"sync/atomic"
	"time"
)

const (
	//maxUDPsize   = 1472
	//maxUDPsize   = 982	// for socket sendmmsg
	//maxUDPsize   = 896	// for zsocket 1k send frameSize
	maxUDPsize   = 512
	heartBeatInt = 2
	maxGoes      = 512
	maxWindow    = 120000
	PPms         = 100 // packets per ms
)

// Server struct for MoldUDP server
//	Running		bool
//	Session		session for all messages
type Server struct {
	Session       string
	dstIP         net.IP
	dstPort       int
	conn          McastConn
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
	nSleepMsend   int
	msgs          []Message
}

func (c *Server) EndSession(nWaits int) {
	c.endSession = true
	if nWaits > c.waits {
		c.waits = nWaits
	}
}

func (c *Server) FeedMessages(feeds []Message) {
	c.msgs = append(c.msgs, feeds...)
}

type hostControl struct {
	remote   net.UDPAddr
	bEnd     bool
	seqAcked uint64
	seqNext  uint64 // max nak sequence
	running  int32
}

// RequestLoop		go routine process request retrans
func (c *Server) RequestLoop() {
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
	rbuff := make([]byte, 1024)
	for c.Running {
		n, remoteAddr, err := c.connReq.ReadFromUDP(rbuff)
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
		if err := DecodeHead(rbuff[:n], &head); err != nil {
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
			} else {
				continue
			}
			if time.Now().Sub(lastLog) >= time.Second {
				log.Info(rAddr, "in process retrans for", hc.seqAcked, seqNext)
				lastLog = time.Now()
			}
		} else {
			hc = new(hostControl)
			hc.seqAcked = head.SeqNo
			hc.remote = *remoteAddr
			// always send response to dstPort
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

func (c *Server) SeqNo() int {
	return int(c.seqNo)
}

func (c *Server) DumpStats() {
	log.Infof("Total Sent: %d HeartBeat: %d seqNo: %d, sleep: %d/%d\n"+
		"Recv: %d, errors: %d, reSent: %d, maxGoes: %d",
		c.nSent, c.nHeartBB, c.seqNo, c.nSleep, c.nSleepMsend, c.nRecvs,
		c.nError, c.nResent, c.nMaxGoes)
}

func (c *Server) Close() error {
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

func NewServer(udpAddr string, port int, ifName string, bLoop bool, conn McastConn) (*Server, error) {
	server := Server{conn: conn, dstPort: port, seqNo: 1, PPms: PPms, waits: 5}
	// sequence number is 1 based
	server.dstIP = net.ParseIP(udpAddr)
	if !server.dstIP.IsMulticast() {
		log.Info(server.dstIP, " is not multicast IP")
		server.dstIP = net.IPv4(224, 0, 0, 1)
	}
	laddr := net.UDPAddr{IP: net.IPv4(0, 0, 0, 0), Port: port + 1}
	if conn, err := net.ListenUDP("udp4", &laddr); err != nil {
		log.Error("can't listen on request port")
	} else {
		server.connReq = conn
		log.Info("Request Server listen", server.connReq.LocalAddr())
	}
	var ifn *net.Interface
	if ifName != "" {
		if ifnn, err := net.InterfaceByName(ifName); err != nil {
			log.Errorf("Ifn(%s) error: %v\n", ifName, err)
			ifn = nil
		} else {
			ifn = ifnn
		}
	}
	if err := server.conn.OpenSend(server.dstIP, port, bLoop, ifn); err != nil {
		log.Error("Open Multicast", err)
		return nil, err
	}
	server.Running = true
	return &server, nil
}

// ServerLoop	go routine multicast UDP and heartbeat
func (c *Server) ServerLoop() {
	var buff [maxUDPsize]byte
	head := Header{Session: c.Session}
	lastSend := time.Now().Unix()
	mcastBuff := func(buff []byte, bLen int) {
		if err := EncodeHead(buff[:headSize], &head); err != nil {
			log.Error("EncodeHead for proccess mcast", err)
		} else {
			if _, err := c.conn.Send(buff[:headSize+bLen]); err != nil {
				log.Error("mcast send", err)
			}
			atomic.StoreInt64(&lastSend, time.Now().Unix())
			c.nSent++
		}
	}
	bMmsg := c.conn.Enabled(HasMmsg)
	if bMmsg {
		log.Info("Using Sendmmsg for multicast send")
	}
	var sbuffs, obuffs []Packet
	if bMmsg {
		sbuffs = make([]Packet, c.PPms)
		obuffs = make([]Packet, c.PPms)
		for i := 0; i < c.PPms; i++ {
			sbuffs[i] = make([]byte, maxUDPsize)
		}
	}
	hbLogs := 0
	for c.Running {
		st := time.Now()
		seqNo := int(c.seqNo)
		if seqNo > len(c.msgs) {
			// check for heartbeat sent
			if st.Unix()-atomic.LoadInt64(&lastSend) >= heartBeatInt {
				head.SeqNo = c.seqNo
				if c.endSession {
					// endSession must be last packets
					head.MessageCnt = 0xffff
					if hbLogs < 5 {
						log.Info("endSession sent EOS instead of HB")
						hbLogs++
					}
				} else {
					head.MessageCnt = 0
				}
				c.nHeartBB++
				mcastBuff(buff[:], 0)
			}
			if c.endTime != 0 {
				if c.endTime < time.Now().Unix() {
					c.Running = false
					break
				}
			} else if c.endSession {
				c.endTime = time.Now().Unix()
				c.endTime += int64(c.waits)
				// leave sent out endSession to heartbeat interval
				// send End of Session packet
				/*
					head.SeqNo = c.seqNo
					head.MessageCnt = 0xffff
					mcastBuff(buff[:], 0)
				*/
				log.Info("All messages sent, end Session")
			}
			runtime.Gosched()
			continue
		}

		sbuff := buff[:]
		nObuff := 0
		for i := 0; i < c.PPms; i++ {
			if seqNo > len(c.msgs) {
				break
			}
			if bMmsg {
				sbuff = []byte(sbuffs[i])
			}
			msgCnt, bLen := Marshal(sbuff[headSize:], c.msgs[seqNo-1:])
			if msgCnt == 0 {
				break
			}
			head.SeqNo = uint64(seqNo)
			head.MessageCnt = uint16(msgCnt)
			if bMmsg {
				if err := EncodeHead(sbuff[:headSize], &head); err != nil {
					log.Error("EncodeHead for proccess mcast", err)
				} else {
					obuffs[nObuff] = sbuff[:headSize+bLen]
					nObuff++
				}
			} else {
				mcastBuff(sbuff, bLen)
			}

			seqNo += msgCnt
			//time.Sleep(time.Microsecond * 10)
			//runtime.Gosched()
			// 500ns need tx qlen>=2000, 200ns need 5000
			//Sleep(time.Nanosecond * 250)
			//Sleep(time.Microsecond * 1)
			if !bMmsg {
				Sleep(time.Nanosecond * 250)
			}
		}
		if bMmsg && nObuff > 0 {
			// sendout MSend
			off := 0
			for off < nObuff {
				if n, err := c.conn.MSend(obuffs[off:nObuff]); err != nil {
					log.Error("MSend", err)
					break
				} else {
					//lastSend = time.Now()
					atomic.StoreInt64(&lastSend, time.Now().Unix())
					off += n
					c.nSent += n
					c.nSleepMsend++
					Sleep(time.Nanosecond * 5000)
				}
			}
		}
		c.seqNo = uint64(seqNo)
		dur := time.Now().Sub(st)
		// sleep to 1 ms
		if dur < time.Microsecond*990 {
			c.nSleep++
			if toSp := time.Millisecond - dur; toSp < time.Microsecond*100 {
				Sleep(toSp)
			} else {
				time.Sleep(toSp)
			}
		}
	}
}
