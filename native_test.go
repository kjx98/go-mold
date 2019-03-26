// +build nativeEndian

package MoldUDP

import (
	"bytes"
)

var head0 = Header{Session: "test0", SeqNo: 1, MessageCnt: 2}
var headBytes [20]byte
var msgBuf0 []byte
var msgBuf1 []byte
var msgBuf2 []byte
var msg0, msg1 Message

func init() {
	copy(headBytes[:], []byte(bytes.Repeat([]byte("  "), 5)))
	copy(headBytes[:], []byte(head0.Session))
	headBytes[10] = 1
	headBytes[18] = 2
	msgBuf0 = make([]byte, 256)
	msgBuf0[0] = 8
	msgBuf0[10] = 208
	msgBuf0[220] = 64
	msgBuf1 = msgBuf0[:10]
	msgBuf2 = msgBuf0[:220]
	msg0.Data = msgBuf0[2:10]
	msg1.Data = msgBuf0[12:220]
}
