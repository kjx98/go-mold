package MoldUDP

import (
	"bytes"
	"reflect"
	"testing"
	"time"
	"unsafe"
)

type crayUDP struct {
	Session  uint32
	trackId  uint16
	MsgCount uint16
	SeqNo    uint64
}

func TestCrayUDP(t *testing.T) {
	if nn := unsafe.Sizeof(crayUDP{}); nn != 16 {
		t.Errorf("crayUDP headSize diff: %d expect 16", nn)
	}
}

func TestEncodeHead(t *testing.T) {
	bb := make([]byte, 20)
	if err := EncodeHead(bb, &head0); err != nil {
		t.Error("EncodeHead()", err)
	}
	if !reflect.DeepEqual(bb, headBytes[:]) {
		t.Errorf("EncodeHead() expect %v, buff got %v", headBytes, bb)
	}
}

func TestDecodeHead(t *testing.T) {
	var hh Header
	if err := DecodeHead(headBytes[:], &hh); err != nil {
		t.Error("DecodeHead()", err)
	}
	if hh.Session != head0.Session || hh.SeqNo != head0.SeqNo || hh.MessageCnt != head0.MessageCnt {
		t.Errorf("DecodeHead() expect %v, buff got %v", head0, hh)
	}
}

func TestUnmarshal(t *testing.T) {
	tests := []struct {
		name    string
		arg     []byte
		cnt     int
		wantRet []Message
		wantErr bool
	}{
		// TODO: Add test cases.
		{"Unmarshal1", msgBuf0, 4, nil, true},
		{"Unmarshal2", msgBuf1, 1, []Message{msg0}, false},
		{"Unmarshal3", msgBuf2, 2, []Message{msg0, msg1}, false},
		{"Unmarshal4", msgBuf3, 3, []Message{msg0, msg1, msg2}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRet, err := Unmarshal(tt.arg, tt.cnt)
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
		{"testMarshal1", args{msgBuf1, []Message{msg0}}, 1, 10},
		{"testMarshal2", args{msgBuf2, []Message{msg0, msg1}}, 2, 220},
		{"testMarshal3", args{msgBuf3, []Message{msg0, msg1, msg2}}, 3, 222},
	}
	buff := make([]byte, 256)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMsgCnt, gotBufLen := Marshal(buff, tt.args.msgs)
			if gotMsgCnt != tt.wantMsgCnt {
				t.Errorf("Marshal() gotMsgCnt = %v, want %v", gotMsgCnt, tt.wantMsgCnt)
			}
			if gotBufLen != tt.wantBufLen {
				t.Errorf("Marshal() gotBufLen = %v, want %v", gotBufLen, tt.wantBufLen)
			}
			if !reflect.DeepEqual(buff[:gotBufLen], tt.args.buff) {
				t.Errorf("Marshal buffer = %v, want %v", buff[:gotBufLen], tt.args.buff)
			}
		})
	}
}

func BenchmarkEncodeHead(b *testing.B) {
	bb := make([]byte, 20)
	for i := 0; i < b.N; i++ {
		EncodeHead(bb, &head0)
	}
}

func BenchmarkDecodeHead(b *testing.B) {
	var hh Header
	for i := 0; i < b.N; i++ {
		DecodeHead(headBytes[:], &hh)
	}
}

func BenchmarkMarshal(b *testing.B) {
	var buff [256]byte
	for i := 0; i < b.N; i++ {
		Marshal(buff[:], []Message{msg0, msg1, msg2})
	}
}

func BenchmarkUnmarshal(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Unmarshal(msgBuf3, 3)
	}
}

func BenchmarkBytesRepeat(b *testing.B) {
	var hh [1500]byte
	for i := 0; i < b.N; i++ {
		_ = bytes.Repeat(hh[:], 1)
	}
}

func BenchmarkByteCopy(b *testing.B) {
	var hh [1500]byte
	for i := 0; i < b.N; i++ {
		dd := make([]byte, 1500)
		copy(dd, hh[:])
	}
}

func BenchmarkAppend(b *testing.B) {
	var hh [1500]byte
	for i := 0; i < b.N; i++ {
		_ = append([]byte{}, hh[:]...)
	}
}

func BenchmarkSysSleep(b *testing.B) {
	for i := 0; i < b.N; i++ {
		time.Sleep(time.Microsecond * 10)
	}
}

func BenchmarkSleep(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Sleep(time.Microsecond * 10)
	}
}
