// +build rawSocket

package MoldUDP

import (
	"net"
	"strings"
	"syscall"
	"time"

	"github.com/kjx98/golib/to"
)

// Client struct for MoldUDP client
//	Running		bool
//	LastRecv	int64	last time recv UDP
type Client struct {
	dst    [4]byte
	port   int
	fd     int
	reqSrv []syscall.SockaddrInet4

	ClientBase
}

func (c *Client) Close() error {
	if c.fd < 0 {
		return errClosed
	}
	err := syscall.Close(c.fd)
	c.fd = -1
	return err
}

func NewClient(udpAddr string, port int, opt *Option) (*Client, error) {
	var err error
	client := Client{fd: -1, port: port, ClientBase: ClientBase{seqNo: opt.NextSeq}}
	ifn := client.ClientBase.initClientBase(opt)
	if maddr := net.ParseIP(udpAddr); maddr != nil {
		if maddr.IsMulticast() {
			copy(client.dst[:], maddr.To4())
		} else {
			// set to 224.0.0.1
			client.dst[0] = 224
			client.dst[3] = 1
		}
	}
	client.fd, err = syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, 0)
	if err != nil {
		return nil, err
	}

	ReserveRecvBuf(client.fd)
	syscall.SetsockoptInt(client.fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
	// set Multicast
	err = syscall.Bind(client.fd, &syscall.SockaddrInet4{Port: port})
	if err != nil {
		syscall.Close(client.fd)
		log.Error("syscall.Bind", err)
		return nil, err
	}
	var mreq = &syscall.IPMreq{Multiaddr: client.dst}
	if ifn != nil {
		if addrs, err := ifn.Addrs(); err != nil {
			log.Info("Get if Addr", err)
		} else if len(addrs) > 0 {
			adr := strings.Split(addrs[0].String(), "/")[0]
			log.Infof("Try %s for MC group", adr)
			if ifAddr := net.ParseIP(adr); ifAddr != nil {
				copy(mreq.Interface[:], ifAddr.To4())
				log.Info("Use", adr, "for Multicast interface")
			}
		}
	}
	err = syscall.SetsockoptIPMreq(client.fd, syscall.IPPROTO_IP, syscall.IP_ADD_MEMBERSHIP, mreq)
	if err != nil {
		log.Info("add multi group", err)
	}
	for _, daddr := range opt.Srvs {
		ss := strings.Split(daddr, ":")
		if srvAddr := net.ParseIP(ss[0]); srvAddr == nil {
			continue
		} else {
			udpA := syscall.SockaddrInet4{}
			copy(udpA.Addr[:], srvAddr.To4())
			if len(ss) == 1 {
				udpA.Port = port
			} else {
				udpA.Port = to.Int(ss[1])
			}
			client.reqSrv = append(client.reqSrv, udpA)
		}
	}
	client.Running = true
	client.LastRecv = time.Now().Unix()
	go client.requestLoop()
	go client.doMsgLoop()
	return &client, nil
}

func (c *Client) requestLoop() {
	ticker := time.NewTicker(time.Millisecond * 200)
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
		n, remoteAddr, err := syscall.Recvfrom(c.fd, c.buff, 0)
		if err != nil {
			log.Error("ReadFromUDP from", remoteAddr, " ", err)
			continue
		}
		if err := c.ClientBase.gotBuff(n); err != nil {
			log.Error("Packet from", remoteAddr, " error:", err)
			continue
		} else {
			if len(c.reqSrv) == 0 {
				// try add source as Request server
				if adr, ok := remoteAddr.(*syscall.SockaddrInet4); ok {
					c.reqSrv = append(c.reqSrv, *adr)
				}
			}
		}
	}
}

func (c *Client) request(buff []byte) {
	if len(c.reqSrv) == 0 {
		return
	}
	c.nRequest++
	adr := &c.reqSrv[c.robinN]
	if c.nRequest < 5 {
		log.Info("Send reTrans seq:", c.seqNo, " req to", adr.Addr, adr.Port)
	}
	if err := syscall.Sendto(c.fd, buff, 0, adr); err != nil {
		log.Error("Req Sendto", err)
	}
	c.robinN++
	if c.robinN >= len(c.reqSrv) {
		c.robinN = 0
	}
}
