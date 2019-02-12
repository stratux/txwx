package main

import (
	"./proto"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/cyoung/ADDS"
	"github.com/golang/protobuf/proto"
	"github.com/kellydunn/golang-geo"
	"hash/crc64"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	uatradio "../gouatradio"
	humanize "github.com/dustin/go-humanize"
)

const (
	SITUATION_URL       = "http://localhost/getSituation"
	REPORTS_UPDATE_TIME = 5 * time.Minute
	BEACON_TIME         = 1 * time.Second
)

type status struct {
	MessagesSent uint64
}

type MySituation struct {
	GPSLatitude    float32
	GPSLongitude   float32
	GPSAltitudeMSL float32 // Feet MSL
	GPSFixQuality  uint8
	GPSTime        time.Time
}

var globalStatus status

var Location MySituation

var crc64Table *crc64.Table

var stationGeoPt *geo.Point // Station location.

var lookupMutex *sync.Mutex // Protects the following weather data variables.
var allMETARs []ADDS.ADDSMETAR
var allTAFs []ADDS.ADDSTAF

// Run options.
var beaconMode bool // Just send beacons, don't send any weather.
var txMetars bool   // Send METARs on/off.
var txTafs bool     // Send TAFs on/off.

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

func createMETARWeatherMessage(metar ADDS.ADDSMETAR) *txwx.WeatherMessage {
	return &txwx.WeatherMessage{
		Type:            txwx.WeatherMessage_METAR,
		TxTime:          uint32(time.Now().Unix()),
		StationLat:      Location.GPSLatitude,
		StationLng:      Location.GPSLongitude,
		TextData:        metar.Text,
		ObservationTime: uint32(metar.Observation.Time.Unix()),
	}
}

func createTAFWeatherMessage(taf ADDS.ADDSTAF) *txwx.WeatherMessage {
	return &txwx.WeatherMessage{
		Type:            txwx.WeatherMessage_TAF,
		TxTime:          uint32(time.Now().Unix()),
		StationLat:      Location.GPSLatitude,
		StationLng:      Location.GPSLongitude,
		TextData:        taf.Text,
		ObservationTime: uint32(taf.BulletinTime.Time.Unix()),
	}
}

func sendBeaconMessage(u *uatradio.UATRadio) error {
	serverStatus := &txwx.ServerStatus{
		TimeOk:                 true, //FIXME.
		WeatherUpdatesOk:       true, //FIXME.
		MetarsTracked:          uint32(len(allMETARs)),
		TafsTracked:            uint32(len(allTAFs)),
		FreqBandStart:          902,                           //FIXME.
		FreqBandEnd:            928,                           //FIXME.
		FreqSchemeList:         []uint32{(915 - 902) * 65536}, //FIXME: 915 MHz fixed, for now.
		FreqSchemeDwell:        []uint32{10000},               //FIXME.
		FreqSchemeCurrentIndex: 0,                             //FIXME.
	}
	msg := &txwx.WeatherMessage{
		Type:            txwx.WeatherMessage_BEACON,
		TxTime:          uint32(time.Now().Unix()),
		StationLat:      Location.GPSLatitude,
		StationLng:      Location.GPSLongitude,
		ObservationTime: uint32(time.Now().Unix()),
		ServerStatus:    serverStatus,
	}
	return txWeatherMessage(u, msg)
}

func preparePacketFromWeatherMessage(msg *txwx.WeatherMessage) []byte {
	data, err := proto.Marshal(msg)
	if err != nil {
		panic(err)
	}
	// Add CRC.
	crc := crc64.Checksum(data, crc64Table)
	headerBuf := new(bytes.Buffer)
	msgLen := uint16(len(data))
	binary.Write(headerBuf, binary.LittleEndian, msgLen)
	binary.Write(headerBuf, binary.LittleEndian, crc)
	data = append(headerBuf.Bytes(), data...)
	return data
}

func txWeatherMessage(u *uatradio.UATRadio, msg *txwx.WeatherMessage) error {
	data := preparePacketFromWeatherMessage(msg)
	if len(data) < 150 {
		u.TX(data)
		globalStatus.MessagesSent++
		return nil
	}
	return errors.New("txWeatherMessage(): Message too long.")
}

func updateWeather() {
	updateTicker := time.NewTicker(REPORTS_UPDATE_TIME)
	for {
		for stationGeoPt == nil {
			log.Printf("Waiting for GPS position from Stratux...\n")
			time.Sleep(15 * time.Second)
		}
		var metars []ADDS.ADDSMETAR
		var tafs []ADDS.ADDSTAF
		var err error
		if txMetars { // Only request METARs when METAR TX is enabled.
			// Get all METARs within 500 sm.
			metars, err = ADDS.GetLatestADDSMETARsInRadiusOf(500, stationGeoPt)
			if err != nil {
				panic(err)
			}
		}
		if txTafs { // Only request TAFs when TAF TX is enabled.
			// Get all TAFs within 500 sm.
			tafs, err = ADDS.GetLatestADDSTAFsInRadiusOf(500, stationGeoPt)
			if err != nil {
				panic(err)
			}
		}
		lookupMutex.Lock()
		allMETARs = metars
		allTAFs = tafs
		lookupMutex.Unlock()

		<-updateTicker.C
	}
}

func printStats() {
	statTimer := time.NewTicker(1 * time.Minute)
	startTime := time.Now()
	for {
		<-statTimer.C
		log.Printf("stats [started: %s]\n", humanize.RelTime(startTime, time.Now(), "ago", "from now"))
		log.Printf(" - Messages sent: %d\n", globalStatus.MessagesSent)
	}
}

func startup() {
	lookupMutex = &sync.Mutex{}
	flag.BoolVar(&beaconMode, "beaconMode", false, "Transmit beacons only.")
	flag.BoolVar(&txMetars, "metars", true, "Transmit METARs. OFF in beaconMode, regardless of setting.")
	flag.BoolVar(&txTafs, "tafs", true, "Transmit TAFs. OFF in beaconMode, regardless of setting.")

	flag.Parse()
}

func main() {
	startup()
	beaconTicker := time.NewTicker(BEACON_TIME)

	fp, err := os.OpenFile("/var/log/txwx.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		fmt.Printf("Unable to open logfile: %s\n", err.Error())
		return
	}
	mfp := io.MultiWriter(fp, os.Stdout)
	log.SetOutput(mfp)

	u, err := uatradio.NewUATRadio()

	if err != nil {
		log.Printf("Unable to open radio: %s\n", err.Error())
		panic(err)
	}

	crc64Table = crc64.MakeTable(crc64.ECMA)

	go situationUpdater() // Update current station position from Stratux.
	go updateWeather()    // Update weather data from ADDS.
	go printStats()       // Periodically print stats.

	for {
		lookupMutex.Lock()
		metars := allMETARs
		tafs := allTAFs
		lookupMutex.Unlock()

		if !beaconMode {
			if txMetars {
				for _, v := range metars {
					msg := createMETARWeatherMessage(v)
					txWeatherMessage(u, msg)
				}
			}

			if txTafs {
				for _, v := range tafs {
					if v.Text[:4] == "TAF" {
						v.Text = v.Text[4:]
					}
					msg := createTAFWeatherMessage(v)
					txWeatherMessage(u, msg)
				}
			}
		}

		select {
		case <-beaconTicker.C:
			// Time to send a beacon message.
			sendBeaconMessage(u)
		default:
			// Default case so that the select doesn't block.
			//  In reality, if BEACON_TIME is shorter than it takes to transmit all weather data,
			//  a beacon message will be sent out on each loop.
		}
	}
}
