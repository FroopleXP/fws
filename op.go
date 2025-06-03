package main

type opCode uint8

const (
	continuation = opCode(iota)
	text
	binary
	// 3-7 are reserved
	_
	_
	_
	_
	_
	connclose
	ping
	pong
)

func (o opCode) String() string {
	switch o {
	case continuation:
		return "continuation"
	case text:
		return "text"
	case binary:
		return "binary"
	case connclose:
		return "connection close"
	case ping:
		return "ping"
	case pong:
		return "pong"
	}
	return "unknown"
}

func (o opCode) isControl() bool {
	switch o {
	case ping, pong, connclose:
		return true
	}
	return false
}

func (o opCode) isReserved() bool {
	if (o > 2 && o < 8) || o > 10 {
		return true
	}
	return false
}
