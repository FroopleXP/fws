package main

import (
	"log"
	"net"
)

const sockAddr string = ":3000"

type status uint16

const (
	statusNormal = status(iota + 1000)
	statusGoingAway
	statusProtoErr
	statusUnacceptable
	_
	_
	_
	statusViolation
	statusTooBig
	_
	statusUnexpected
)

func (s status) String() string {
	switch s {
	case statusNormal:
		return "normal"
	case statusGoingAway:
		return "going away"
	case statusProtoErr:
		return "protocol error"
	case statusUnacceptable:
		return "unacceptable data"
	case statusViolation:
		return "violation"
	case statusTooBig:
		return "message too big to process"
	case statusUnexpected:
		return "unexpected error during processing"
	}
	return "unknown"
}

func handle(c net.Conn) {
	defer func(c net.Conn) {
		log.Printf("Closing connection to %s\n", c.RemoteAddr())
		c.Close()
	}(c)

	var conn *conn = newConn(c)

	// When 'handle' is done, so is the client so we can close the connection
	if err := conn.handle(); err != nil {
		log.Printf("failed to handle connection: %v\n", err)
		return
	}

	log.Printf("client %s disconnected\n", c.RemoteAddr())
}

func main() {
	log.Printf("starting socket server on %s", sockAddr)
	l, err := net.Listen("tcp", sockAddr)
	if err != nil {
		log.Fatalf("failed to start socket server: %v\n", err)
	}

	for {
		c, err := l.Accept()
		if err != nil {
			log.Printf("failed to accept incoming connection: %v\n", err)
			continue
		}

		if err := upgrade(c); err != nil {
			log.Printf("failed to upgrade client: %v\n", err)
		}

		log.Printf("new connection from %s\n", c.RemoteAddr())
		go handle(c)
	}
}
