package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"math"
	"net"
)

type state uint8

const (
	open = state(iota)
	closed
	closing
	peerClosing
)

func (s state) String() string {
    switch s {
    case open:
        return "open"
    case closed:
        return "close"
    case closing:
        return "closing"
    case peerClosing:
        return "peerClosing"
    }
    return ""
}

type conn struct {
	socket    net.Conn
	h         *header
    p         *payload
	w         *bufio.Writer
	r         *bufio.Reader
	state     state
    lastOp    *opCode
}

func newConn(socket net.Conn) *conn {
    var c conn = conn{}
    c.socket = socket
    c.h = &header{}
    c.r = bufio.NewReader(c.socket)
    c.w = bufio.NewWriter(c.socket)
    c.p = newPayload()
    c.state = open
    c.lastOp = nil

    return &c
}

func (c *conn) handle() error {
	for c.state == open {
        // Read the header
		if err := c.h.read(c.r); err != nil {
			if err == io.EOF {
				log.Printf("failed to read header, client disconnected\n")
				break
			}
			log.Printf("failed to read header: %v\n", err)
			break
		}

        // If they're sending a fragmented frame and the op code is not
        // a contuation, we must fail the connection
        if c.lastOp != nil && c.h.op != continuation {
            if err := c.sendClose(statusProtoErr, false); err != nil {
                return err
            }
            return nil
        }

        // Cannot have an RSV bit set, nor can the op-code be reserved
		if c.h.rsv != 0x00 || c.h.op.isReserved() {
			if err := c.sendClose(statusProtoErr, false); err != nil {
				return err
			}
			return nil
		}

        // The incoming length cannot be bigger than we have room for in the buffer
		if int(c.h.length) > c.p.capacity() {
			if err := c.sendClose(statusTooBig, false); err != nil {
				return err
			}
			return nil
		}

		log.Printf("client frame isFin=%t, rsv=%08b fType=%08b (%s), isMasked=%t, payloadLen=%d, header.size()=%d\n", c.h.isFin, c.h.rsv, c.h.op, c.h.op, c.h.isMasked, c.h.length, c.h.size())

		n, err := c.p.read(c.r, int(c.h.length))
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

        log.Printf("payload after read frames=%d, last=%v\n", len(c.p.frames), c.p.last)

		if n != int(c.h.length) {
			panic(fmt.Sprintf("have payload length of %d but only could only read %d byte(s)\n", c.h.length, n))
		}

        // If the last read frame is masked, unmask it
		if c.h.isMasked {
			for i := 0; i < int(c.h.length); i++ {
				c.p.last.data[i] ^= c.h.mask[i%4]
			}
		}

		if c.h.op.isControl() {
			if err := c.handleControlFrame(); err != nil {
				log.Printf("failed to handle control frame: %v\n", err)
				if err := c.sendClose(statusProtoErr, true); err != nil {
					return err
				}
				return nil
			}

            // If we were in the middle of handling a fragmented payload when 
            // the control frame came in, we need to pop the last frame from 
            // payload in order to continue reading correctly.
            if c.lastOp != nil {
                c.p.pop() 
            }

			continue
		}

        op := c.h.op

        // If 'fin' is false, we are reading a sequence of fragments
        if !c.h.isFin {
            if c.lastOp == nil {
                c.lastOp = &op
            }
            log.Printf("received non-fin frame, continuing with read\n")
            continue

        } else {
            if c.lastOp != nil {
                c.h.op = *c.lastOp
            }
            c.lastOp = nil
            log.Printf("fragmented read complete, payload=%v, op=%s\n", c.p.combine(), c.h.op)
        }

        // Echo back the data
        if err := c.send(false); err != nil {
            log.Printf("failed to send echo: %v\n", err)
            break
        }

        c.p.reset()
	}

    // TODO: We're not waiting for the peer to send their response back
    // after updating this to close the connection directly after we started
    // the handshake, all tests still closed cleanly.
    // TODO: Double check in the specification to make sure that this 
    // approach is considered 'clean' universally as this may just be
    // a quirk of the autobahn testsuite.
    // We drop into here when a close negotiation has started by either
    // us or by the remote client
    for c.state == closing || c.state == peerClosing { 
        //log.Printf("dropped into close loop, current state = %s\n", c.state)
		//if err := c.h.read(c.r); err != nil {
		//	if err == io.EOF {
		//		log.Printf("failed to read header, client disconnected\n")
		//		break
		//	}
		//	log.Printf("failed to read header: %v\n", err)
		//	break
		//}

        //log.Printf("rx'd op code %d (%s)\n", c.h.op, c.h.op)

        //// Right now all we care about is close op codes
        //if c.h.op == connclose {
        //    log.Printf("connclose rx'd closing, finally! c.state=%s\n", c.state)
        //    break
        //}
        break
    }

	return nil
}

