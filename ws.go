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

func send(w *bufio.Writer, h *header, b []byte) error {
    // TODO: We're assuming here that we're always the server and thus we never mask
    h.isFin = false
    h.isMasked = false

    frame := 0
    payloadBytesToWrite := h.length
    maxPayloadBytesPerFrame := uint64(w.Size()) - h.size()
    payloadByteOffset := 0

    log.Printf("starting to write frames payloadLen=%d, capacity=%d, maxPayloadBytesPerFrame=%d\n", h.length, w.Size(), maxPayloadBytesPerFrame)

    for payloadBytesToWrite > 0 {
        totalPayloadBytesThisFrame := uint64(math.Min(float64(payloadBytesToWrite), float64(maxPayloadBytesPerFrame)))
        h.length = totalPayloadBytesThisFrame
        
        if payloadBytesToWrite < maxPayloadBytesPerFrame {
            h.isFin = true
        }   

        if err := h.write(w); err != nil {
            return err
        }

        n, err := w.Write(b[payloadByteOffset:payloadByteOffset + int(totalPayloadBytesThisFrame)])
        if err != nil {
            return err
        }

        payloadBytesToWrite -= uint64(n)
        payloadByteOffset += n

        log.Printf("sending frame #%d of %d byte(s), header.length=%d, isFin=%t, finRsvOp=%08b\n", frame + 1, n, h.length, h.isFin, h.op)

        if err := w.Flush(); err != nil {
            return err
        }

        frame++
    }

    return nil
}

func handle(c net.Conn) {
	defer c.Close()

    var h *header = &header{}

	var buffer []byte = make([]byte, 1024*1024*2)

	r := bufio.NewReader(c)
	w := bufio.NewWriter(c)

	for {
        if err := h.read(r); err != nil {
            if err == io.EOF {
                log.Printf("failed to read header, client disconnected\n")
                break
            }
            log.Printf("failed to read header: %v\n", err)
            break
        }

        // TOOD: Send actual close when this happens
        if h.length > uint64(cap(buffer)) {
            panic(fmt.Sprintf("payload too big for curent buffer capacity, length=%d, capacity=%s\n", h.length, cap(buffer)))
        }

		log.Printf("client frame isFin=%t, fType=%08b (%s), isMasked=%t, payloadLen=%d\n", h.isFin, h.op, h.op, h.isMasked, h.length)

        n, err := io.ReadFull(r, buffer[0:h.length])
        if err != nil {
            if err == io.EOF {
                break
            }
            log.Printf("failed to read payload: %v\n", err)
            continue
        }

        if n != int(h.length) {
            panic(fmt.Sprintf("have payload length of %d but only could only read %d byte(s)\n", h.length, n))
        }

        switch h.op {
        case ping:
            //h.op = pong
            //if err := send(w, h, buffer); err != nil {
            //    log.Printf("failed to send pong: %v\n", err)
            //    continue
            //}
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
            if h.isMasked {
                for i := 0; i < int(h.length); i++ {
                    buffer[i] ^= h.mask[i%4]
                }
            }
            if err := send(w, h, buffer); err != nil {
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
