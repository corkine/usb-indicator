//go:build !windows

package main

import "fmt"

func main() {
	fmt.Printf("usb-indicator %s supports Windows only\n", version)
}
