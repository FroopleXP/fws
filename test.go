package main

import (
	"log"
)

func main1() {

	var s []byte

	s = append(s, 0xff)
	s = append(s, 0xff)
	s = append(s, 0xff)
	s = append(s, 0xff)
	s = append(s, 0xff)
	s = append(s, 0xff)

	log.Printf("cap=%d, len=%d\n", cap(s), len(s))
}
