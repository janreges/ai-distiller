package store

import "testing"

func TestOpen(t *testing.T) {
	if Open("memory") == nil {
		t.Fatal("nil store")
	}
}
