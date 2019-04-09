// +build !nativeEndian

package MoldUDP

import (
	"bytes"
)

var head0 = Header{Session: "test0", SeqNo: 1, MessageCnt: 2}
var headBytes [20]byte
var msgBuf0 []byte
var msgBuf1 []byte
var msgBuf2 []byte
var msgBuf3 []byte
var msg0, msg1, msg2 Message

func init() {
	copy(headBytes[:], []byte(bytes.Repeat([]byte("  "), 5)))
	copy(headBytes[:], []byte(head0.Session))
	headBytes[17] = 1
	headBytes[19] = 2
	msgBuf0 = make([]byte, 256)
	msgBuf0[1] = 8
	msgBuf0[11] = 208
	msgBuf0[223] = 64
	msgBuf1 = msgBuf0[:10]
	msgBuf2 = msgBuf0[:220]
	msgBuf3 = msgBuf0[:222]
	msg0.Data = msgBuf0[2:10]
	msg1.Data = msgBuf0[12:220]
	msg2.Data = []byte{}
}
