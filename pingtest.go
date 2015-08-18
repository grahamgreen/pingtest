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
	name      string
	ip        net.IP
	fails     uint64
	rrdFile   string
	rttColor  string
	failColor string
}

func BuildRRD(dbfile string, overwrite bool) error {
	now := time.Now()
	start := now.Add(-5 * time.Second)

	c := rrd.NewCreator(dbfile, start, step)
	c.DS("rtt", "GAUGE", heartbeat, "U", "U")
	c.DS("fail", "GAUGE", heartbeat, "U", "U")
	c.RRA("AVERAGE", 0.5, 1, 300)    //5min w/ sec res
	c.RRA("AVERAGE", 0.5, 10, 90)    //10min w/ sec res
	c.RRA("AVERAGE", 0.5, 60, 60)    //1h w/ min res
	c.RRA("AVERAGE", 0.5, 60, 360)   //6h
	c.RRA("AVERAGE", 0.5, 60, 720)   //12h
	c.RRA("AVERAGE", 0.5, 60, 1440)  //24h
	c.RRA("AVERAGE", 0.5, 60, 10080) //1w
	c.RRA("AVERAGE", 0.5, 60, 44640) //1month
	err := c.Create(overwrite)
	return err
}

func BuildGraph(hosts map[string]*host) *rrd.Grapher {
	g := rrd.NewGrapher()
	g.SetTitle("Hosts")
	g.SetSize(750, 300)
	g.SetSlopeMode()
	for _, host := range hosts {
		rttName := fmt.Sprintf("%s_rtt", host.name)
		failName := fmt.Sprintf("%s_fail", host.name)
		g.Def(rttName, host.rrdFile, "rtt", "AVERAGE")
		g.Def(failName, host.rrdFile, "fail", "AVERAGE")
		g.Line(2, rttName, host.rttColor, rttName)
		g.Tick(failName, host.failColor, "1.0")
		//g.Line(2, failName, host.failColor, failName)
	}

	return g
}

func main() {
	rtt_colors := []string{"00bb00", "009600", "005e00"}
	fail_colors := []string{"ff0000", "cc0000", "800000"}
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
		ticker60 := time.NewTicker(60 * time.Second)
		ticker600 := time.NewTicker(600 * time.Second)
		ticker3600 := time.NewTicker(3600 * time.Second)

		overwrite := context.GlobalBool("overwrite")

		f, err := os.OpenFile("/home/ggreen/tmp/pingtest/pingfail2.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			fmt.Errorf("error opening file: %v", err)
		}
		defer f.Close()

		log.SetOutput(f)
		log.Println("Starting PingTest")

		hosts := make(map[string]*host)
		for i, val := range context.Args() {
			split := strings.Split(val, ":")
			theHost := host{}
			if len(split) == 1 {
				theHost = host{name: split[0], ip: net.ParseIP(split[0])}
			} else {
				theHost = host{name: split[0], ip: net.ParseIP(split[1])}
			}

			hosts[theHost.ip.String()] = &theHost
			theHost.rrdFile = fmt.Sprintf("%s%s.rrd", rrd_dir, theHost.name)
			theHost.rttColor = rtt_colors[i]
			theHost.failColor = fail_colors[i]
			err := BuildRRD(theHost.rrdFile, overwrite)
			if err != nil && overwrite {
				goutils.Check(err)
			}
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
						err := u.Update(time.Now(), res.rtt.Seconds()*1e3, 0)
						fmt.Printf("%s -- %v\n", host.name, res.rtt.Seconds()*1e3)
						goutils.Check(err)
					}
				case host := <-failChan:
					host.fails++
					dbfile := fmt.Sprintf("%s%s.rrd", rrd_dir, host.name)
					u := rrd.NewUpdater(dbfile)
					err := u.Update(time.Now(), 0, 1)
					goutils.Check(err)
				case <-ticker5.C:
					g := BuildGraph(hosts)
					g.SetTitle("RTT 1 Min")
					now := time.Now()
					_, err := g.SaveGraph("/tmp/rtt_1min.png", now.Add(-60*time.Second), now)
					goutils.Check(err)
				case <-ticker60.C:
					g := BuildGraph(hosts)

					g.SetTitle("RTT 5 Min")
					now := time.Now()
					_, err := g.SaveGraph("/tmp/rtt_5min.png", now.Add(-300*time.Second), now)
					goutils.Check(err)

					g.SetTitle("RTT 15 Min")
					_, err = g.SaveGraph("/tmp/rtt_15min.png", now.Add(-900*time.Second), now)
					goutils.Check(err)
				case <-ticker600.C:
					g := BuildGraph(hosts)
					g.SetTitle("RTT 1 Hr")
					now := time.Now()
					_, err := g.SaveGraph("/tmp/rtt_60min.png", now.Add(-3600*time.Second), now)
					goutils.Check(err)

					g.SetTitle("RTT 6 Hrs")
					_, err = g.SaveGraph("/tmp/rtt_6h.png", now.Add(-6*time.Hour), now)
					goutils.Check(err)

					g.SetTitle("RTT 12Hrs")
					_, err = g.SaveGraph("/tmp/rtt_12h.png", now.Add(-12*time.Hour), now)
					goutils.Check(err)
				case <-ticker3600.C:
					g := BuildGraph(hosts)
					g.SetTitle("RTT 24 Hrs")
					now := time.Now()
					_, err := g.SaveGraph("/tmp/rtt_1d.png", now.Add(-24*time.Hour), now)
					goutils.Check(err)

					g.SetTitle("RTT 7 Days")
					_, err = g.SaveGraph("/tmp/rtt_1w.png", now.Add(-168*time.Hour), now)
					goutils.Check(err)

					g.SetTitle("RTT 31 Days")
					_, err = g.SaveGraph("/tmp/rtt_1m.png", now.Add(-744*time.Hour), now)
					goutils.Check(err)
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
