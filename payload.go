package main

import (
    "bufio"
    "io"
    "fmt"
)

const payloadSize int = 1024 * 1024 * 2

type frame struct {
    start int
    end   int
    data  []byte
}

func (f *frame) length() int {
    return f.end - f.start
}

type payload struct {
    b      []byte
    last   *frame
    frames []frame
}

func newPayload() *payload {
    return newPayloadSize(payloadSize)
}

func newPayloadSize(size int) *payload {
    return &payload { make([]byte, size), nil, make([]frame, 0) }
}

// read reads in a new frame to the payload
func (p *payload) read(r *bufio.Reader, count int) (int, error) {
    f, err := p.reserve(count)
    if err != nil {
        return 0, err
    }

    n, err := io.ReadFull(r, f.data)
    if err != nil {
        return n, err
    }
    
    return n, nil
}

// capacity returns how much capacity the buffer has
func (p *payload) capacity() int {
    if p.last != nil {
        return cap(p.b) - p.last.end
    }
    return cap(p.b)
}

// reset clears all frames
func (p *payload) reset() {
    p.frames = []frame{}
    p.last = nil
}

// reserve will allocate a new frame with data size of 'size'
func (p *payload) reserve(size int) (*frame, error) {
    if size > p.capacity() {
        return nil, fmt.Errorf("read count %d cannot be greater than current buffer capacity of %d byte(s)", size, p.capacity())
    }

    start := 0
    if p.last != nil {
        start = p.last.end 
    }
    end := start+size

    var f frame = frame{ start, end, p.b[start:end] } 
    p.frames = append(p.frames, f)
    p.last = &f

    return p.last, nil
}

// pop removes the last read frame
func (p *payload) pop() {
    l := len(p.frames)
    if l < 2 {
        p.reset()
        return
    }
    
    p.frames = p.frames[:len(p.frames) - 1]
    p.last = &p.frames[len(p.frames) - 1]
}

// combine returns a slice containing all frames
func (p *payload) combine() []byte {
    if len(p.frames) == 0 {
        return []byte{}
    }
    start, end := -1, -1
    for i := 0; i < len(p.frames); i++ {
        if start == -1 {
            start = p.frames[i].start
        }
        end = p.frames[i].end
    }
    if start == -1 || end == -1 { panic("payload has frames but couldn't find a valid start and end") }
    return p.b[start:end]
}

// length returns the combined length of all frames
func (p *payload) length() int {
    length := 0
    for i := 0; i < len(p.frames); i++ {
        length += p.frames[i].length()
    }
    return length
}
