package main

//output logs to console or to loggly
//pool output and bulk post every X seconds, default to 10
//json log format:
//TODO

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/grahamgreen/goutils"
	"github.com/tatsushid/go-fastping"
	"gopkg.in/urfave/cli.v1"

	log "github.com/sirupsen/logrus"
)

const (
	version   = "0.2.0"
	step      = 1
	heartbeat = 2 * step
)

type response struct {
	addr *net.IPAddr
	rtt  time.Duration
}

type host struct {
	name string
	ip   net.IP
}

func main() {
	var logfile string
	app := cli.NewApp()
	app.Version = version
	app.Name = "PingTest"
	app.Usage = "testing the pings"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:        "logfile, l",
			Value:       "/var/log/pingtest.log",
			Usage:       "Log output to `FILE`",
			Destination: &logfile,
		},
	}

	app.Action = func(context *cli.Context) {
		f, err := os.OpenFile(logfile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			fmt.Errorf("error opening file: %v", err)
		}
		defer f.Close()
		log.SetFormatter(&log.JSONFormatter{})
		log.SetOutput(f)
		log.SetLevel(log.InfoLevel)

		log.Info("Starting PingTest")

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
		}

		results := make(map[string]*response)

		rttChan := make(chan *response, 6)
		failChan := make(chan *host, 6)

		go func() {
			for {
				select {
				case res := <-rttChan:
					if host, ok := hosts[res.addr.String()]; ok {
						log.WithFields(log.Fields{
							"rtt":  res.rtt.Seconds(),
							"host": host.name,
							"fail": 0,
						}).Info("success")
					}
				case host := <-failChan:
					log.WithFields(log.Fields{
						"rtt":  0,
						"host": host.name,
						"fail": 1,
					}).Info("fail")
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
				log.Debug("Got interrupted; stopping ping test")
				break loop
			case res := <-onRecv:
				if _, ok := results[res.addr.String()]; ok {
					results[res.addr.String()] = res
				}
			case <-onIdle:
				for hostIP, r := range results {
					if r == nil {
						failChan <- hosts[hostIP]
					} else {
						rttChan <- r
					}
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
