package MoldUDP

import (
	"reflect"
	"testing"
)

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
		wantRet []Message
		wantErr bool
	}{
		// TODO: Add test cases.
		{"Unmarshal1", msgBuf0, nil, true},
		{"Unmarshal2", msgBuf1, []Message{msg0}, false},
		{"Unmarshal3", msgBuf2, []Message{msg0, msg1}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRet, err := Unmarshal(tt.arg)
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
