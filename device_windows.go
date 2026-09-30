//go:build windows

package main

import (
	"errors"
	"strings"
	"syscall"
	"unsafe"
)

type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

var keyboardInterfaceGUID = guid{
	Data1: 0x884b96c3,
	Data2: 0x56ef,
	Data3: 0x11d1,
	Data4: [8]byte{0xbc, 0x8c, 0x00, 0xa0, 0xc9, 0x14, 0x05, 0xdd},
}

type deviceInterfaceData struct {
	Size     uint32
	Class    guid
	Flags    uint32
	Reserved uintptr
}

const (
	digcfPresent         = 0x00000002
	digcfDeviceInterface = 0x00000010
	errorNoMoreItems     = syscall.Errno(259)
)

var (
	setupapi                       = syscall.NewLazyDLL("setupapi.dll")
	procSetupDiGetClassDevsW       = setupapi.NewProc("SetupDiGetClassDevsW")
	procSetupDiEnumDeviceInterface = setupapi.NewProc("SetupDiEnumDeviceInterfaces")
	procSetupDiGetInterfaceDetailW = setupapi.NewProc("SetupDiGetDeviceInterfaceDetailW")
	procSetupDiDestroyDeviceInfo   = setupapi.NewProc("SetupDiDestroyDeviceInfoList")
)

func enumerateKeyboards() ([]string, error) {
	handle, _, callErr := procSetupDiGetClassDevsW.Call(
		uintptr(unsafe.Pointer(&keyboardInterfaceGUID)), 0, 0,
		digcfPresent|digcfDeviceInterface,
	)
	if handle == ^uintptr(0) {
		return nil, callErr
	}
	defer procSetupDiDestroyDeviceInfo.Call(handle)

	var paths []string
	for index := uint32(0); ; index++ {
		data := deviceInterfaceData{Size: uint32(unsafe.Sizeof(deviceInterfaceData{}))}
		ok, _, enumErr := procSetupDiEnumDeviceInterface.Call(
			handle, 0, uintptr(unsafe.Pointer(&keyboardInterfaceGUID)), uintptr(index),
			uintptr(unsafe.Pointer(&data)),
		)
		if ok == 0 {
			if errors.Is(enumErr, errorNoMoreItems) {
				break
			}
			return nil, enumErr
		}

		var required uint32
		procSetupDiGetInterfaceDetailW.Call(
			handle, uintptr(unsafe.Pointer(&data)), 0, 0,
			uintptr(unsafe.Pointer(&required)), 0,
		)
		if required < 8 {
			continue
		}
		buffer := make([]byte, required)
		if unsafe.Sizeof(uintptr(0)) == 8 {
			*(*uint32)(unsafe.Pointer(&buffer[0])) = 8
		} else {
			*(*uint32)(unsafe.Pointer(&buffer[0])) = 6
		}
		ok, _, detailErr := procSetupDiGetInterfaceDetailW.Call(
			handle, uintptr(unsafe.Pointer(&data)), uintptr(unsafe.Pointer(&buffer[0])),
			uintptr(required), uintptr(unsafe.Pointer(&required)), 0,
		)
		if ok == 0 {
			return nil, detailErr
		}
		path := syscall.UTF16ToString((*[1 << 20]uint16)(unsafe.Pointer(&buffer[4]))[:])
		paths = append(paths, path)
	}
	return paths, nil
}

func keyboardPresent(match string) (bool, error) {
	paths, err := enumerateKeyboards()
	if err != nil {
		return false, err
	}
	match = strings.ToUpper(match)
	for _, path := range paths {
		if strings.Contains(strings.ToUpper(path), match) {
			return true, nil
		}
	}
	return false, nil
}

func keyboardChoices() ([]keyboardChoice, error) {
	paths, err := enumerateKeyboards()
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	choices := make([]keyboardChoice, 0, len(paths))
	for _, path := range paths {
		match := stableMatch(path)
		if seen[match] {
			continue
		}
		seen[match] = true
		choices = append(choices, keyboardChoice{Match: match, Path: path})
	}
	return choices, nil
}
