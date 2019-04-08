// +build !rawSocket

package MoldUDP

import (
	"net"
	"runtime"
	"time"
)

// Server struct for MoldUDP server
//	Running		bool
//	Session		session for all messages
type Server struct {
	conn *net.UDPConn
	dst  net.UDPAddr
	ServerBase
}

func (c *Server) Close() error {
	if c.conn == nil {
		return errClosed
	}
	err := c.conn.Close()
	c.conn = nil
	c.ServerBase.Close()
	return err
}

func NewServer(udpAddr string, port int, ifName string, bLoop bool) (*Server, error) {
	var err error
	server := Server{}
	server.ServerBase.init(udpAddr, port)
	server.dst.IP = server.dstIP
	server.dst.Port = server.dstPort
	var fd int = -1
	laddr := net.UDPAddr{IP: net.IPv4(0, 0, 0, 0), Port: port}
	if bLoop {
		// let system allc port
		laddr.Port = 0
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
	server.conn, err = net.ListenUDP("udp", &laddr)
	if err != nil {
		return nil, err
	}
	if ff, err := server.conn.File(); err == nil {
		fd = int(ff.Fd())
	} else {
		log.Error("Get UDPConn fd", err)
	}
	log.Info("Server listen", server.conn.LocalAddr())
	ReserveSendBuf(fd)
	/*
		if err := JoinMulticast(fd, server.dstIP, ifn); err != nil {
			log.Info("add multicast group", err)
		}
	*/
	if err := SetMulticastInterface(fd, ifn); err != nil {
		log.Info("set multicast interface", err)
	}
	if bLoop {
		if err := SetMulticastLoop(fd, true); err != nil {
			log.Info("set multicast loopback", err)
		}
	}
	return &server, nil
}

// ServerLoop	go routine multicast UDP and heartbeat
func (c *Server) ServerLoop() {
	var buff [maxUDPsize]byte
	head := Header{Session: c.Session}
	lastSend := time.Now()
	hbInterval := time.Second * heartBeatInt
	mcastBuff := func(bLen int) {
		if err := EncodeHead(buff[:headSize], &head); err != nil {
			log.Error("EncodeHead for proccess mcast", err)
		} else {
			if _, err := c.conn.WriteToUDP(buff[:headSize+bLen], &c.dst); err != nil {
				log.Error("mcast send", err)
			}
			lastSend = time.Now()
			c.nSent++
		}
	}

	for c.Running {
		st := time.Now()
		seqNo := int(c.seqNo)
		if seqNo > len(c.msgs) {
			// check for heartbeat sent
			if st.Sub(lastSend) >= hbInterval {
				head.SeqNo = c.seqNo
				if c.endSession {
					// endSession must be last packet
					head.MessageCnt = 0xffff
				} else {
					head.MessageCnt = 0
					c.nHeartBB++
					mcastBuff(0)
				}
			}
			if c.endTime != 0 {
				if c.endTime < time.Now().Unix() {
					c.Running = false
					break
				}
			} else if c.endSession {
				c.endTime = time.Now().Unix()
				c.endTime += int64(c.waits)
				// send End of Session packet
				head.SeqNo = c.seqNo
				head.MessageCnt = 0xffff
				mcastBuff(0)
				log.Info("All messages sent, end Session")
			}
			runtime.Gosched()
			continue
		}

		for i := 0; i < c.PPms; i++ {
			if seqNo > len(c.msgs) {
				break
			}
			msgCnt, bLen := Marshal(buff[headSize:], c.msgs[seqNo-1:])
			if msgCnt == 0 {
				break
			}
			head.SeqNo = uint64(seqNo)
			head.MessageCnt = uint16(msgCnt)
			mcastBuff(bLen)
			seqNo += msgCnt
			//time.Sleep(time.Microsecond * 10)
			//runtime.Gosched()
			// 500ns need tx qlen>=2000, 200ns need 5000
			//Sleep(time.Nanosecond * 250)
			//Sleep(time.Microsecond * 1)
			Sleep(time.Nanosecond * 250)
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
