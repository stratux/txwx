package main

import (
	"encoding/json"
	"fmt"
	"github.com/kellydunn/golang-geo"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"
)

const (
	CONFIG_LOCATION = "/boot/txwx.conf"
	SITUATION_URL   = "http://localhost/getSituation"

	MODE_TX = 0
	MODE_RX = 1
)

type settings struct {
	Mode         int
	RadioModMode byte
	Freq         float64
	ManualLat    float64 // Manually configured location.
	ManualLng    float64 // Manually configured location.
}

var globalSettings settings

type MySituation struct {
	GPSLatitude    float32
	GPSLongitude   float32
	GPSAltitudeMSL float32 // Feet MSL
	GPSFixQuality  uint8
	GPSTime        time.Time
}

var Location MySituation    // Station GPS data.
var stationGeoPt *geo.Point // Station location.

// Logging.

func setupLogging(fn string) {
	fp, err := os.OpenFile(fn, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		fmt.Printf("Unable to open logfile: %s\n", err.Error())
		return
	}
	mfp := io.MultiWriter(fp, os.Stdout)
	log.SetOutput(mfp)
}

// Location.

func situationUpdater() {
	situationUpdateTicker := time.NewTicker(1 * time.Second)
	for {
		<-situationUpdateTicker.C

		resp, err := http.Get(SITUATION_URL)
		if err != nil {
			log.Printf("HTTP GET error: %s\n", err.Error())
			continue
		}

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Printf("HTTP GET body error: %s\n", err.Error())
			resp.Body.Close()
			continue
		}

		err = json.Unmarshal(body, &Location)

		if err != nil {
			log.Printf("HTTP JSON unmarshal error: %s\n", err.Error())
		}
		resp.Body.Close()
		if Location.GPSFixQuality > 0 {
			if stationGeoPt == nil {
				// First lock.
				log.Printf("First GPS location obtained.\n")
			}
			stationGeoPt = geo.NewPoint(float64(Location.GPSLatitude), float64(Location.GPSLongitude))
		}
	}
}

// Settings.

func defaultSettings() {
	globalSettings.Mode = MODE_TX
	globalSettings.Freq = 915.00
	globalSettings.RadioModMode = 1
}

func readSettings() {
	fd, err := os.Open(CONFIG_LOCATION)
	if err != nil {
		log.Printf("can't read settings %s: %s\n", CONFIG_LOCATION, err.Error())
		defaultSettings()
		saveSettings()
		return
	}
	defer fd.Close()
	buf := make([]byte, 1024)
	count, err := fd.Read(buf)
	if err != nil {
		log.Printf("can't read settings %s: %s\n", CONFIG_LOCATION, err.Error())
		defaultSettings()
		return
	}
	var newSettings settings
	err = json.Unmarshal(buf[0:count], &newSettings)
	if err != nil {
		log.Printf("can't read settings %s: %s\n", CONFIG_LOCATION, err.Error())
		defaultSettings()
		return
	}
	globalSettings = newSettings
	log.Printf("read in settings.\n")
}

func saveSettings() {
	fd, err := os.OpenFile(CONFIG_LOCATION, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(0644))
	if err != nil {
		log.Printf("Can't save settings %s: %s", CONFIG_LOCATION, err.Error())
		return
	}
	defer fd.Close()
	jsonSettings, _ := json.Marshal(&globalSettings)
	fd.Write(jsonSettings)
	log.Printf("Wrote settings.\n")
}
