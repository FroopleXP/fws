package main

import (
	"bufio"
	"math"
)

// Consts prefixed with 'm' declare a mask
const (
	mFin, mMask uint8 = 0x80, 0x80
	mRsv        uint8 = 0x70
	mOp         uint8 = 0x0f
	mPayloadLen uint8 = 0x7f
)

type header struct {
	isFin    bool
	rsv      byte
	op       opCode
	length   uint64
	isMasked bool
	mask     []byte
}

// size returns the size, in bytes, that header will require
func (h *header) size() uint64 {
	var size uint64 = 2

	if h.length > 125 {
		if h.length <= math.MaxUint16 {
			size += 2
		} else if h.length > math.MaxUint16 && h.length <= math.MaxUint64 {
			size += 8
		}
	}

	if h.isMasked {
		size += 4
	}

	return size
}

func (h *header) write(w *bufio.Writer) error {
	var finResOp byte = 0x00
	if h.isFin {
		finResOp |= byte(mFin)
	}
	finResOp |= byte(h.op)

	if err := w.WriteByte(finResOp); err != nil {
		return err
	}

	var maskPayloadLen byte = 0x00
	if h.isMasked {
		maskPayloadLen |= byte(mMask)
	}

	var isExtendedPayloadLen bool = true

	if h.length <= 125 {
		maskPayloadLen |= byte(h.length)
		isExtendedPayloadLen = false
	} else if h.length <= math.MaxUint16 {
		maskPayloadLen |= byte(126)
	} else if h.length <= math.MaxUint64 {
		maskPayloadLen |= byte(127)
	}

	if err := w.WriteByte(maskPayloadLen); err != nil {
		return err
	}

	if isExtendedPayloadLen {
		if h.length <= math.MaxUint16 {
			for i := 16 - 8; i >= 0; i -= 8 {
				if err := w.WriteByte(byte(h.length >> i)); err != nil {
					return err
				}
			}
		} else if h.length <= math.MaxUint64 {
			for i := 64 - 8; i >= 0; i -= 8 {
				if err := w.WriteByte(byte(h.length >> i)); err != nil {
					return err
				}
			}
		}
	}

	if h.isMasked && h.mask != nil && len(h.mask) == 4 {
		if _, err := w.Write(h.mask); err != nil {
			return err
		}
	}

	return nil
}

func (h *header) read(r *bufio.Reader) error {
	b, err := r.Peek(2)
	if err != nil {
		return err
	}

	finRsvOp := b[0]
	if err != nil {
		return err
	}

	h.isFin = false
	if (finRsvOp & mFin) == mFin {
		h.isFin = true
	}

	h.rsv = (finRsvOp & mRsv) >> 4

	h.op = opCode(finRsvOp & mOp)

	maskPayloadLen := b[1]
	if err != nil {
		return err
	}

	if _, err := r.Discard(2); err != nil {
		return err
	}

	h.isMasked = false
	if (maskPayloadLen & mMask) == mMask {
		h.isMasked = true
	}

	h.length = uint64(maskPayloadLen & mPayloadLen)
	if h.length > 125 {
		if h.length == 126 {
			b, err = r.Peek(2)
			if err != nil {
				return err
			}
			h.length = uint64(uint16(b[0])<<(16-8) | uint16(b[1]))
			if _, err := r.Discard(2); err != nil {
				return err
			}
		} else if h.length == 127 {
			b, err = r.Peek(8)
			if err != nil {
				return err
			}
			h.length = (uint64(b[0])<<(64-8) |
				uint64(b[1])<<(64-16) |
				uint64(b[2])<<(64-24) |
				uint64(b[3])<<(64-32) |
				uint64(b[4])<<(64-40) |
				uint64(b[5])<<(64-48) |
				uint64(b[6])<<(64-56) |
				uint64(b[7]))
			if _, err := r.Discard(8); err != nil {
				return err
			}
		}
	}

	if h.isMasked {
		if h.mask == nil {
			h.mask = make([]byte, 4)
		}

		b, err = r.Peek(4)
		if err != nil {
			return err
		}

		copy(h.mask, b[:])

		if _, err := r.Discard(4); err != nil {
			return err
		}
	}

	return nil
}
