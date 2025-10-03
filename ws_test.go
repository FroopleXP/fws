package main

import (
	"testing"
)

func isEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
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
