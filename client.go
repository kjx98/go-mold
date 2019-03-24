// +build !norecover

package MoldUDP

import (
	"github.com/kjx98/golib/to"
	"io"
	"net"
	"strings"
	"syscall"
	"time"
)

const (
	reqInterval = 5
)

type Client struct {
	dst     [4]byte
	fd      int
	port    int
	Running bool
	seqNo   uint64
	reqLast int64 // time for last retrans Request
	robinN  int
	reqSrv  []syscall.SockaddrInet4
	session string
	buff    []byte
}

type Option struct {
	Srvs    []string
	IfName  string
	nextSeq uint64
}

func NewClient(udpAddr string, port int, opt *Option) (*Client, error) {
	var err error
	client := Client{seqNo: opt.nextSeq, port: port}
	if maddr := net.ParseIP(udpAddr); maddr != nil {
		copy(client.dst[:], maddr[12:16])
	}
	var ifn *net.Interface
	if opt.IfName != "" {
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
			if ifAddr := net.ParseIP(addrs[0].String()); ifAddr != nil {
				copy(mreq.Interface[:], ifAddr[12:16])
				log.Info("Use ", addrs[0], " for Multicast interface")
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
			copy(udpA.Addr[:], srvAddr[12:16])
			if len(ss) == 1 {
				udpA.Port = port
			} else {
				udpA.Port = to.Int(ss[1])
			}
			client.reqSrv = append(client.reqSrv, udpA)
		}
	}
	client.buff = make([]byte, 2048)
	return &client, nil
}

func (c *Client) Read() ([]Message, error) {
	for c.Running {
		n, remoteAddr, err := syscall.Recvfrom(c.fd, c.buff, 0)
		if err != nil {
			log.Error("ReadFromUDP from", remoteAddr, " ", err)
			continue
		}
		head, err := DecodeHead(c.buff[:n])
		if err != nil {
			log.Error("DecodeHead from", remoteAddr, " ", err)
			continue
		}
		if c.session == "" {
			c.session = head.Session
		} else if c.session != head.Session {
			log.Errorf("Session dismatch %s got %s", c.session, head.Session)
			continue
		}
		if head.SeqNo != c.seqNo {
			// should request for retransmit
			c.request(head.SeqNo)
		}
		switch head.MessageCnt {
		case 0xffff:
			return nil, io.EOF
		case 0:
			// got heartbeat
			continue
		}
		if head.SeqNo != c.seqNo {
			// cache or not
			continue
		}
		if headSize == n {
			return nil, errMessageCnt
		}
		ret, err := Unmarshal(c.buff[headSize:n])
		if err != nil {
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
	if err := syscall.Sendto(c.fd, buff, 0, c.reqSrv[c.robinN]); err != nil {
		log.Error("Req reTrans", err)
	}
	c.robinN++
	if c.robinN >= len(c.reqSrv) {
		c.robinN = 0
	}
}

//加入组播域
func UDPMulticast(socketMC int, multiaddr [4]byte) error {
	var mreq = &syscall.IPMreq{Multiaddr: multiaddr}
	err := syscall.SetsockoptIPMreq(socketMC, syscall.IPPROTO_IP, syscall.IP_ADD_MEMBERSHIP, mreq)
	return err
}

//退出组播域
func ExitMultiCast(socketMC int, multiaddr [4]byte) {
	var mreq = &syscall.IPMreq{Multiaddr: multiaddr}
	syscall.SetsockoptIPMreq(socketMC, syscall.IPPROTO_IP, syscall.IP_DROP_MEMBERSHIP, mreq)
}

//设置路由的TTL值
func SetTTL(fd, ttl int) error {
	return syscall.SetsockoptInt(fd, syscall.IPPROTO_IP, syscall.IP_MULTICAST_TTL, ttl)
}

//检查是否是有效的组播地址范围
func CheckMultiCast(addr [4]byte) bool {
	if addr[0] > 239 || addr[0] < 224 {
		return false
	}
	if addr[2] == 0 {
		return addr[3] <= 18
	}
	return true
}
