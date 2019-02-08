package main

import (
	uatradio "../gouatradio"
	"./proto"
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/cyoung/ADDS"
	"github.com/golang/protobuf/proto"
	"github.com/kellydunn/golang-geo"
	"hash/crc64"
	"sync"
	"time"
)

const (
	STATION_LAT         = 44.168290
	STATION_LNG         = -81.629470
	REPORTS_UPDATE_TIME = 5 * time.Minute
)

func createMETARWeatherMessage(metar ADDS.ADDSMETAR) *txwx.WeatherMessage {
	tn := time.Now().Unix()
	return &txwx.WeatherMessage{
		Type:            txwx.WeatherMessage_METAR,
		TxTime:          uint32(tn),
		StationLat:      STATION_LAT,
		StationLng:      STATION_LNG,
		TextData:        metar.Text,
		ObservationTime: uint32(metar.Observation.Time.Unix()),
	}
}
func createTAFWeatherMessage(taf ADDS.ADDSTAF) *txwx.WeatherMessage {
	tn := time.Now().Unix()
	return &txwx.WeatherMessage{
		Type:            txwx.WeatherMessage_TAF,
		TxTime:          uint32(tn),
		StationLat:      STATION_LAT,
		StationLng:      STATION_LNG,
		TextData:        taf.Text,
		ObservationTime: uint32(taf.BulletinTime.Time.Unix()),
	}
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

var crc64Table *crc64.Table

var stationGeoPt *geo.Point // Station location.

var lookupMutex *sync.Mutex // Protects the following weather data variables.
var allMETARs []ADDS.ADDSMETAR
var allTAFs []ADDS.ADDSTAF

func updateWeather() {
	updateTicker := time.NewTicker(REPORTS_UPDATE_TIME)
	for {
		// Get all METARs within 500 sm.
		metars, err := ADDS.GetLatestADDSMETARsInRadiusOf(500, stationGeoPt)
		if err != nil {
			panic(err)
		}
		// Get all TAFs within 500 sm.
		tafs, err := ADDS.GetLatestADDSTAFsInRadiusOf(500, stationGeoPt)
		if err != nil {
			panic(err)
		}
		lookupMutex.Lock()
		allMETARs = metars
		allTAFs = tafs
		lookupMutex.Unlock()

		<-updateTicker.C
	}
}

func main() {
	lookupMutex = &sync.Mutex{}

	u, err := uatradio.NewUATRadio()

	if err != nil {
		panic(err)
	}

	crc64Table = crc64.MakeTable(crc64.ECMA)
	stationGeoPt = geo.NewPoint(STATION_LAT, STATION_LNG)

	go updateWeather()

	for {
		lookupMutex.Lock()
		metars := allMETARs
		tafs := allTAFs
		lookupMutex.Unlock()

		for _, v := range metars {
			fmt.Printf("METAR=%s\n", v.Text)
			msg := createMETARWeatherMessage(v)
			data := preparePacketFromWeatherMessage(msg)
			if len(data) < 150 {
				u.TX(data)
			}
		}

		for _, v := range tafs {
			if v.Text[:4] == "TAF" {
				v.Text = v.Text[4:]
			}
			fmt.Printf("TAF=%s\n", v.Text)
			msg := createTAFWeatherMessage(v)
			data := preparePacketFromWeatherMessage(msg)
			if len(data) < 150 {
				u.TX(data)
			}
		}

	}
}
