// +build rawSocket

package MoldUDP

import (
	"net"
	"runtime"
	"syscall"
	"time"
)

// Server struct for MoldUDP server
//	Running		bool
//	Session		session for all messages
type Server struct {
	dst SockaddrInet4
	fd  int
	ServerBase
}

func (c *Server) Close() error {
	if c.fd < 0 {
		return errClosed
	}
	err := Close(c.fd)
	c.fd = -1
	c.ServerBase.Close()
	return err
}

func NewServer(udpAddr string, port int, ifName string, bLoop bool) (*Server, error) {
	var err error
	server := Server{}
	server.ServerBase.init(udpAddr, port)
	server.dst.Port = server.dstPort
	copy(server.dst.Addr[:], server.dstIP.To4())
	var ifn *net.Interface
	if ifName != "" {
		if ifn, err = net.InterfaceByName(ifName); err != nil {
			log.Errorf("Ifn(%s) error: %v\n", ifName, err)
			ifn = nil
		}
	}
	laddr := SockaddrInet4{Port: port}
	server.fd, err = Socket(syscall.AF_INET, syscall.SOCK_DGRAM, 0)
	if err != nil {
		return nil, err
	}
	fd := server.fd
	ReserveSendBuf(fd)
	log.Info("Server listen", LocalAddr(fd))
	SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
	err = Bind(fd, &laddr)
	if err != nil {
		Close(fd)
		log.Error("syscall.Bind", err)
		return nil, err
	}
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
			if err := Sendto(c.fd, buff[:headSize+bLen], 0, &c.dst); err != nil {
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
