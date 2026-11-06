package main

import "testing"

func TestRejectRemovedCommandsAndMissingInputs(t *testing.T) {
	for _, arguments := range [][]string{{}, {"root"}, {"targets"}, {"sign"}, {"merge"}, {"make"}, {"make", "--out", "unused", "--sequence", "0"}, {"renew", "--out", "unused", "--sequence", "1"}, {"keygen", "--out", "same", "--public", "same"}} {
		if err := run(arguments); err == nil {
			t.Fatalf("accepted incomplete or removed command %v", arguments)
		}
	}
}
