package main

import (
	"testing"
    "bytes"
)

func isEqual(a, b []byte) bool {
    if len(a) != len(b) { return false }
    for i := range a {
        if a[i] != b[i] { return false }
    }
    return true
}

func TestAcceptKeyGeneration(t *testing.T) {
	var key string = "dGhlIHNhbXBsZSBub25jZQ=="
	var expected string = "s3pPLMBiTxaQ9kYGzzhZRbK+xOo="
	test, err := generateAcceptKey(key)
	if err != nil {
		t.Errorf("failed to generate accept key: %v", err)
	}

	if test != expected {
		t.Errorf("invalid key generated, expected %s - got %s", expected, test)
	}
}

func TestNReader(t *testing.T) {
    data := []byte{ 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09 }
    reader := bytes.NewReader(data)

    // NOTE: Building nReader like this is bad, this is just for the unit test
    //nReader := &nReader{ init: true, buffer: make([]byte, 2), reader: reader }
    nReader := &nReader{ init: true, buffer: make([]byte, 3), reader: reader }
    if err := nReader.fill(0); err != nil {
        t.Errorf("failed to do initial fill of buffer: %v\n", err)
    }

    var cases [][]byte = [][]byte{
        []byte{ data[0] }, 
        []byte{ data[1] },
        []byte{ data[2], data[3] },
        []byte{ data[4] },
        []byte{ data[5], data[6] },
        []byte{ data[7], data[8], data[9] },
    }

    for _, c := range cases {
        r, err := nReader.read(len(c))     
        if err != nil {
            t.Error(err)
        } 
        if !isEqual(r, c) {
            t.Errorf("unexpected got %v, expected %v", c, r)
        }
    }
}
