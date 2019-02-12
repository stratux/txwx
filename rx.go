package main

import (
	"./proto"
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"github.com/cyoung/uatsynth"
	"github.com/golang/protobuf/proto"
	"github.com/kellydunn/golang-geo"
	"hash/crc64"
	"log"
	"os"
	"time"

	uatradio "../gouatradio"
	humanize "github.com/dustin/go-humanize"
)

const (
	RECEIVE_LOG = "/var/log/messages_received.log"
)

type status struct {
	MessagesReceived uint64
	CRCErrors        uint64
}

var globalStatus status

// Run options.
var manualLat float64 // Manually entered station lat.
var manualLng float64 // Manually entered station lng.

// Message logging.
var receiveLogFp *os.File

func generateUATEncodedTextReportMessage(msg *txwx.WeatherMessage) {
	// Observation time - zulu.
	observationTime := time.Unix(int64(msg.ObservationTime), 0)

	var uatMsg uatsynth.UATMsg
	uatMsg.Decoded = true
	uatMsg.Lat = float64(msg.StationLat)
	uatMsg.Lon = float64(msg.StationLng)
	uatMsg.UTCCoupled = true
	f := new(uatsynth.UATFrame)
	switch msg.Type {
	case txwx.WeatherMessage_METAR:
		f.Text_data = []string{"METAR " + msg.TextData}
	case txwx.WeatherMessage_TAF:
		f.Text_data = []string{"TAF " + msg.TextData}
	}
	f.FISB_hours = uint32(observationTime.Hour())
	f.FISB_minutes = uint32(observationTime.Minute())
	f.Product_id = 413
	f.Frame_type = 0
	uatMsg.Frames = append(uatMsg.Frames, f)
	encodedMessages, err := uatMsg.EncodeUplink()
	if err != nil {
		log.Printf("error encoding: %s\n", err.Error())
		return
	}
	for _, m := range encodedMessages {
		fmt.Printf("+")
		for i := 0; i < len(m); i++ {
			fmt.Printf("%02x", m[i])
		}
		fmt.Printf(";\n")
	}
}

func printStats() {
	statTimer := time.NewTicker(1 * time.Minute)
	startTime := time.Now()
	for {
		<-statTimer.C
		log.Printf("stats [started: %s]\n", humanize.RelTime(startTime, time.Now(), "ago", "from now"))
		log.Printf(" - Messages received: %d, CRC errors: %d.\n", globalStatus.MessagesReceived, globalStatus.CRCErrors)
		log.Printf(" - Current location: (%0.4f, %0.4f).\n", Location.GPSLatitude, Location.GPSLongitude)
	}
}

func startup() {
	flag.Float64Var(&globalSettings.ManualLat, "lat", 0.0, "Station latitude. If entered with longitude, GPS data is not used.")
	flag.Float64Var(&globalSettings.ManualLng, "lng", 0.0, "Station longitude. If entered with latitude, GPS data is not used.")

	flag.Parse()
}

func openReceiveLog() {
	fp, err := os.OpenFile(RECEIVE_LOG, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		fmt.Printf("Unable to open logfile: %s\n", err.Error())
		panic(err)
		return
	}
	receiveLogFp = fp
}

func writeReceiveLog(stationLat float32, stationLng float32, msg string) {
	fmt.Fprintf(receiveLogFp, "%0.4f,%0.4f,%0.4f,%0.4f,%s\n", stationLat, stationLng, Location.GPSLatitude, Location.GPSLongitude, msg)
}

func main() {
	startup()
	setupLogging("/var/log/rxwx.log") // Open logfile, set "log" output to save there and print to stdout.

	readSettings()

	if globalSettings.Mode != MODE_RX {
		log.Printf("Configuration file does not enable RX mode. Doing nothing.\n")
		//FIXME: Since this binary will be temporarily run and piped to Stratux in the rxwx/txwx
		// configuration, we just loop to keep gen_gdl90 running.
		for {
			time.Sleep(1 * time.Second)
		}
		return
	}

	openReceiveLog()

	go printStats()

	u, err := uatradio.NewUATRadio(globalSettings.Freq, globalSettings.RadioModMode)
	if err != nil {
		log.Printf("Unable to open radio: %s\n", err.Error())
		panic(err)
	}

	if globalSettings.ManualLat != 0. || globalSettings.ManualLng != 0. {
		// Save the manually entered lat/lng as the current location, for now. If a GPS lock is obtained,
		//  then this position will be overwritten.
		Location.GPSLatitude = float32(globalSettings.ManualLat)
		Location.GPSLongitude = float32(globalSettings.ManualLng)
		stationGeoPt = geo.NewPoint(globalSettings.ManualLat, globalSettings.ManualLng)
	}

	go situationUpdater() // Update current station position from Stratux.

	crc64Table := crc64.MakeTable(crc64.ECMA)

	c := make(chan uatradio.UATRadioMessage, 1024)

	u.SubscribeListener(c)

	n := 0

	for {
		radioMsg := <-c

		var msgLen uint16
		var crc uint64

		dataBuf := bytes.NewReader(radioMsg.Data)
		err := binary.Read(dataBuf, binary.LittleEndian, &msgLen)
		if err != nil {
			//fmt.Printf("binary.Read(): %s\n", err.Error())
			continue
		}
		err = binary.Read(dataBuf, binary.LittleEndian, &crc)
		if err != nil {
			//fmt.Printf("binary.Read(): %s\n", err.Error())
			continue
		}

		if int(msgLen)+10 > len(radioMsg.Data) {
			//fmt.Printf("msgLen=%d, len(radioMsg.Data)=%d. skipping.\n", msgLen, len(radioMsg.Data))
			continue
		}

		globalStatus.MessagesReceived++

		// Trim data according to msgLen.
		trimmedData := radioMsg.Data[10 : msgLen+10]

		// Calculate CRC of the message.
		calculatedCRC := crc64.Checksum(trimmedData, crc64Table)
		if crc != calculatedCRC {
			//fmt.Printf("skipping - CRC bad.\n")
			writeReceiveLog(0, 0, "crcbad")
			globalStatus.CRCErrors++
			continue
		}
		//fmt.Printf("msgLen=%d, crc=%d, calculatedCRC=%d\n", msgLen, crc, calculatedCRC)

		msg := new(txwx.WeatherMessage)
		err = proto.Unmarshal(trimmedData, msg)
		if err != nil {
			log.Printf("err proto.Unmarshal(): %s\n", err.Error())
			continue
		}

		switch msg.Type {
		case txwx.WeatherMessage_METAR, txwx.WeatherMessage_TAF:
			generateUATEncodedTextReportMessage(msg)
			writeReceiveLog(msg.StationLat, msg.StationLng, msg.TextData)
		case txwx.WeatherMessage_BEACON:
			if msg.ServerStatus != nil {
				beaconStr := fmt.Sprintf("TimeOk=%t, WeatherUpdatesOk=%t, MetarsTracked=%d, TafsTracked=%d", msg.ServerStatus.TimeOk, msg.ServerStatus.WeatherUpdatesOk, msg.ServerStatus.MetarsTracked, msg.ServerStatus.TafsTracked)
				log.Printf("Received beacon message from station (%0.4f, %0.4f): %s.\n", msg.StationLat, msg.StationLng, beaconStr)
				writeReceiveLog(msg.StationLat, msg.StationLng, beaconStr)
			}
		default:
		}

		n++
	}
}
