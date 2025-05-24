package main

import (
	"log"
	"net"
    "bufio"
    "fmt"
    "io"
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
    connclose
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
    case connclose:
        return "connection close"
    case ping:
        return "ping"
    case pong:
        return "pong"
    }
    return "unknown"
}

// Consts prefixed with 'm' declare a mask
const (
    mFin, mMask uint8 = 0x80, 0x80
    mOp         uint8 = 0x0f
    mPayloadLen uint8 = 0x7f
)

func handle(c net.Conn) {
	defer c.Close()

	//var buf []byte = make([]byte, 1024)
    var payload []byte = make([]byte, 1024 * 1024 * 2)
    var maskingKey []byte = make([]byte, 4)

    r := bufio.NewReader(c)

	for {
        finRsvOp, err := r.ReadByte()
        if err != nil {
            log.Printf("failed to read flags: %v\n", err)
            continue
        }
    
        isFin := false
        if (finRsvOp & mFin) == mFin {
            isFin = true
        }

        fType := frameType(finRsvOp & mOp) 

        if fType.String() == "unknown" {
            log.Printf("received unsupported frame type '%08b', ignoring frame\n", fType)
            continue
        }

        maskPayloadLen, err := r.ReadByte()
        if err != nil {
            log.Printf("failed to read mask and payload length: %v\n", err)
            continue
        }

        isMasked := false
        if (maskPayloadLen & mMask) == mMask {
            isMasked = true
        }

        payloadLen := uint64(maskPayloadLen & mPayloadLen)
        if payloadLen > 125 {
            if payloadLen == 126 {
                b, err := r.Peek(2)
                if err != nil {
                    log.Printf("failed to read the rest of payload length: %v\n", err)
                    continue
                }
                payloadLen = uint64( uint16(b[0]) << (16 - 8) | uint16(b[1]))
                if _, err := r.Discard(2); err != nil {
                    log.Printf("failed to discard 2 bytes: %v\n", err)
                    continue
                }
            } else if payloadLen == 127 {
                b, err := r.Peek(8)
                if err != nil {
                    log.Printf("failed to read the rest of payload length: %v\n", err)
                    continue
                }
                payloadLen = (
                    uint64(b[0]) << (64 - 8)  |
                    uint64(b[1]) << (64 - 16) |
                    uint64(b[2]) << (64 - 24) |
                    uint64(b[3]) << (64 - 32) |
                    uint64(b[4]) << (64 - 40) |
                    uint64(b[5]) << (64 - 48) |
                    uint64(b[6]) << (64 - 56) |
                    uint64(b[7]))
                if _, err := r.Discard(8); err != nil {
                    log.Printf("failed to discard 8 bytes: %v\n", err)
                    continue
                }
            }
        }

        if isMasked {
            if _, err = r.Read(maskingKey); err != nil {
                log.Printf("failed to read masking key: %v\n", err)
                continue
            }
        }

        log.Printf("client frame isFin=%t (%08b), fType=%08b (%s), isMasked=%t, payloadLen=%d\n", isFin, (finRsvOp & mFin), fType, fType, isMasked, payloadLen) 

        n, err := io.ReadFull(r, payload[0:payloadLen])
        if err != nil {
            log.Printf("failed to read payload: %v\n", err)
            continue
        }

        if n != int(payloadLen) {
            panic(fmt.Sprintf("have payload length of %d but only could only read %d byte(s)\n", payloadLen, n))
        }

        log.Printf("read %d byte(s) from the payload, payload length is %d byte(s)\n", n, payloadLen)

        if isMasked {
            for i := range payloadLen {
                payload[i] ^= maskingKey[i % 4]
            }
        }

        if payloadLen > 32 {
            log.Printf("payload '%s', message truncated orginal size is '%d'\n", string(payload[:32]), payloadLen)
        } else {
            log.Printf("payload '%s'\n", payload[:payloadLen])
        }
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

        log.Printf("new connection from %s\n", c.RemoteAddr())
		go handle(c)
	}
}
