// +build rawSocket

package MoldUDP

import (
	"net"
	"strings"
	"syscall"
	"time"

	"github.com/kjx98/golib/to"
)

const (
	reqInterval = 5
	maxMessages = 1024
)

// Client struct for MoldUDP client
//	Running		bool
//	LastRecv	int64	last time recv UDP
type Client struct {
	dst                      [4]byte
	fd                       int
	port                     int
	Running                  bool
	LastRecv                 int64
	seqNo                    uint64
	reqLast                  int64 // time for last retrans Request
	nRecvs, nError, nRequest int
	robinN                   int
	reqSrv                   []syscall.SockaddrInet4
	session                  string
	buff                     []byte
}

// Option	options for Client connection
//	Srvs	request servers, host[:port]
//	IfName	if nor blank, if interface for Multicast
//	NextSeq	next sequence number for listen packet
type Option struct {
	Srvs    []string
	IfName  string
	NextSeq uint64
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
	client := Client{fd: -1, seqNo: opt.NextSeq, port: port}
	// sequence number is 1 based
	if client.seqNo == 0 {
		client.seqNo++
	}
	if maddr := net.ParseIP(udpAddr); maddr != nil {
		if maddr.IsMulticast() {
			copy(client.dst[:], maddr.To4())
		} else {
			// set to 224.0.0.1
			client.dst[0] = 224
			client.dst[3] = 1
		}
	}
	var ifn *net.Interface
	if opt.IfName != "" {
		log.Info("Try ifname", opt.IfName, " for multicast")
		if ifn, err = net.InterfaceByName(opt.IfName); err != nil {
			log.Errorf("Ifn(%s) error: %v\n", opt.IfName, err)
			ifn = nil
		}
	}
	client.fd, err = syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, 0)
	if err != nil {
		return nil, err
	}
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
				log.Info("Use ", adr, " for Multicast interface")
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
	client.buff = make([]byte, 2048)
	client.Running = true
	return &client, nil
}

// Read			Get []Message in order
//	[]Message	messages received in order
//	return   	nil,nil   for end of session or finished
func (c *Client) Read() ([]Message, error) {
	for c.Running {
		n, remoteAddr, err := syscall.Recvfrom(c.fd, c.buff, 0)
		if err != nil {
			log.Error("ReadFromUDP from", remoteAddr, " ", err)
			continue
		}
		c.nRecvs++
		c.LastRecv = time.Now().Unix()
		var head Header
		if err := DecodeHead(c.buff[:n], &head); err != nil {
			log.Error("DecodeHead from", remoteAddr, " ", err)
			c.nError++
			continue
		}
		if nMsg := head.MessageCnt; nMsg != 0xffff && nMsg >= maxMessages {
			log.Errorf("Invalid MessageCnt(%d) from %s", nMsg, remoteAddr)
			c.nError++
			continue
		}
		if len(c.reqSrv) == 0 {
			// try add source as Request server
			if adr, ok := remoteAddr.(*syscall.SockaddrInet4); ok {
				c.reqSrv = append(c.reqSrv, *adr)
			}
		}
		if c.session == "" {
			c.session = head.Session
		} else if c.session != head.Session {
			log.Errorf("Session dismatch %s got %s", c.session, head.Session)
			c.nError++
			continue
		}
		if head.SeqNo != c.seqNo {
			// should request for retransmit
			c.request(head.SeqNo)
		}
		switch head.MessageCnt {
		case 0xffff:
			return nil, nil
		case 0:
			// got heartbeat
			continue
		}
		if head.SeqNo != c.seqNo {
			// cache or not
			continue
		}
		if headSize == n {
			c.nError++
			return nil, errMessageCnt
		}
		ret, err := Unmarshal(c.buff[headSize:n])
		if err != nil {
			c.nError++
			return nil, err
		}
		c.seqNo += uint64(len(ret))
		return ret, nil
	}
	return nil, nil
}

func (c *Client) request(seqNo uint64) {
	if len(c.reqSrv) == 0 {
		return
	}
	tt := time.Now().Unix()
	if c.reqLast+reqInterval > tt {
		return
	}
	c.reqLast = tt
	cnt := seqNo - c.seqNo
	if cnt > 60000 {
		cnt = 60000
	}
	head := Header{Session: c.session, SeqNo: c.seqNo}
	head.MessageCnt = uint16(cnt)
	buff := make([]byte, headSize)
	if err := EncodeHead(buff, &head); err != nil {
		log.Error("EncodeHead for Req reTrans", err)
		return
	}
	c.nRequest++
	if err := syscall.Sendto(c.fd, buff, 0, &c.reqSrv[c.robinN]); err != nil {
		log.Error("Req reTrans", err)
	}
	c.robinN++
	if c.robinN >= len(c.reqSrv) {
		c.robinN = 0
	}
}

func (c *Client) SeqNo() int {
	return int(c.seqNo)
}

func (c *Client) DumpStats() {
	log.Infof("Total Recv: %d, errors: %d, sent Request: %d",
		c.nRecvs, c.nError, c.nRequest)
}
