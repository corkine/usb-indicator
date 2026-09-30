//go:build windows

package main

import (
	"errors"
	"regexp"
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

var (
	mouseInterfaceGUID = guid{
		Data1: 0x378de44c, Data2: 0x56ef, Data3: 0x11d1,
		Data4: [8]byte{0xbc, 0x8c, 0x00, 0xa0, 0xc9, 0x14, 0x05, 0xdd},
	}
	hidInterfaceGUID = guid{
		Data1: 0x4d1e55b2, Data2: 0xf16f, Data3: 0x11cf,
		Data4: [8]byte{0x88, 0xcb, 0x00, 0x11, 0x11, 0x00, 0x00, 0x30},
	}
	bluetoothAddressPattern = regexp.MustCompile(`(?i)_([0-9A-F]{12})(?:&|#)`)
)

const m590BluetoothHardwareID = "VID&02046D_PID&B01B"

var unifyingReceiverProductIDs = []string{"PID_C52B", "PID_C532"}

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

func enumerateDeviceInterfaces(class *guid) ([]string, error) {
	handle, _, callErr := procSetupDiGetClassDevsW.Call(
		uintptr(unsafe.Pointer(class)), 0, 0,
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
			handle, 0, uintptr(unsafe.Pointer(class)), uintptr(index),
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

func enumerateKeyboards() ([]string, error) {
	return enumerateDeviceInterfaces(&keyboardInterfaceGUID)
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

func mouseChoices() ([]mouseChoice, error) {
	var choices []mouseChoice
	seen := make(map[string]bool)

	mice, err := enumerateDeviceInterfaces(&mouseInterfaceGUID)
	if err != nil {
		return nil, err
	}
	for _, path := range mice {
		upper := strings.ToUpper(path)
		if !strings.Contains(upper, m590BluetoothHardwareID) {
			continue
		}
		match := bluetoothAddressPattern.FindStringSubmatch(upper)
		if len(match) != 2 || seen["bluetooth:"+match[1]] {
			continue
		}
		seen["bluetooth:"+match[1]] = true
		choices = append(choices, mouseChoice{
			Mode: "bluetooth", ID: match[1], Slot: 0,
			Label: "M585/M590（蓝牙 " + match[1] + "）",
		})
	}

	hids, err := enumerateDeviceInterfaces(&hidInterfaceGUID)
	if err != nil {
		return nil, err
	}
	for _, path := range hids {
		upper := strings.ToUpper(path)
		if strings.Contains(upper, m590BluetoothHardwareID) {
			address := bluetoothAddressPattern.FindStringSubmatch(upper)
			if len(address) == 2 && !seen["bluetooth:"+address[1]] {
				seen["bluetooth:"+address[1]] = true
				choices = append(choices, mouseChoice{
					Mode: "bluetooth", ID: address[1],
					Label: "M585/M590（蓝牙 " + address[1] + "）",
				})
			}
		}
		match := stableMatch(upper)
		if !strings.Contains(upper, "VID_046D") || !containsAny(upper, unifyingReceiverProductIDs) ||
			seen["unifying:"+match] {
			continue
		}
		seen["unifying:"+match] = true
		choices = append(choices, mouseChoice{
			Mode: "unifying", ID: match, Slot: 0,
			Label: "Logitech Unifying 接收器（自动检测在线设备槽）",
		})
	}
	return choices, nil
}

func containsAny(value string, candidates []string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}

func bluetoothMousePresent(address string) (bool, error) {
	paths, err := enumerateDeviceInterfaces(&mouseInterfaceGUID)
	if err != nil {
		return false, err
	}
	address = strings.ToUpper(strings.TrimSpace(address))
	for _, path := range paths {
		if strings.Contains(strings.ToUpper(path), address) {
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
		choices = append(choices, keyboardChoice{MatchRule: match, Path: path})
	}
	return choices, nil
}
