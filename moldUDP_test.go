package MoldUDP

import (
	"bytes"
	"reflect"
	"testing"
)

var head0 = Header{Session: "test0", SeqNo: 1, MessageCnt: 2}
var headBytes [20]byte

func init() {
	copy(headBytes[:], []byte(bytes.Repeat([]byte("  "), 5)))
	copy(headBytes[:], []byte(head0.Session))
	headBytes[17] = 1
	headBytes[19] = 2
}

func TestEncodeHead(t *testing.T) {
	bb := make([]byte, 20)
	if err := EncodeHead(bb, &head0); err != nil {
		t.Error("EncodeHead()", err)
	}
	if !bytes.Equal(bb, headBytes[:]) {
		t.Errorf("EncodeHead() expect %v, buff got %v", headBytes, bb)
	}
}

func TestDecodeHead(t *testing.T) {
	hh, err := DecodeHead(headBytes[:])
	if err != nil {
		t.Error("DecodeHead()", err)
	}
	if hh.Session != head0.Session || hh.SeqNo != head0.SeqNo || hh.MessageCnt != head0.MessageCnt {
		t.Errorf("DecodeHead() expect %v, buff got %v", head0, hh)
	}
}

func TestUnmarshal(t *testing.T) {
	type args struct {
		buff []byte
	}
	tests := []struct {
		name    string
		args    args
		wantRet []Message
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRet, err := Unmarshal(tt.args.buff)
			if (err != nil) != tt.wantErr {
				t.Errorf("Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotRet, tt.wantRet) {
				t.Errorf("Unmarshal() = %v, want %v", gotRet, tt.wantRet)
			}
		})
	}
}

func TestMarshal(t *testing.T) {
	type args struct {
		buff []byte
		msgs []Message
	}
	tests := []struct {
		name       string
		args       args
		wantMsgCnt int
		wantBufLen int
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMsgCnt, gotBufLen := Marshal(tt.args.buff, tt.args.msgs)
			if gotMsgCnt != tt.wantMsgCnt {
				t.Errorf("Marshal() gotMsgCnt = %v, want %v", gotMsgCnt, tt.wantMsgCnt)
			}
			if gotBufLen != tt.wantBufLen {
				t.Errorf("Marshal() gotBufLen = %v, want %v", gotBufLen, tt.wantBufLen)
			}
		})
	}
}
