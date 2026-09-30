//go:build windows

package main

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

type hidCaps struct {
	Usage                     uint16
	UsagePage                 uint16
	InputReportByteLength     uint16
	OutputReportByteLength    uint16
	FeatureReportByteLength   uint16
	Reserved                  [17]uint16
	NumberLinkCollectionNodes uint16
	NumberInputButtonCaps     uint16
	NumberInputValueCaps      uint16
	NumberInputDataIndices    uint16
	NumberOutputButtonCaps    uint16
	NumberOutputValueCaps     uint16
	NumberOutputDataIndices   uint16
	NumberFeatureButtonCaps   uint16
	NumberFeatureValueCaps    uint16
	NumberFeatureDataIndices  uint16
}

const (
	fileFlagOverlapped = 0x40000000
	waitObject0        = 0
	errorIOPending     = syscall.Errno(997)
)

var (
	hidDLL                    = syscall.NewLazyDLL("hid.dll")
	procHidDGetPreparsedData  = hidDLL.NewProc("HidD_GetPreparsedData")
	procHidDFreePreparsedData = hidDLL.NewProc("HidD_FreePreparsedData")
	procHidPGetCaps           = hidDLL.NewProc("HidP_GetCaps")
	procHidDSetOutputReport   = hidDLL.NewProc("HidD_SetOutputReport")
	procCreateEventW          = kernel32.NewProc("CreateEventW")
	procReadFile              = kernel32.NewProc("ReadFile")
	procWaitForSingleObject   = kernel32.NewProc("WaitForSingleObject")
	procGetOverlappedResult   = kernel32.NewProc("GetOverlappedResult")
	procCancelIoEx            = kernel32.NewProc("CancelIoEx")

	hidppCacheMu    sync.Mutex
	hidppCacheAt    time.Time
	hidppCacheMatch string
	hidppCacheSlot  int
	hidppCacheValue bool
	hidppCacheErr   error
	hidppDebug      bool
)

func unifyingMousePresent(match string, slot, pollIntervalMS int) (bool, error) {
	hidppCacheMu.Lock()
	defer hidppCacheMu.Unlock()
	if match == hidppCacheMatch && slot == hidppCacheSlot && time.Since(hidppCacheAt) < unifyingCacheDuration(pollIntervalMS) {
		return hidppCacheValue, hidppCacheErr
	}

	connectedSlot, deviceInfo, err := queryUnifyingConnection(match)
	if errors.Is(err, errHIDPPTimeout) {
		err = nil
	}
	present := err == nil && unifyingMouseLinkEstablished(deviceInfo) &&
		(slot == 0 || slot == connectedSlot)
	hidppCacheAt, hidppCacheMatch, hidppCacheSlot = time.Now(), match, slot
	hidppCacheValue, hidppCacheErr = present, err
	return present, err
}

func unifyingCacheDuration(pollIntervalMS int) time.Duration {
	switch pollIntervalMS {
	case 100:
		return 300 * time.Millisecond
	case 500:
		return time.Second
	default:
		return 500 * time.Millisecond
	}
}

var errHIDPPTimeout = errors.New("HID++ response timeout")

func unifyingMouseLinkEstablished(deviceInfo byte) bool {
	const (
		deviceTypeMask      = 0x0f
		mouseDeviceType     = 0x02
		linkUnavailableMask = 0x40
	)
	return deviceInfo&deviceTypeMask == mouseDeviceType && deviceInfo&linkUnavailableMask == 0
}

