package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/grahamgreen/goutils"
	"github.com/matryer/try"
	"github.com/ziutek/rrd"
)

const (
	dbfile    = "/home/ggreen/power.rrd"
	step      = 1
	heartbeat = 2 * step
)

type Downstream struct {
	Name           string
	DCID           string
	Freq           string
	Power          float64
	SNR            float64
	Modulation     string
	Octets         int64
	Correcteds     int64
	Uncorrectables int64
}

type Upstream struct {
	Name        string
	UCID        int64
	Freq        float64
	Power       float64
	ChannelType string
	SymbolRate  int64
	Modulation  string
}

type Status struct {
	Uptime   time.Duration
	DateTime time.Time
}

type Record struct {
	DS   []Downstream
	US   []Upstream
	Stat Status
}

func BuildRRD() {
	now := time.Now()
	start := now.Add(-20 * time.Second)

	c := rrd.NewCreator(dbfile, start, step)
	c.DS("1", "GAUGE", heartbeat, "U", "U")
	c.DS("2", "GAUGE", heartbeat, "U", "U")
	c.DS("3", "GAUGE", heartbeat, "U", "U")
	c.DS("4", "GAUGE", heartbeat, "U", "U")
	c.DS("5", "GAUGE", heartbeat, "U", "U")
	c.DS("6", "GAUGE", heartbeat, "U", "U")
	c.DS("7", "GAUGE", heartbeat, "U", "U")
	c.DS("8", "GAUGE", heartbeat, "U", "U")
	c.RRA("AVERAGE", 0.5, 1, 300)
	c.RRA("AVERAGE", 0.5, 10, 90)
	c.RRA("AVERAGE", 0.5, 60, 60)
	//add longer averages
	err := c.Create(true)
	goutils.Check(err)
}

func ParseDS(ds []string) Downstream {
	var theDS Downstream
	var err error
	theDS.Name = ds[0]
	theDS.DCID = strings.TrimSpace(ds[1])
	freqHolder := strings.Split(ds[2], " ")
	theDS.Freq = freqHolder[1]
	powerHolder := strings.Split(ds[3], " ")
	theDS.Power, err = strconv.ParseFloat(powerHolder[1], 64)
	goutils.Check(err)
	snrHolder := strings.Split(ds[4], " ")
	theDS.SNR, err = strconv.ParseFloat(snrHolder[1], 64)
	goutils.Check(err)
	theDS.Modulation = ds[5]
	theDS.Octets, err = strconv.ParseInt(strings.TrimSpace(ds[6]), 10, 64)
	goutils.Check(err)
	theDS.Correcteds, err = strconv.ParseInt(strings.TrimSpace(ds[7]), 10, 64)
	goutils.Check(err)
	theDS.Uncorrectables, err = strconv.ParseInt(strings.TrimSpace(ds[8]), 10, 64)
	goutils.Check(err)

	return theDS
}

func ParseUS(us []string) Upstream {
	var theUS Upstream
	var err error
	theUS.Name = us[0]
	theUS.UCID, err = strconv.ParseInt(strings.TrimSpace(us[1]), 10, 64)
	goutils.Check(err)
	freqHolder := strings.Split(us[2], " ")
	theUS.Freq, err = strconv.ParseFloat(freqHolder[1], 64)
	goutils.Check(err)
	powerHolder := strings.Split(us[3], " ")
	theUS.Power, err = strconv.ParseFloat(powerHolder[1], 64)
	goutils.Check(err)
	theUS.ChannelType = us[4]
	srHolder := strings.Split(us[5], " ")
	theUS.SymbolRate, err = strconv.ParseInt(strings.TrimSpace(srHolder[1]), 10, 64)
	goutils.Check(err)
	theUS.Modulation = us[6]

	return theUS

}

func CleanString(s string) string {
	//remove spaces
	s = strings.TrimSpace(s)
	//remove last char
	s = s[:len(s)-1]
	s = strings.TrimSpace(s)

	return s
}

