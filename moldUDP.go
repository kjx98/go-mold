package MoldUDP

import (
	//"bytes"
	"errors"
	"github.com/op/go-logging"
	"os"
)

var log = logging.MustGetLogger("go-mold")

const (
	headSize = 20
)

var (
	errTooShort   = errors.New("buffer too short")
	errUnmarshal  = errors.New("Unmarshal error")
	errMessageCnt = errors.New("MessageCount not zero without payload message")
	errClosed     = errors.New("socket already closed")
	errNoIP       = errors.New("No IP addr")
)

// a MoldUDP packet may contain multiple messages
//	message size less than 64k, uin16 for size
// Message	astract a Message Block
type Message struct {
	Data []byte
}

// UDP packet contains Header follow zero or more payload messages
//	Header
//		Session 10 ANUM	Indicates the session to which this packet belongs
//		SeqNo	sequence number of the first message in the packet
//		MessageCnt	the count of messages contained in this packet(request for
//					the number of messages requested for retransmission)
type Header struct {
	Session    string //	length fixed to 10 byte
	SeqNo      uint64
	MessageCnt uint16
}

func EncodeHead(buff []byte, head *Header) error {
	if len(buff) < headSize {
		return errTooShort
	}
	for i := 0; i < 10; i++ {
		buff[i] = ' '
	}
	// bytes.Repeat cost more than 100ns
	//copy(buff[:10], bytes.Repeat([]byte("  "), 5))
	copy(buff[:10], []byte(head.Session))
	coder.PutUint64(buff[10:18], head.SeqNo)
	coder.PutUint16(buff[18:20], head.MessageCnt)
	return nil
}

func DecodeHead(buff []byte, head *Header) error {
	if len(buff) < headSize {
		return errTooShort
	}
	// use bytes.TrimRight too slow, more than 150ns
	i := 0
	for i = 9; i > 0; i-- {
		if buff[i] != ' ' {
			i++
			break
		}
	}
	head.Session = string(buff[:i])
	head.SeqNo = coder.Uint64(buff[10:18])
	head.MessageCnt = coder.Uint16(buff[18:20])
	return nil
}

func Unmarshal(buff []byte, cnt int) (ret []Message, err error) {
	if cnt == 0 {
		return
	}
	// if cnt < 0 { return nil, errMessageCnt }
	ret = make([]Message, cnt)
	n := len(buff)
	i := 0
	off := 0
	for off < n {
		if off+2 > n {
			return nil, errUnmarshal
		}
		ll := int(coder.Uint16(buff[off : off+2]))
		off += 2
		if off+ll > n {
			return nil, errUnmarshal
		}
		/*
			mess := Message{}
			mess.Data = make([]byte, ll)
			copy(mess.Data, buff[off:off+ll])
		*/
		mess := Message{Data: buff[off : off+ll]}
		off += ll
		ret[i] = mess
		i++
		if i == cnt {
			break
		}
	}
	if off != n {
		return nil, errUnmarshal
	}
	return
}

func Marshal(buff []byte, msgs []Message) (msgCnt int, bufLen int) {
	n := len(buff)
	for _, msg := range msgs {
		mLen := len(msg.Data)
		if bufLen+2+mLen > n {
			break
		}
		coder.PutUint16(buff[bufLen:bufLen+2], uint16(mLen))
		bufLen += 2
		if mLen > 0 {
			copy(buff[bufLen:bufLen+mLen], msg.Data)
			bufLen += mLen
		}
		msgCnt++
	}
	return
}

//  `%{color}%{time:15:04:05.000} %{shortfunc} ▶ %{level:.4s} %{id:03x}%{color:reset} %{message}`
func init() {
	var format = logging.MustStringFormatter(
		`%{color}%{time:01-02 15:04:05.000}  ▶ %{level:.4s} %{color:reset} %{message}`,
	)

	logback := logging.NewLogBackend(os.Stderr, "", 0)
	logfmt := logging.NewBackendFormatter(logback, format)
	logging.SetBackend(logfmt)
}
