package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tatsushid/go-fastping"
)

type response struct {
	addr *net.IPAddr
	rtt  time.Duration
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage:\n %s [options] ip\n\nOptions:\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	var ips = make([]string, len(flag.Args()))
	copy(ips, flag.Args())
	results := make(map[string]*response)

	pinger := fastping.NewPinger()

	for _, ipaddr := range ips {
		addr, err := net.ResolveIPAddr("ip4:icmp", ipaddr)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
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
			break loop
		case res := <-onRecv:
			if _, ok := results[res.addr.String()]; ok {
				results[res.addr.String()] = res
			}
		case <-onIdle:
			for host, r := range results {
				if r == nil {
					fmt.Printf("%s : unreachable %v\n", host, time.Now())
				} else {
					fmt.Printf("%s : %v %v\n", host, r.rtt, time.Now())
				}
				results[host] = nil
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