func (c *conn) handleControlFrame() error {
    // Control frame MUST NOT be fragmented
    if !c.h.isFin {
        return c.sendClose(statusProtoErr, false)
    }

	switch c.h.op {
	case ping:
		c.h.op = pong
	case pong:
		c.h.op = ping
	case connclose:
		// If we're 'closing' and we've recevied a close frame, we know it's from the peer,
		// responding to our initiated close handshake.
		if c.state == closing {
			c.state = closed
			return nil
		}

		// If we're 'open' and we've recevied a close frame, it means the peer has initiated
		// the close handshake.
		if c.state == open {
			c.state = peerClosing
		}

		return c.sendClose(statusNormal, false)
	}

    // This will just back what ever is in the buffer which is most likely
    // what was sent in the original payload.
	return c.send(true)
}

func (c *conn) sendClose(status status, text bool) error {
	c.h.op = connclose

	// If the connection was open and we're now sending a close it means
	// we've started the close handshake, else the peer has started the close
	// handshake so this is the last frame we're sending.
	if c.state == open {
		c.state = closing
	} else if c.state == peerClosing {
		c.state = closed
	}

    f, err := c.p.reserve(2)
    if err != nil {
        return err
    }

	b := bytes.NewBuffer(f.data)
	b.Reset()

	for i := 16 - 8; i >= 0; i -= 8 {
		if err := b.WriteByte(byte(status >> i)); err != nil {
			return err
		}
	}

    // TODO: Disabling text for now
	//if text {
	//	if _, err := b.WriteString(status.String()); err != nil {
	//		return err
	//	}
	//}

	c.h.length = uint64(b.Len())

	if err := c.send(true); err != nil {
        return err
    }

    //c.state = closed
    return nil
}

// send will write the combined frames currently in payload or just the last frame
func (c *conn) send(last bool) error {
	// TODO: We're assuming here that we're always the server and thus we never mask
	c.h.isFin = false
	c.h.isMasked = false

	// A control frame's payload may not exceed 125 bytes
	if c.h.op.isControl() && c.h.length > 125 {
		return c.sendClose(statusProtoErr, true)
	}

	// Control frames must always be sent in 1 frame
	if c.h.op.isControl() {
		c.h.isFin = true
	}

	// If there's no payload, we still need to repsond with empty
	if c.h.length == 0 {
		c.h.isFin = true
		if err := c.h.write(c.w); err != nil {
			return err
		}

		if err := c.w.Flush(); err != nil {
			return err
		}

		return nil
	}

    if last && c.p.last == nil {
        return fmt.Errorf("last frame write was requested but last frame is nil")
    }

    payloadToSend := c.p.combine()
    if last {
        payloadToSend = c.p.last.data
    } 

    log.Printf("payload=%v, len=%d\n", payloadToSend, len(payloadToSend))

	frame := 0
	payloadBytesToWrite := uint64(len(payloadToSend))
	maxPayloadBytesPerFrame := uint64(c.w.Size()) - c.h.size()
	payloadByteOffset := 0

	log.Printf("starting to write frames payloadLen=%d, capacity=%d, maxPayloadBytesPerFrame=%d\n", payloadBytesToWrite, c.w.Size(), maxPayloadBytesPerFrame)

	for payloadBytesToWrite > 0 {
		totalPayloadBytesThisFrame := uint64(math.Min(float64(payloadBytesToWrite), float64(maxPayloadBytesPerFrame)))
		c.h.length = totalPayloadBytesThisFrame

		// If we're not on the first frame, we must set the 'continuation' op code
		if payloadByteOffset > 0 {
			c.h.op = continuation
		}

		// If we're on the last frame, set 'fin'
		if payloadBytesToWrite < maxPayloadBytesPerFrame {
			c.h.isFin = true
		}

		if err := c.h.write(c.w); err != nil {
			return err
		}

		n, err := c.w.Write(payloadToSend[payloadByteOffset : payloadByteOffset+int(totalPayloadBytesThisFrame)])
		if err != nil {
			return err
		}

		payloadBytesToWrite -= uint64(n)
		payloadByteOffset += n

		log.Printf("sending frame #%d of %d byte(s), header.length=%d, isFin=%t, finRsvOp=%08b (%s)\n", frame+1, n, c.h.length, c.h.isFin, c.h.op, c.h.op)

		if err := c.w.Flush(); err != nil {
			return err
		}

		frame++
	}

	return nil
}
