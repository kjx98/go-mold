// +build !nativeEndian

package MoldUDP

import (
	"encoding/binary"
)

var coder = binary.BigEndian
