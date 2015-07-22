package main

import (
	"bytes"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/grahamgreen/goutils"
	"github.com/ziutek/rrd"
)

const (
	dbfile    = "/tmp/power.rrd"
	step      = 1
	heartbeat = 2 * step
)

type Downstream struct {
	Name           string
	DCID           int64
	Freq           float64
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

func BuildRRD() {
	c := rrd.NewCreator(dbfile, time.Now(), step)
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
	err := c.Create(true)
	goutils.Check(err)
}

func ParseDS(ds []string) Downstream {
	var theDS Downstream
	var err error
	theDS.Name = ds[0]
	theDS.DCID, err = strconv.ParseInt(strings.TrimSpace(ds[1]), 10, 64)
	goutils.Check(err)
	freqHolder := strings.Split(ds[2], " ")
	theDS.Freq, err = strconv.ParseFloat(freqHolder[1], 64)
	goutils.Check(err)
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

func ArrisScrape(ds chan []Downstream) {
	var lineHolder bytes.Buffer
	var allDS []Downstream
	var allUS []Upstream
	var status Status
	doc, err := goquery.NewDocument("http://192.168.100.1/cgi-bin/status_cgi")
	if err != nil {
		log.Fatal(err)
	}

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
			fmt.Println(s.Next().Text())
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
			t, err := time.Parse("Mon 2006-01-02 15:04:05", tString)
			goutils.Check(err)
			status.DateTime = t
		}
	})
	ds <- allDS
}

func main() {
	BuildRRD()
	dsChan := make(chan []Downstream, 2)
	//usChan := make(chan *Upstream, 5)
	go func() {
		for {
			select {
			case ds := <-dsChan:
				fmt.Println(ds)
				u := rrd.NewUpdater(dbfile)
				err := u.Update(time.Now(), ds[0].Power, ds[1].Power, ds[2].Power, ds[3].Power, ds[4].Power, ds[5].Power, ds[6].Power, ds[7].Power)
				goutils.Check(err)

			}
		}
	}()
	for i := 0; i < 3; i++ {
		ArrisScrape(dsChan)

		time.Sleep(2 * time.Second)
	}
	//	RRDTest()
	//	u := rrd.NewUpdater("/home/ggreen/test.rrd")
	//	for i := 0; i < 900; i++ {
	//		time.Sleep(1 * time.Second)
	//		err := u.Update(time.Now(), 1.5*float64(i))
	//		goutils.Check(err)
	//	}
	// Graph
	//g := rrd.NewGrapher()
	//g.SetTitle("Test")
	//g.SetSize(800, 300)
	//g.Def("v1", "/home/ggreen/test.rrd", "power", "AVERAGE")
	//g.VDef("avg2", "v1,AVERAGE")
	//g.Line(1, "v1", "ff0000", "var 1")

	//i, err := g.SaveGraph("/home/ggreen/test_rrd1.png", start, end)
	//fmt.Printf("%+v\n", i)
	//goutils.Check(err)

	//i, buf, err := g.Graph(start, end)
	//fmt.Printf("%+v\n", i)
	//goutils.Check(err)
	//err = ioutil.WriteFile("/home/ggreen/test_rrd2.png", buf, 0666)
	//goutils.Check(err)
	//
	//c := make(chan os.Signal, 1)
	//signal.Notify(c, os.Interrupt)
	//signal.Notify(c, syscall.SIGTERM)

}
