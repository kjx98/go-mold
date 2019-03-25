package main

import (
	"flag"
	"fmt"
	"github.com/kjx98/go-mold"
	"github.com/op/go-logging"
	"os"
	"time"
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
	cc, err := MoldUDP.NewClient(maddr, port, &opt)
	if err != nil {
		log.Error("NewClient", err)
		os.Exit(1)
	}
	cc.Running = true
	waits := int64(10)
	tick := time.NewTicker(time.Second)
	go func() {
		for cc.Running {
			select {
			case <-tick.C:
				tt := time.Now().Unix()
				if cc.LastRecv == 0 {
					cc.LastRecv = tt
				} else if cc.LastRecv+waits < tt {
					cc.Running = false
					log.Errorf("No UDP recv for %d seconds", waits)
				}
			}
		}
		cc.DumpStats()
		time.Sleep(time.Second * 3)
		os.Exit(0)
	}()
	for cc.Running {
		mess, err := cc.Read()
		if err != nil {
			fmt.Println("Client Read", err)
			continue
		}
		if mess == nil {
			break
		}
		fmt.Printf("Got %d messages", len(mess))
	}
	cc.DumpStats()
	cc.Close()
}
