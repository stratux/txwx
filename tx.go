package main

import (
	uatradio "../gouatradio"
	"./proto"
	"fmt"
	"github.com/cyoung/ADDS"
	"github.com/golang/protobuf/proto"
	"time"
)

func main() {
	u, err := uatradio.NewUATRadio()

	if err != nil {
		panic(err)
	}

	for {
		// Get all Canadian METARs.
		p, err := ADDS.GetADDSMETARsByIdent("C")
		if err != nil {
			panic(err)
		}
		for _, v := range p {
			tn := time.Now().Unix()
			msg := &txwx.WeatherMessage{
				Type:       txwx.WeatherMessage_METAR,
				TxTime:     uint32(tn),
				StationLat: 44.168290,
				StationLng: -81.629470,
				TextData:   v.Text}
			fmt.Printf("%v\n", msg)
			data, err := proto.Marshal(msg)
			if err != nil {
				panic(err)
			}
			if len(data) < 150 {
				u.TX(data)
			}
		}
		time.Sleep(30 * time.Second)
	}
}
