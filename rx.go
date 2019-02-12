package main

import (
	uatradio "../gouatradio"
	"./proto"
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/cyoung/uatsynth"
	"github.com/golang/protobuf/proto"
	"hash/crc64"
	"time"
)

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
		fmt.Printf("error encoding: %s\n", err.Error())
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

func main() {
	u, err := uatradio.NewUATRadio()
	if err != nil {
		panic(err)
	}

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
			fmt.Printf("binary.Read(): %s\n", err.Error())
			continue
		}
		err = binary.Read(dataBuf, binary.LittleEndian, &crc)
		if err != nil {
			fmt.Printf("binary.Read(): %s\n", err.Error())
			continue
		}

		if int(msgLen)+10 > len(radioMsg.Data) {
			fmt.Printf("msgLen=%d, len(radioMsg.Data)=%d. skipping.\n", msgLen, len(radioMsg.Data))
			continue
		}

		// Trim data according to msgLen.
		trimmedData := radioMsg.Data[10 : msgLen+10]

		// Calculate CRC of the message.
		calculatedCRC := crc64.Checksum(trimmedData, crc64Table)
		if crc != calculatedCRC {
			fmt.Printf("skipping - CRC bad.\n")
			continue
		}
		fmt.Printf("msgLen=%d, crc=%d, calculatedCRC=%d\n", msgLen, crc, calculatedCRC)

		msg := new(txwx.WeatherMessage)
		err = proto.Unmarshal(trimmedData, msg)
		if err != nil {
			fmt.Printf("err proto.Unmarshal(): %s\n", err.Error())
			continue
		}

		switch msg.Type {
		case txwx.WeatherMessage_METAR, txwx.WeatherMessage_TAF:
			generateUATEncodedTextReportMessage(msg)
			fmt.Printf("OK: %s\n", msg.TextData)
		case txwx.WeatherMessage_BEACON:
			if msg.ServerStatus != nil {
				fmt.Printf("Received beacon message from station (%0.4f, %0.4f): TimeOk=%t, WeatherUpdatesOk=%t, MetarsTracked=%d, TafsTracked=%d.\n", msg.StationLat, msg.StationLng, msg.ServerStatus.TimeOk, msg.ServerStatus.WeatherUpdatesOk, msg.ServerStatus.MetarsTracked, msg.ServerStatus.TafsTracked)
			}
		default:
		}

		n++
	}
}