func ArrisScrape(rec chan Record) {
	loc, _ := time.LoadLocation("Local")
	var lineHolder bytes.Buffer
	var allDS []Downstream
	var allUS []Upstream
	var status Status
	var doc *goquery.Document
	err := try.Do(func(attempt int) (bool, error) {
		var err error
		doc, err = goquery.NewDocument("http://192.168.100.1/cgi-bin/status_cgi")
		if err != nil {
			time.Sleep(2 * time.Second)
		}
		return attempt < 5, err
	})
	goutils.Check(err)

	doc.Find("h4").Each(func(i int, s *goquery.Selection) {
		if s.Text() == " Downstream " {
			s.Next().Find("tr").Each(func(j int, s2 *goquery.Selection) {
				if len(s2.Find("td").First().Text()) > 0 {
					//fmt.Printf("%s\n", s2.Text())
					s2.Find("td").Each(func(k int, s3 *goquery.Selection) {
						lineHolder.WriteString(s3.Text() + ", ")
						//fmt.Printf("%s, ", s3.Text())
					})
					theSplitLine := strings.Split(lineHolder.String(), ",")
					allDS = append(allDS, ParseDS(theSplitLine))
					lineHolder.Reset()
				}
			})
		}
		if s.Text() == " Upstream " {
			s.Next().Find("tr").Each(func(j int, s2 *goquery.Selection) {
				if len(s2.Find("td").First().Text()) > 0 {
					//fmt.Printf("%s\n", s2.Text())
					s2.Find("td").Each(func(k int, s3 *goquery.Selection) {
						lineHolder.WriteString(s3.Text() + ", ")
						//fmt.Printf("%s, ", s3.Text())
					})
					theSplitLine := strings.Split(lineHolder.String(), ",")
					allUS = append(allUS, ParseUS(theSplitLine))

					lineHolder.Reset()
				}
			})

		}
		//send allus down the ds channel
	})
	//this sucks and searches the td's twice
	doc.Find("td").Each(func(i int, s *goquery.Selection) {
		if s.Text() == "System Uptime: " {
			//fmt.Println(s.Next().Text())
			poo := strings.Split(s.Next().Text(), ":")
			d, h, m := poo[0], poo[1], poo[2]
			dInt, err := strconv.ParseInt(CleanString(d), 10, 64)
			goutils.Check(err)
			hInt, err := strconv.ParseInt(CleanString(h), 10, 64)
			goutils.Check(err)
			mInt, err := strconv.ParseInt(CleanString(m), 10, 64)
			goutils.Check(err)
			hInt += dInt * 24
			dString := fmt.Sprintf("%dh%dm", hInt, mInt)
			dur, err := time.ParseDuration(dString)
			goutils.Check(err)
			status.Uptime = dur
		}
		if s.Text() == "Time and Date:" {
			tString := s.Next().Text()
			t, err := time.ParseInLocation("Mon 2006-01-02 15:04:05", tString, loc)
			goutils.Check(err)
			status.DateTime = t
		}
	})
	rec <- Record{DS: allDS, US: allUS, Stat: status}
}

func BuildPowerGraph() *rrd.Grapher {
	g := rrd.NewGrapher()
	g.SetTitle("Power 1 Min")
	g.SetSize(750, 300)
	g.Def("1", dbfile, "1", "AVERAGE")
	g.Def("2", dbfile, "2", "AVERAGE")
	g.Def("3", dbfile, "3", "AVERAGE")
	g.Def("4", dbfile, "4", "AVERAGE")
	g.Def("5", dbfile, "5", "AVERAGE")
	g.Def("6", dbfile, "6", "AVERAGE")
	g.Def("7", dbfile, "7", "AVERAGE")
	g.Def("8", dbfile, "8", "AVERAGE")
	g.Line(2, "1", "ff0000", "597")
	g.Line(2, "2", "00ff00", "555")
	g.Line(2, "3", "0000ff", "561")
	g.Line(2, "4", "E16E00", "567")
	g.Line(2, "5", "A0A0A3", "573")
	g.Line(2, "6", "5C654E", "579")
	g.Line(2, "7", "85C9FF", "585")
	g.Line(2, "8", "7FB37C", "591")

	return g
}

