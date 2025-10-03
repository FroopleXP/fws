package main

import (
    "testing"
    "bufio"
    "bytes"
)

func TestPayloadRead(t *testing.T) {
    var data []byte = []byte{ 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16 }
    var r *bufio.Reader = bufio.NewReader(bytes.NewBuffer(data))

    payload := newPayload(r)
    
    // Read 1
    if _, err := payload.read(5); err != nil {
        t.Errorf("failed to read 1st payload: %v", err)
    }
    t.Logf("frames 1: %v", payload.last)

    if len(payload.frames) != 1 {
        t.Errorf("expected frames length to be 1, got %d", len(payload.frames))
    }

    if payload.last == nil {
        t.Errorf("successful read but payload.last == nil")
    }

    // Read 2
    if _, err := payload.read(5); err != nil {
        t.Errorf("failed to read 2nd payload: %v", err)
    }
    t.Logf("frames 2: %v", payload.last)

    // Combined
    t.Logf("combined %v\n", payload.combine())
    
    // Read 3
    if n, err := payload.read(10); err != nil {
        t.Errorf("err=%v, n=%d\n", err, n)
    }
    t.Logf("frames 3: %v", payload.last)

    payload.pop()


    if _, err := payload.read(5); err != nil {
        t.Errorf("failed to read 3rd payload: %v", err)
    }

    t.Logf("frames 4: %v", payload.last)
}
