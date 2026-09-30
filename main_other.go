//go:build !windows

package main

import "fmt"

func main() {
	fmt.Println("usb-indicator supports Windows only")
}
