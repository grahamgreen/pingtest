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

	"github.com/codegangsta/cli"
	"github.com/grahamgreen/goutils"
	"github.com/ziutek/rrd"
)

const (
	version   = "0.1.0"
	rrd_dir   = "/home/ggreen/"
	step      = 1
	heartbeat = 2 * step
)

type response struct {
	addr *net.IPAddr
	rtt  time.Duration
}

type host struct {
	name    string
	ip      net.IP
	fails   uint64
	rrdFile string
}

func BuildRRD(dbfile string, overwrite bool) error {
	now := time.Now()
	start := now.Add(-5 * time.Second)

	c := rrd.NewCreator(dbfile, start, step)
	c.DS("rtt", "GAUGE", heartbeat, "U", "U")
	c.DS("fail", "COUNTER", heartbeat, "U", "U")
	c.RRA("AVERAGE", 0.5, 1, 300)    //5min w/ sec res
	c.RRA("AVERAGE", 0.5, 10, 90)    //10min w/ sec res
	c.RRA("AVERAGE", 0.5, 60, 60)    //1h w/ min res
	c.RRA("AVERAGE", 0.5, 60, 360)   //6h
	c.RRA("AVERAGE", 0.5, 60, 720)   //12h
	c.RRA("AVERAGE", 0.5, 60, 1440)  //24h
	c.RRA("AVERAGE", 0.5, 3600, 168) //1w w/ hour res
	c.RRA("AVERAGE", 0.5, 3600, 744) //1month
	err := c.Create(overwrite)
	return err
}

//func BuildGraph(title string) *rrd.Grapher {
//	g := rrd.NewGrapher()
//	g.SetTitle(title)
//	g.SetSize(750, 300)
//	g.Def("1", dbfile, "1", "AVERAGE")
//	g.Def("2", dbfile, "2", "AVERAGE")
//	g.Line(2, "1", "ff0000", "597")
//	g.Line(2, "2", "00ff00", "555")
//
//	return g
//}

func main() {
	app := cli.NewApp()
	app.Version = version
	app.Name = "PingTest"
	app.Usage = "testing the pings"
	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:  "overwrite",
			Usage: "Overwrite the RRD file(s) if they exist Default: False",
		},
	}
	app.Action = func(context *cli.Context) {
		ticker5 := time.NewTicker(5 * time.Second)
		overwrite := context.GlobalBool("overwrite")

		f, err := os.OpenFile("/home/ggreen/tmp/pingtest/pingfail2.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			fmt.Errorf("error opening file: %v", err)
		}
		defer f.Close()

		log.SetOutput(f)
		log.Println("Starting PingTest")

		hosts := make(map[string]*host)
		for _, val := range context.Args() {
			split := strings.Split(val, ":")
			theHost := host{}
			if len(split) == 1 {
				theHost = host{name: split[0], ip: net.ParseIP(split[0])}
			} else {
				theHost = host{name: split[0], ip: net.ParseIP(split[1])}
			}

			hosts[theHost.ip.String()] = &theHost
			theHost.rrdFile = fmt.Sprintf("%s%s.rrd", rrd_dir, theHost.name)
			goutils.Check(BuildRRD(theHost.rrdFile, overwrite))
		}

		results := make(map[string]*response)

		rttChan := make(chan *response, 6)
		failChan := make(chan *host, 6)

		go func() {
			for {
				select {
				case res := <-rttChan:
					if host, ok := hosts[res.addr.String()]; ok {
						dbfile := fmt.Sprintf("%s%s.rrd", rrd_dir, host.name)
						u := rrd.NewUpdater(dbfile)
						err := u.Update(time.Now(), res.rtt.Nanoseconds(), 0)
						goutils.Check(err)
					}
				case host := <-failChan:
					host.fails++
					dbfile := fmt.Sprintf("%s%s.rrd", rrd_dir, host.name)
					u := rrd.NewUpdater(dbfile)
					err := u.Update(time.Now(), 0, 1)
					goutils.Check(err)
				case <-ticker5.C:
					fmt.Println("ticker5")
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
				log.Println("Fail Count")
				for _, host := range hosts {
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
						log.Printf("%v, %s, %s\n", time.Now().Format(time.RFC3339), hosts[hostIP].name, hostIP)
						failChan <- hosts[hostIP]
					} else {
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

	app.Run(os.Args)
}
