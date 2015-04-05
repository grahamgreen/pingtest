package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"tatsushid/go-fastping"
	"time"

	"github.com/grahamgreen/goutils"
	"github.com/segmentio/go-loggly"
)

//just some text

type response struct {
	addr *net.IPAddr
	rtt  time.Duration
}

type RunningAvg struct {
	avg   time.Duration
	count int64
}

func (avg *RunningAvg) UpdateAvg(val time.Duration) {
	newAvg := ((avg.avg.Nanoseconds() * avg.count) + val.Nanoseconds()) / (avg.count + 1)
	avg.avg = time.Duration(newAvg)
	avg.count++
}

type Updateable interface {
	UpdateAvg(val time.Duration)
}

func UpdateAvg(v Updateable, val time.Duration) {
	v.UpdateAvg(val)
}

type host struct {
	name  string
	ip    net.IP
	fails uint64
	avg   RunningAvg
}

func main() {
	logToken := os.Getenv("LOGTOKEN")
	goutils.NotEmpty(logToken)
	logs := loggly.New(logToken)
	logs.FlushInterval = 30 * time.Second

	f, err := os.OpenFile("pingfail.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		fmt.Errorf("error opening file: %v", err)
	}
	defer f.Close()

	log.SetOutput(f)
	log.Println("Starting PingTest")

	hosts := make(map[string]*host)
	for _, val := range os.Args[1:] {
		split := strings.Split(val, ":")
		theHost := host{}
		if len(split) == 1 {
			theHost = host{name: split[0], ip: net.ParseIP(split[0])}
		} else {
			theHost = host{name: split[0], ip: net.ParseIP(split[1])}
		}
		hosts[theHost.ip.String()] = &theHost
	}

	results := make(map[string]*response)

	rttChan := make(chan *response, 6)
	failChan := make(chan *host, 6)

	go func() {
		for {
			select {
			case res := <-rttChan:
				if host, ok := hosts[res.addr.String()]; ok {
					UpdateAvg(&host.avg, res.rtt)
					msg := loggly.Message{
						"timestamp": time.Now().Format(time.RFC3339),
						"name":      host.name,
						"avg":       host.avg,
						"rtt":       res.rtt,
					}
					logs.Info("sucess", msg)
				}
			case host := <-failChan:
				host.fails++
				msg := loggly.Message{
					"timestamp": time.Now().Format(time.RFC3339),
					"name":      host.name,
					"avg":       host.avg,
					"failcount": host.fails,
				}
				logs.Error("fail", msg)
			}
		}
	}()

	pinger := fastping.NewPinger()

	for _, aHost := range hosts {
		addr, err := net.ResolveIPAddr("ip4:icmp", aHost.ip.String())
		goutils.Check(err)
		results[addr.String()] = nil
		pinger.AddIPAddr(addr)
	}

	onRecv, onIdle := make(chan *response), make(chan bool)
	pinger.OnRecv = func(addr *net.IPAddr, rtt time.Duration) {
		onRecv <- &response{addr: addr, rtt: rtt}
	}
	pinger.OnIdle = func() {
		onIdle <- true
	}
	pinger.MaxRTT = time.Second
	pinger.RunLoop()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	signal.Notify(c, syscall.SIGTERM)

loop:
	for {
		select {
		case <-c:
			fmt.Println("got interrupted")
			log.Println("Stopping PingTest")
			log.Println("RTT AVG")
			for _, host := range hosts {
				fmt.Printf("%s : %v\n", host.name, host.avg.avg)
				log.Printf("%s : %v\n", host.name, host.avg.avg)
			}
			log.Println("Fail Count")
			for _, host := range hosts {
				fmt.Printf("%s : %d\n", host.name, host.fails)
				log.Printf("%s : %d\n", host.name, host.fails)
			}
			break loop
		case res := <-onRecv:
			if _, ok := results[res.addr.String()]; ok {
				results[res.addr.String()] = res
			}
		case <-onIdle:
			for hostIP, r := range results {
				if r == nil {
					fmt.Printf("%v %s : unreachable\n", time.Now().Format(time.RFC3339), hosts[hostIP].name)
					log.Printf("%v, %s, %s\n", time.Now().Format(time.RFC3339), hosts[hostIP].name, hostIP)
					failChan <- hosts[hostIP]
				} else {
					fmt.Printf("time:%v,name:%v,totalAvg:%v,rtt:%v\n", time.Now().Format(time.RFC3339), hosts[hostIP].name, hosts[hostIP].avg.avg, r.rtt)
					rttChan <- r
				}
				results[hostIP] = nil
			}
		case <-pinger.Done():
			if err := pinger.Err(); err != nil {
				fmt.Println("ping failed:", err)
			}
			break loop
		}
	}
	signal.Stop(c)
	pinger.Stop()
}

//p := fastping.NewPinger()
//ra, err := net.ResolveIPAddr("ip4:icmp", os.Args[1])
//if err != nil {
//	fmt.Println(err)
//	os.Exit(1)
//}
//p.AddIPAddr(ra)
//p.OnRecv = func(addr *net.IPAddr, rtt time.Duration) {
//	fmt.Printf("IP Addr: %s receive, RTT: %v\n", addr.String(), rtt)
//}
//p.OnIdle = func() {
//	fmt.Println("finish")
//}
//err = p.Run()
//if err != nil {
//	fmt.Println(err)
//}
