package main

import (
	"bufio"
	"fmt"
    "math"
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

func send(w net.Conn, t frameType, b []byte, payload []byte, payloadLen uint64) error {
    headerLen := 2

    if payloadLen > 125 {
        if payloadLen <= math.MaxUint16 {
            headerLen += 2
        } else {
            headerLen += 8
        }
    }

    if cap(b) < headerLen {
        panic("write buffer capacity is too short")
    }

    maxPayloadBytesPerFrame := uint64(cap(b) - headerLen)
    frame := 0
    isFin := false
    ptr := 0

    log.Printf("starting to write frames payloadLen=%d, capacity=%d, maxPayloadBytesPerFrame=%d\n", payloadLen, cap(b), maxPayloadBytesPerFrame)

    payloadBytesToWrite := payloadLen

    for payloadBytesToWrite > 0 {
        ptr = 0

        totalPayloadBytesThisFrame := uint64(math.Min(float64(payloadBytesToWrite), float64(maxPayloadBytesPerFrame)))

        // Are we on the last frame?
        if payloadBytesToWrite < maxPayloadBytesPerFrame {
            isFin = true
        }   

        if isFin {
            b[ptr] = mFin
        }

        // NOTE: Only on the first frame do we set the type
        if frame == 0 {
            b[ptr] |= byte(t)
        }
        ptr++

        // NOTE: For each of these, we're assuming 'no-mask' which
        // is normal for data being sent to the client from the server.
        if totalPayloadBytesThisFrame <= 125 {
            b[ptr] = byte(totalPayloadBytesThisFrame); ptr++
        } else if totalPayloadBytesThisFrame <= math.MaxUint16 {
            b[ptr] = byte(126); ptr++
            for j := 16 - 8; j >= 0; j -= 8 {
                b[ptr] = byte(totalPayloadBytesThisFrame >> j)
                ptr++
            }
        } else if totalPayloadBytesThisFrame <= math.MaxUint64 {
            b[ptr] = byte(127); ptr++
            for j := 64 - 8; j >= 0; j -= 8 {
                b[ptr] = byte(totalPayloadBytesThisFrame >> j)
                ptr++
            }
        }

        // NOTE: We're skipping the masking key as this func, currently, should
        // only be used for sending data to the client
        
        payloadBytesWritten := int(payloadLen - payloadBytesToWrite)

        copy(b[ptr:], payload[payloadBytesWritten:payloadBytesWritten + int(totalPayloadBytesThisFrame)])

        n, err := w.Write(b[:headerLen + int(totalPayloadBytesThisFrame)])
        if err != nil {
            return err
        }

        payloadBytesToWrite -= uint64(n - headerLen)

        log.Printf("sending frame #%d of %d byte(s), isFin=%t, finRsvOp=%08b\n", frame + 1, n, isFin, b[0])

        frame++
    }

    return nil
}

func handle(c net.Conn) {
	defer c.Close()

    var wBuffer []byte = make([]byte, 1024)
	var payload []byte = make([]byte, 1024*1024*2)
	var maskingKey []byte = make([]byte, 4)

	r := bufio.NewReader(c)

	for {
		finRsvOp, err := r.ReadByte()
		if err != nil {
			if err == io.EOF {
				break
			}
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
			if err == io.EOF {
				break
			}
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
					if err == io.EOF {
						break
					}
					log.Printf("failed to read the rest of payload length: %v\n", err)
					continue
				}
				payloadLen = uint64(uint16(b[0])<<(16-8) | uint16(b[1]))
				if _, err := r.Discard(2); err != nil {
					if err == io.EOF {
						break
					}
					log.Printf("failed to discard 2 bytes: %v\n", err)
					continue
				}
			} else if payloadLen == 127 {
				b, err := r.Peek(8)
				if err != nil {
					if err == io.EOF {
						break
					}
					log.Printf("failed to read the rest of payload length: %v\n", err)
					continue
				}
				payloadLen = (uint64(b[0])<<(64-8) |
					uint64(b[1])<<(64-16) |
					uint64(b[2])<<(64-24) |
					uint64(b[3])<<(64-32) |
					uint64(b[4])<<(64-40) |
					uint64(b[5])<<(64-48) |
					uint64(b[6])<<(64-56) |
					uint64(b[7]))
				if _, err := r.Discard(8); err != nil {
					if err == io.EOF {
						break
					}
					log.Printf("failed to discard 8 bytes: %v\n", err)
					continue
				}
			}
		}

		if isMasked {
			if _, err = r.Read(maskingKey); err != nil {
				if err == io.EOF {
					break
				}
				log.Printf("failed to read masking key: %v\n", err)
				continue
			}
		}

		log.Printf("client frame isFin=%t (%08b), fType=%08b (%s), isMasked=%t, payloadLen=%d\n", isFin, (finRsvOp & mFin), fType, fType, isMasked, payloadLen)

        n, err := io.ReadFull(r, payload[0:payloadLen])
        if err != nil {
            if err == io.EOF {
                break
            }
            log.Printf("failed to read payload: %v\n", err)
            continue
        }

        if n != int(payloadLen) {
            panic(fmt.Sprintf("have payload length of %d but only could only read %d byte(s)\n", payloadLen, n))
        }

        switch fType {
        case ping:
            if err := send(c, pong, wBuffer, payload, payloadLen); err != nil {
                log.Printf("failed to send pong: %v\n", err)
                continue
            }
        case connclose:
            // TODO: Clean this up, we handle a close here differently to how 
            // we handle close for any other case.
            // TODO: Closing here, in this way, actually triggers 2 closes
            // the one here, and the one from the 'defer'.
            if err := c.Close(); err != nil {
                log.Printf("failed to close connection to client\n", err)
            }
            return
        case text, binary:
            if isMasked {
                for i := 0; i < int(payloadLen); i++ {
                    payload[i] ^= maskingKey[i%4]
                }
            }

            // NOTE: For now, we just echo back the data
            if err := send(c, fType, wBuffer, payload, payloadLen); err != nil {
                log.Printf("failed to send echo: %v\n", err)
                continue
            }
        }
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
