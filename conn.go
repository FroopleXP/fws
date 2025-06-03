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

type conn struct {
	socket net.Conn
	buffer []byte
	h      *header
	w      *bufio.Writer
	r      *bufio.Reader

	state state
}

func (c *conn) handle() error {
	if c.socket == nil {
		return fmt.Errorf("socket is not defined")
	}

	if c.buffer == nil {
		c.buffer = make([]byte, 1024*1024*2)
	}

	if c.h == nil {
		c.h = &header{}
	}

	if c.w == nil {
		c.w = bufio.NewWriter(c.socket)
	}

	if c.r == nil {
		c.r = bufio.NewReader(c.socket)
	}

	c.state = open

	for c.state != closed {
		if err := c.h.read(c.r); err != nil {
			if err == io.EOF {
				log.Printf("failed to read header, client disconnected\n")
				break
			}
			log.Printf("failed to read header: %v\n", err)
			break
		}

		// Can't be having those reserve bits set or use a reserved non-control op-code, who the fuck do you think you are?
		if c.h.rsv != 0x00 || c.h.op.isReserved() {
			if err := c.sendClose(statusProtoErr, false); err != nil {
				return err
			}
			return nil
		}

		if c.h.length > uint64(cap(c.buffer)) {
			if err := c.sendClose(statusTooBig, false); err != nil {
				return err
			}
			return nil
		}

		log.Printf("client frame isFin=%t, rsv=%08b fType=%08b (%s), isMasked=%t, payloadLen=%d, header.size()=%d\n", c.h.isFin, c.h.rsv, c.h.op, c.h.op, c.h.isMasked, c.h.length, c.h.size())

		n, err := io.ReadFull(c.r, c.buffer[:c.h.length])
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		if n != int(c.h.length) {
			panic(fmt.Sprintf("have payload length of %d but only could only read %d byte(s)\n", c.h.length, n))
		}

		if c.h.isMasked {
			for i := 0; i < int(c.h.length); i++ {
				c.buffer[i] ^= c.h.mask[i%4]
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
			continue
		}

		switch c.h.op {
		case text, binary:
			if err := c.send(); err != nil {
				log.Printf("failed to send echo: %v\n", err)
				break
			}
		}
	}

	return nil
}

func (c *conn) handleControlFrame() error {
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

	return c.send()
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

	b := bytes.NewBuffer(c.buffer)
	b.Reset()

	for i := 16 - 8; i >= 0; i -= 8 {
		if err := b.WriteByte(byte(status >> i)); err != nil {
			return err
		}
	}

	if text {
		if _, err := b.WriteString(status.String()); err != nil {
			return err
		}
	}

	c.h.length = uint64(b.Len())

	return c.send()
}

// send whatever the current contents of 'buffer' is, fragmenting the data when necessary
func (c *conn) send() error {
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

	frame := 0
	payloadBytesToWrite := c.h.length
	maxPayloadBytesPerFrame := uint64(c.w.Size()) - c.h.size()
	payloadByteOffset := 0

	log.Printf("starting to write frames payloadLen=%d, capacity=%d, maxPayloadBytesPerFrame=%d\n", c.h.length, c.w.Size(), maxPayloadBytesPerFrame)

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

		n, err := c.w.Write(c.buffer[payloadByteOffset : payloadByteOffset+int(totalPayloadBytesThisFrame)])
		if err != nil {
			return err
		}

		payloadBytesToWrite -= uint64(n)
		payloadByteOffset += n

		log.Printf("sending frame #%d of %d byte(s), header.length=%d, isFin=%t, finRsvOp=%08b\n", frame+1, n, c.h.length, c.h.isFin, c.h.op)

		if err := c.w.Flush(); err != nil {
			return err
		}

		frame++
	}

	return nil
}
