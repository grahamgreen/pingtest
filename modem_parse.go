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
	UCID        int
	Freq        float32
	Power       float32
	ChannelType string
	SymbolRate  int64
	Modulation  string
}

type Status struct {
	UptimeDay  int
	UptimeHour int
	UptimeMin  int
	DateTime   time.Time
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

func ExampleScrape() {
	var lineHolder bytes.Buffer
	doc, err := goquery.NewDocument("http://192.168.100.1/cgi-bin/status_cgi")
	if err != nil {
		log.Fatal(err)
	}

	doc.Find("h4").Each(func(i int, s *goquery.Selection) {
		if s.Text() == " Downstream " {
			//var ds Downstream
			s.Next().Find("tr").Each(func(j int, s2 *goquery.Selection) {
				if len(s2.Find("td").First().Text()) > 0 {
					//fmt.Printf("%s\n", s2.Text())
					s2.Find("td").Each(func(k int, s3 *goquery.Selection) {
						lineHolder.WriteString(s3.Text() + ", ")
						//fmt.Printf("%s, ", s3.Text())
					})
					theSplitLine := strings.Split(lineHolder.String(), ",")
					fmt.Println(ParseDS(theSplitLine))

					lineHolder.Reset()
				}
			})
		}
	})

}

func main() {
	ExampleScrape()
}
