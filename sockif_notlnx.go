// +build !linux

package MoldUDP

import "net"

func (c *sockIf) Enabled(opts int) bool {
	return false
}

func (c *sockIf) MSend(buffs []Packet) (int, error) {
	if c.bRead {
		return 0, errModeRW
	}
	return 0, errNotSupport
}

func (c *sockIf) MRecv() ([]Packet, *net.UDPAddr, error) {
	if !c.bRead {
		return nil, nil, errModeRW
	}
	return nil, nil, errNotSupport
}
