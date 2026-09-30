//go:build windows

package main

import (
	"testing"
	"time"
)

func TestUnifyingMouseLinkEstablished(t *testing.T) {
	tests := []struct {
		name       string
		deviceInfo byte
		want       bool
	}{
		{name: "mouse online", deviceInfo: 0x32, want: true},
		{name: "mouse paired but offline", deviceInfo: 0x72, want: false},
		{name: "keyboard online", deviceInfo: 0x31, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := unifyingMouseLinkEstablished(test.deviceInfo); got != test.want {
				t.Fatalf("unifyingMouseLinkEstablished(0x%02X) = %t, want %t", test.deviceInfo, got, test.want)
			}
		})
	}
}

func TestUnifyingCacheDuration(t *testing.T) {
	tests := []struct {
		pollMS int
		want   time.Duration
	}{
		{pollMS: 100, want: 300 * time.Millisecond},
		{pollMS: 200, want: 500 * time.Millisecond},
		{pollMS: 500, want: time.Second},
	}
	for _, test := range tests {
		if got := unifyingCacheDuration(test.pollMS); got != test.want {
			t.Fatalf("unifyingCacheDuration(%d) = %s, want %s", test.pollMS, got, test.want)
		}
	}
}
