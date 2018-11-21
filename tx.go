package main

import (
	uatradio "../gouatradio"
	"./proto"
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/cyoung/ADDS"
	"github.com/golang/protobuf/proto"
	"hash/crc64"
	"time"
)

func main() {
	u, err := uatradio.NewUATRadio()

	if err != nil {
		panic(err)
	}

	crc64Table := crc64.MakeTable(crc64.ECMA)

	for {
		// Get all Canadian METARs.
		p, err := ADDS.GetADDSMETARsByIdent("C")
		if err != nil {
			panic(err)
		}
		for _, v := range p {
			tn := time.Now().Unix()
			msg := &txwx.WeatherMessage{
				Type:            txwx.WeatherMessage_METAR,
				TxTime:          uint32(tn),
				StationLat:      44.168290,
				StationLng:      -81.629470,
				TextData:        v.Text,
				ObservationTime: uint32(v.Observation.Time.Unix()),
			}
			fmt.Printf("%v\n", msg)
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

			if len(data) < 150 {
				u.TX(data)
			}
		}
		time.Sleep(30 * time.Second)
	}
}
