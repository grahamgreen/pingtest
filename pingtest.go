package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/tatsushid/go-fastping"
)

func main() {
	flag.Usage = func() {
		fmt.Printf(os.Stderr, "Usage:\n %s [options] ip\n\nOptions:\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	ip := flag.Arg(0)
	if len(ip) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	pinger := fastping.NewPinger()
	addr, err := net.ResolveIPAddr("ip4:icmp", ip)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	pinger.AddIPAddr(addr)
	pinger.OnRecv = func(addr *net.IPAddr, rtt time.Duration) {
		fmt.Printf("IP Addr: %s receive, RTT: %v\n", addr.String(), rtt)
	}
	pinger.OnIdle = func() {
		fmt.Println("finished")
	}
	err = pinger.Run()
	if err != nil {
		fmt.Println(err)
	}
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
