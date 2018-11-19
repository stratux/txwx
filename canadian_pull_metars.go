package main

import (
	"fmt"
	"github.com/cyoung/ADDS"
)

func main() {
	// Get all Canadian METARs.
	p, err := ADDS.GetADDSMETARsByIdent("C")
	if err != nil {
		panic(err)
	}
	for _, v := range p {
		fmt.Printf("%s\n", v.Text)
	}
}