func exchangeHIDPP(path string, request []byte, timeoutMS uintptr) ([]byte, error) {
	handle, err := syscall.CreateFile(
		syscall.StringToUTF16Ptr(path), syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING,
		fileFlagOverlapped, 0,
	)
	if err != nil {
		return nil, err
	}
	defer syscall.CloseHandle(handle)

	var preparsed uintptr
	ok, _, callErr := procHidDGetPreparsedData.Call(uintptr(handle), uintptr(unsafe.Pointer(&preparsed)))
	if ok == 0 {
		return nil, callErr
	}
	defer procHidDFreePreparsedData.Call(preparsed)
	var caps hidCaps
	status, _, callErr := procHidPGetCaps.Call(preparsed, uintptr(unsafe.Pointer(&caps)))
	if uint32(status) != 0x00110000 || int(caps.InputReportByteLength) < len(request) || int(caps.OutputReportByteLength) < len(request) {
		if callErr != syscall.Errno(0) {
			return nil, callErr
		}
		return nil, errors.New("HID collection does not carry this HID++ report")
	}
	if hidppDebug {
		fmt.Printf("path=%s\n  usage=%04X:%04X input=%d output=%d feature=%d request=% X\n", path, caps.UsagePage, caps.Usage, caps.InputReportByteLength, caps.OutputReportByteLength, caps.FeatureReportByteLength, request)
	}

	event, _, callErr := procCreateEventW.Call(0, 1, 0, 0)
	if event == 0 {
		return nil, callErr
	}
	defer procCloseHandle.Call(event)
	input := make([]byte, caps.InputReportByteLength)
	var read uint32
	overlap := syscall.Overlapped{HEvent: syscall.Handle(event)}
	ok, _, readErr := procReadFile.Call(
		uintptr(handle), uintptr(unsafe.Pointer(&input[0])), uintptr(len(input)),
		uintptr(unsafe.Pointer(&read)), uintptr(unsafe.Pointer(&overlap)),
	)
	if ok == 0 && readErr != errorIOPending {
		return nil, readErr
	}

	output := make([]byte, caps.OutputReportByteLength)
	copy(output, request)
	ok, _, writeErr := procHidDSetOutputReport.Call(
		uintptr(handle), uintptr(unsafe.Pointer(&output[0])), uintptr(len(output)),
	)
	if ok == 0 {
		procCancelIoEx.Call(uintptr(handle), uintptr(unsafe.Pointer(&overlap)))
		if hidppDebug {
			fmt.Printf("  write error: %v\n", writeErr)
		}
		return nil, writeErr
	}

	wait, _, _ := procWaitForSingleObject.Call(event, timeoutMS)
	if wait != waitObject0 {
		procCancelIoEx.Call(uintptr(handle), uintptr(unsafe.Pointer(&overlap)))
		if hidppDebug {
			fmt.Println("  response: timeout")
		}
		return nil, errHIDPPTimeout
	}
	ok, _, readErr = procGetOverlappedResult.Call(
		uintptr(handle), uintptr(unsafe.Pointer(&overlap)), uintptr(unsafe.Pointer(&read)), 0,
	)
	if ok == 0 || read < 7 {
		return nil, readErr
	}
	if hidppDebug {
		fmt.Printf("  response (%d): % X\n", read, input[:read])
	}
	return input[:read], nil
}

func queryUnifyingConnection(match string) (slot int, deviceInfo byte, err error) {
	paths, err := enumerateDeviceInterfaces(&hidInterfaceGUID)
	if err != nil {
		return 0, 0, err
	}
	var lastErr error = errors.New("no usable Logitech HID++ interface")
	request := []byte{0x10, 0xff, 0x80, 0x02, 0x02, 0x00, 0x00}
	for _, path := range paths {
		upperPath := strings.ToUpper(path)
		if !strings.Contains(upperPath, strings.ToUpper(match)) ||
			!strings.Contains(upperPath, "&MI_02&COL01#") {
			continue
		}
		response, exchangeErr := exchangeHIDPP(path, request, 75)
		if exchangeErr != nil {
			lastErr = exchangeErr
			continue
		}
		if len(response) >= 5 && response[0] == 0x10 && response[2] == 0x41 {
			return int(response[1]), response[4], nil
		}
		lastErr = fmt.Errorf("unexpected connection response: % X", response)
	}
	return 0, 0, lastErr
}