func main() {
	//BuildRRD()
	recordChan := make(chan Record, 8)
	//usChan := make(chan *Upstream, 5)
	ticker5 := time.NewTicker(5 * time.Second)
	ticker60 := time.NewTicker(60 * time.Second)
	ticker600 := time.NewTicker(600 * time.Second)
	ticker3600 := time.NewTicker(3600 * time.Second)
	go func() {
		for {
			select {
			case <-ticker5.C:
				g := BuildPowerGraph()
				now := time.Now()
				i, err := g.SaveGraph("/tmp/power_1min.png", now.Add(-60*time.Second), now)
				fmt.Printf("%+v\n", i)
				goutils.Check(err)
			case <-ticker60.C:
				g := BuildPowerGraph()
				now := time.Now()
				g.SetTitle("Power 5 Min")
				i, err := g.SaveGraph("/tmp/power_5min.png", now.Add(-300*time.Second), now)
				fmt.Printf("%+v\n", i)
				goutils.Check(err)
				g.SetTitle("Power 15 Min")
				i, err = g.SaveGraph("/tmp/power_15min.png", now.Add(-900*time.Second), now)
				fmt.Printf("%+v\n", i)
				goutils.Check(err)
			case <-ticker600.C:
				g := BuildPowerGraph()
				now := time.Now()
				g.SetTitle("Power 1 Hour")
				i, err := g.SaveGraph("/tmp/power_60min.png", now.Add(-3600*time.Second), now)
				fmt.Printf("%+v\n", i)
				goutils.Check(err)
				g.SetTitle("Power 6 Hours")
				i, err = g.SaveGraph("/tmp/power_6h.png", now.Add(-6*time.Hour), now)
				fmt.Printf("%+v\n", i)
				goutils.Check(err)
				g.SetTitle("Power 12 Hours")
				i, err = g.SaveGraph("/tmp/power_12h.png", now.Add(-12*time.Hour), now)
				fmt.Printf("%+v\n", i)
				goutils.Check(err)
			case <-ticker3600.C:
				g := BuildPowerGraph()
				now := time.Now()
				g.SetTitle("Power 24 Hrs")
				i, err := g.SaveGraph("/tmp/power_1d.png", now.Add(-24*time.Hour), now)
				fmt.Printf("%+v\n", i)
				goutils.Check(err)
				g.SetTitle("Power 7 Days")
				i, err = g.SaveGraph("/tmp/power_1w.png", now.Add(-168*time.Hour), now)
				fmt.Printf("%+v\n", i)
				goutils.Check(err)
				g.SetTitle("Power 31 Days")
				i, err = g.SaveGraph("/tmp/power_1m.png", now.Add(-744*time.Hour), now)
				fmt.Printf("%+v\n", i)
				goutils.Check(err)

			case rec := <-recordChan:
				//fmt.Println(rec)
				// just do the update
				// have the graphs redrawn on a tick
				u := rrd.NewUpdater(dbfile)
				err := u.Update(rec.Stat.DateTime, rec.DS[0].Power,
					rec.DS[1].Power, rec.DS[2].Power,
					rec.DS[3].Power, rec.DS[4].Power,
					rec.DS[5].Power, rec.DS[6].Power, rec.DS[7].Power)
				goutils.Check(err)
			}
		}
	}()
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	signal.Notify(c, syscall.SIGTERM)
loop:
	for {
		select {
		case <-c:
			fmt.Println("got interrupted")
			log.Println("Stopping Scrape")
			ticker5.Stop()
			ticker60.Stop()
			ticker600.Stop()
			ticker3600.Stop()
			break loop
		default:
			ArrisScrape(recordChan)
			time.Sleep(500 * time.Millisecond)
		}
	}
	signal.Stop(c)
}
