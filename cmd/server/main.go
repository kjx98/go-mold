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
	"github.com/kjx98/golib/julian"
	logging "github.com/op/go-logging"
)

var log = logging.MustGetLogger("mold-server")

func main() {
	var maddr, ifName string
	var port int
	var ppms int
	var bLoop bool
	flag.StringVar(&maddr, "m", "224.0.0.1", "Multicast IPv4 to listen")
	flag.StringVar(&ifName, "i", "", "Interface name for multicast")
	flag.BoolVar(&bLoop, "l", false, "multicast loopback")
	flag.IntVar(&port, "p", 5858, "UDP port to listen")
	flag.IntVar(&ppms, "s", 500, "PPms packets per ms")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: server [options]\n")
		flag.PrintDefaults()
		os.Exit(2)
	}
	flag.Parse()
	cc, err := MoldUDP.NewServer(maddr, port, ifName, bLoop)
	if err != nil {
		log.Error("NewServer", err)
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

	cc.Session = time.Now().Format("20060102")
	cc.PPms = ppms
	// fill Messages
	msgs := []MoldUDP.Message{}
	enDate := julian.FromUint32(20180101)
	if eur, err := ats.LoadTickFX("EURUSD", 0, enDate, 0); err == nil {
		log.Infof("Load %d EURUSD ticks", len(eur))
		for i := 0; i < len(eur); i++ {
			msg := MoldUDP.Message{}
			msg.Data = ats.TickFX2Bytes(eur[i : i+1])
			msgs = append(msgs, msg)
		}
	}
	cc.FeedMessages(msgs)
	st := time.Now()
	go cc.RequestLoop()
	go cc.ServerLoop()
	for cc.Running {
		time.Sleep(time.Second)
		if cc.SeqNo() >= len(msgs) {
			cc.EndSession()
		}
	}
	du := time.Now().Sub(st)
	cc.DumpStats()
	log.Infof("exit server, running %.3f seconds", du.Seconds())
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
