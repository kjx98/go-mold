package main

import (
	"flag"
	"fmt"
	"github.com/kjx98/go-mold"
	"github.com/op/go-logging"
	"os"
)

var log = logging.MustGetLogger("mold-client")

var opt MoldUDP.Option

func main() {
	var maddr string
	var port int
	flag.StringVar(&maddr, "m", "224.0.0.1", "Multicast IPv4 to listen")
	flag.StringVar(&opt.IfName, "i", "", "Interface name for multicast")
	flag.IntVar(&port, "p", 5858, "UDP port to listen")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: client [options]\n")
		flag.PrintDefaults()
		os.Exit(2)
	}
	flag.Parse()
	if cc, err := MoldUDP.NewClient(maddr, port, &opt); err != nil {
		log.Error("NewClient", err)
	} else {
		cc.Close()
	}
}
