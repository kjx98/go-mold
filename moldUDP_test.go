package MoldUDP

import (
	"reflect"
	"testing"
)

func TestEncodeHead(t *testing.T) {
	type args struct {
		buff []byte
		head *Header
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := EncodeHead(tt.args.buff, tt.args.head); (err != nil) != tt.wantErr {
				t.Errorf("EncodeHead() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDecodeHead(t *testing.T) {
	type args struct {
		buff []byte
	}
	tests := []struct {
		name    string
		args    args
		want    *Header
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodeHead(tt.args.buff)
			if (err != nil) != tt.wantErr {
				t.Errorf("DecodeHead() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DecodeHead() = %v, want %v", got, tt.want)
			}
		})
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
