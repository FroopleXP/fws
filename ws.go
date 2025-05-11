package main

import (
	"io"
	"log"
	"net"
)

const sockAddr string = ":3000"

type frameType uint8

const (
    continuation = frameType(iota)
    text
    binary
    // 3-7 are reserved
    _
    _
    _
    _
    _
    ping
    pong
)

func (f frameType) String() string {
    switch f {
    case continuation:
        return "continuation"
    case text:
        return "text"
    case binary:
        return "binary"
    case ping:
        return "ping"
    case pong:
        return "pong"
    }
    return ""
}

// Consts prefixed with 'm' declare a mask
const (
    mFin, mMask uint8 = 0x80, 0x80
    mOp         uint8 = 0x0f
    mPayloadLen uint8 = 0x7f
)

func handle(c net.Conn) {
	defer c.Close()

	var buf []byte = make([]byte, 1024)

	for {
		_, err := c.Read(buf)
		if err != nil {
			if err == io.EOF {
                log.Printf("client %s disconnected\n", c.RemoteAddr())
				return
			}
			log.Printf("failed to read from client: %v\n", err)
            return
		}

        ptr := 0 

        finRsvOp := buf[ptr]
        ptr++
    
        isFin := false
        if (finRsvOp & mFin) == mFin {
            isFin = true
        }

        fType := frameType(finRsvOp & mOp) 

        maskPayloadLen := buf[ptr]
        ptr++

        isMasked := false
        if (maskPayloadLen & mMask) == mMask {
            isMasked = true
        }

        payloadLen := uint64(maskPayloadLen & mPayloadLen)
        if payloadLen > 125 {
            if payloadLen == 126 {
                payloadLen = uint64(
                    uint16(buf[ptr]) << (16 - 8) |
                    uint16(buf[ptr+1]))
                ptr+=2
            } else if payloadLen == 127 {
                payloadLen = (
                    uint64(buf[ptr])   << (64 - 8)  |
                    uint64(buf[ptr+1]) << (64 - 16) |
                    uint64(buf[ptr+2]) << (64 - 24) |
                    uint64(buf[ptr+3]) << (64 - 32) |
                    uint64(buf[ptr+4]) << (64 - 40) |
                    uint64(buf[ptr+5]) << (64 - 48) |
                    uint64(buf[ptr+6]) << (64 - 56) |
                    uint64(buf[ptr+7]))
                ptr+=8
            }
        }

        var mask uint32 = 0
        if isMasked {
            mask = (
                uint32(buf[ptr])   << (32 - 8)  |
                uint32(buf[ptr+1]) << (32 - 16) |
                uint32(buf[ptr+2]) << (32 - 24) |
                uint32(buf[ptr+3]) << (32 - 32))
            ptr+=4
        }

        log.Printf("client frame isFin=%t, fType=%s, isMasked=%t, payloadLen=%d, mask=%d\n", isFin, fType, isMasked, payloadLen, mask) 
	}
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

		go handle(c)
	}
}
