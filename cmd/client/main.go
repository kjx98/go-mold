package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kjx98/go-ats"
	MoldUDP "github.com/kjx98/go-mold"
	logging "github.com/op/go-logging"
)

var log = logging.MustGetLogger("mold-client")

var opt MoldUDP.Option

func main() {
	var maddr string
	var port int
	var waits int
	var firstP []MoldUDP.Message
	var firstTic, lastTic *ats.TickFX
	flag.StringVar(&maddr, "m", "224.0.0.1", "Multicast IPv4 to listen")
	flag.StringVar(&opt.IfName, "i", "", "Interface name for multicast")
	flag.IntVar(&port, "p", 5858, "UDP port to listen")
	flag.IntVar(&waits, "w", 0, "seconds wait for UDP packet, 0 unlimited")
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
	defer cc.Close()
	// catch  SIGTERM, SIGINT, SIGUP
	sigC := make(chan os.Signal, 10)
	signal.Notify(sigC)
	go func() {
		for s := range sigC {
			switch s {
			case os.Kill, os.Interrupt, syscall.SIGTERM:
				log.Info("退出", s)
				cc.Running = false
				ExitFunc()
			case syscall.SIGQUIT:
				log.Info("Quit", s)
				cc.Running = false
				ExitFunc()
			default:
				log.Info("Got signal", s)
			}
		}
	}()

	//cc.Running = true
	go func() {
		for cc.Running {
			mess, err := cc.Read()
			if err != nil {
				log.Error("Client Read", err)
				continue
			}
			if mess == nil {
				break
			}
			if firstP == nil {
				log.Infof("Got first %d messages", len(mess))
				firstP = mess
				firstTic = ats.Bytes2TickFX(firstP[0].Data)
			}
			if n := len(mess); n > 0 {
				lastTic = ats.Bytes2TickFX(mess[n-1].Data)
			}
		}
		// should we stop?
		cc.Running = false
	}()
	tick := time.NewTicker(time.Second)
	if cc.LastRecv == 0 {
		cc.LastRecv = time.Now().Unix()
	}
	nextDisp := int64(0)
	for cc.Running {
		var tt int64
		select {
		case <-tick.C:
			tt = time.Now().Unix()
			if waits > 0 && cc.LastRecv+int64(waits) < tt {
				cc.Running = false
				log.Errorf("No UDP recv for %d seconds", waits)
			}
		}
		if nextDisp == 0 {
			if len(firstP) > 0 {
				if firstTic != nil {
					log.Info("First message:", firstTic)
				}
				nextDisp = tt + 30
			}
		} else if nextDisp < tt {
			cc.DumpStats()
			nextDisp = tt + 30
		}
	}
	if lastTic != nil {
		log.Info("Last message:", lastTic)
	}
	cc.DumpStats()
	log.Info("exit client")
	os.Exit(0)
}

func ExitFunc() {
	//beeep.Alert("orSync Quit!", "try to exit", "")
	log.Warning("开始退出...")
	log.Warning("执行退出...")
	log.Warning("结束退出...")
	// wait 3 seconds
	time.Sleep(time.Second * 3)
	os.Exit(1)
}

/*
//  `%{color}%{time:15:04:05.000} %{shortfunc} ▶ %{level:.4s} %{id:03x}%{color:reset} %{message}`,
func init() {
	var format = logging.MustStringFormatter(
		`%{color}%{time:01-02 15:04:05}  ▶ %{level:.4s} %{color:reset} %{message}`,
	)

	logback := logging.NewLogBackend(os.Stderr, "", 0)
	logfmt := logging.NewBackendFormatter(logback, format)
	logging.SetBackend(logfmt)
}
*/
