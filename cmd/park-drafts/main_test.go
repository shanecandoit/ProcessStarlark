package main

import "testing"

func TestDevelopmentAddressStaysOnLoopback(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:8081", "[::1]:8081"} {
		if err := validateAddress(addr); err != nil {
			t.Error(addr, err)
		}
	}
	for _, addr := range []string{"0.0.0.0:8081", ":8081", "192.168.1.2:8081", "example.com:8081", "bad"} {
		if err := validateAddress(addr); err == nil {
			t.Error("accepted non-loopback address", addr)
		}
	}
}
