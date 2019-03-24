// +build norecover

package MoldUDP

import (
	"github.com/kjx98/golib/to"
	"io"
	"net"
	"strings"
)

type Client struct {
	dst     net.UDPAddr
	conn    *net.UDPConn
	Running bool
	seqNo   uint64
	robinN  int
	reqSrv  []*net.UDPAddr
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
	client := Client{seqNo: opt.nextSeq}
	client.dst.IP = net.ParseIP(udpAddr)
	client.dst.Port = port
	var ifn *net.Interface
	if opt.IfName != "" {
		if ifn, err = net.InterfaceByName(opt.IfName); err != nil {
			log.Errorf("Ifn(%s) error: %v\n", opt.IfName, err)
			ifn = nil
		}
	}
	client.conn, err = net.ListenMulticastUDP("udp", ifn, &client.dst)
	if err != nil {
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
	return &client, nil
}

func (c *Client) Read() ([]Message, error) {
	for c.Running {
		n, remoteAddr, err := c.conn.ReadFromUDP(c.buff)
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
