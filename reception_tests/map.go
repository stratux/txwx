package main

import (
	"bufio"
	"encoding/xml"
	"fmt"
	"github.com/kellydunn/golang-geo"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	TS_TOLERANCE = 1 * time.Second // Max difference in time between GDL90 message timestamp and track log time.
)

// Generated with "github.com/miku/zek".
type KML struct {
	XMLName  xml.Name `xml:"kml"`
	Text     string   `xml:",chardata"`
	Xmlns    string   `xml:"xmlns,attr"`
	Gx       string   `xml:"gx,attr"`
	Document struct {
		Text string `xml:",chardata"`
		Open struct {
			Text string `xml:",chardata"`
		} `xml:"open"`
		Visibility struct {
			Text string `xml:",chardata"`
		} `xml:"visibility"`
		Style []struct {
			Text      string `xml:",chardata"`
			ID        string `xml:"id,attr"`
			LineStyle struct {
				Text  string `xml:",chardata"`
				Color struct {
					Text string `xml:",chardata"`
				} `xml:"color"`
				Width struct {
					Text string `xml:",chardata"`
				} `xml:"width"`
			} `xml:"LineStyle"`
			PolyStyle struct {
				Text  string `xml:",chardata"`
				Color struct {
					Text string `xml:",chardata"`
				} `xml:"color"`
			} `xml:"PolyStyle"`
		} `xml:"Style"`
		Placemark []struct {
			Text string `xml:",chardata"`
			Name struct {
				Text string `xml:",chardata"`
			} `xml:"name"`
			StyleUrl struct {
				Text string `xml:",chardata"`
			} `xml:"styleUrl"`
			Track struct {
				Text         string `xml:",chardata"`
				AltitudeMode []struct {
					Text string `xml:",chardata"`
				} `xml:"altitudeMode"`
				Extrude struct {
					Text string `xml:",chardata"`
				} `xml:"extrude"`
				Interpolate struct {
					Text string `xml:",chardata"`
				} `xml:"interpolate"`
				When []struct {
					Text string `xml:",chardata"`
				} `xml:"when"`
				Coord []struct {
					Text string `xml:",chardata"`
				} `xml:"coord"`
			} `xml:"Track"`
			Point struct {
				Text        string `xml:",chardata"`
				Coordinates struct {
					Text string `xml:",chardata"`
				} `xml:"coordinates"`
			} `xml:"Point"`
		} `xml:"Placemark"`
		ExtendedData struct {
			Text       string `xml:",chardata"`
			SchemaData struct {
				Text            string `xml:",chardata"`
				SimpleArrayData []struct {
					Text  string `xml:",chardata"`
					Name  string `xml:"name,attr"`
					Value []struct {
						Text string `xml:",chardata"`
					} `xml:"value"`
				} `xml:"SimpleArrayData"`
			} `xml:"SchemaData"`
			Data []struct {
				Text        string `xml:",chardata"`
				Name        string `xml:"name,attr"`
				DisplayName struct {
					Text string `xml:",chardata"`
				} `xml:"displayName"`
				Value struct {
					Text string `xml:",chardata"`
				} `xml:"value"`
			} `xml:"Data"`
		} `xml:"ExtendedData"`
	} `xml:"Document"`
}

type Message struct {
	TS   time.Time
	Data string
}

type Track struct {
	TS                 time.Time
	Lat                float64
	Lng                float64
	Alt                float64 // Meters.
	AssociatedMessages []Message
}

func main() {
	tracks := make([]Track, 0)
	messages := make([]Message, 0)
	if len(os.Args) < 3 {
		fmt.Printf("usage: %s <tracklog.kml> <gdl90 log>\n", os.Args[0])
	}

	// Parse in the KML.
	var kml KML

	f, err := os.Open(os.Args[1])
	if err != nil {
		panic(err)
	}

	if err := xml.NewDecoder(f).Decode(&kml); err != nil {
		panic(err)
	}

	for k, v := range kml.Document.Placemark[0].Track.When {
		var thisTrack Track
		thisTrack.TS, err = time.Parse(time.RFC3339Nano, v.Text)
		if err != nil {
			panic(err)
		}

		x := strings.Split(kml.Document.Placemark[0].Track.Coord[k].Text, " ")
		lat, err := strconv.ParseFloat(x[1], 64)
		if err != nil {
			panic(err)
		}
		lng, err := strconv.ParseFloat(x[0], 64)
		if err != nil {
			panic(err)
		}
		alt, err := strconv.ParseFloat(x[2], 64)
		if err != nil {
			panic(err)
		}
		thisTrack.Lat = lat
		thisTrack.Lng = lng
		thisTrack.Alt = alt

		tracks = append(tracks, thisTrack)
	}
	f.Close()

	// Parse in the ".gdl90" file.
	f, err = os.Open(os.Args[2])
	if err != nil {
		panic(err)
	}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		s := scanner.Text()
		x := strings.Split(s, ",")
		if len(x) < 2 || s[0] == '#' {
			continue
		}
		// Only count uplink messages.
		if x[1][:4] != "7e07" {
			continue
		}
		var thisMessage Message
		ts, err := strconv.ParseFloat(x[0], 64)
		if err != nil {
			panic(err)
		}
		tsSec := int64(ts)
		tsNsec := int64((ts - float64(tsSec)) * 1000000000)
		thisMessage.TS = time.Unix(tsSec, tsNsec)
		thisMessage.Data = x[1]

		fmt.Printf("%d . %d\n", tsSec, tsNsec)
		fmt.Printf("%s\n", s)
		fmt.Printf("%v\n", thisMessage)
		messages = append(messages, thisMessage)
	}
	if err := scanner.Err(); err != nil {
		panic(err)
	}

	// Match up each track data point with messages.
	for k, v := range tracks {
		finMessages := make([]Message, 0)
		for i := 0; i < len(messages); i++ {
			if v.TS.After(messages[i].TS) && (v.TS.Sub(messages[i].TS) < TS_TOLERANCE) {
				// Associate this data message with this track.
				tracks[k].AssociatedMessages = append(tracks[k].AssociatedMessages, messages[i])
			} else {
				// Do not associate with this track. Add to finMessages to be returned back to 'messages'
				finMessages = append(finMessages, messages[i])
			}
		}
		messages = finMessages
	}

	if len(messages) > 0 {
		fmt.Printf("WARNING: some un-associated messages were left over.\n")
		fmt.Printf("%v\n", messages)
	}

	p := geo.NewPoint(44.251610, -81.601750)
	var maxDistPoint *geo.Point
	var maxDist float64
	var i int
	// Print the lat, lngs with positive messages.
	for _, v := range tracks {
		if len(v.AssociatedMessages) > 0 {
			p2 := geo.NewPoint(v.Lat, v.Lng)
			if d := p.GreatCircleDistance(p2); d > maxDist {
				maxDist = d
				maxDistPoint = p2
			}
			fmt.Printf("addDot(%f,%f,\"%d\", true);\n", v.Lat, v.Lng, len(v.AssociatedMessages))
		} else {
			if i%10 == 0 {
				// Print only 10% of the fail tracks.
				fmt.Printf("addDot(%f,%f,\"0\", false);\n", v.Lat, v.Lng)
			}
			i++
		}
	}

	fmt.Printf("addMaxDistLine(44.251610,-81.601750,%f,%f,%0.2f);\n", maxDistPoint.Lat(), maxDistPoint.Lng(), maxDist)
}
