package main

import "testing"

func TestParseFlags(t *testing.T) {
	if parseFlags().DSN == "" {
		t.Fatal("empty DSN")
	}
}
