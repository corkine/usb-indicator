//go:build windows

package main

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"
)

var (
	indicatorKeyboardMatch string
	indicatorPosition      = defaultPosition
	indicatorSize          = int32(defaultSize)
	indicatorOffsetX       int32
	indicatorOffsetY       int32
	indicatorMouseMode     string
	indicatorMouseID       string
	indicatorMouseSlot     int
	indicatorPollInterval  = uintptr(defaultPollMS)
	indicatorKeyboardOn    bool
	indicatorMouseOn       bool
	indicatorWindow        uintptr
	deviceNotification     uintptr
)

func acquireSingleInstance() (release func(), acquired bool, err error) {
	// This also recognizes an instance built before the mutex guard existed.
	existing, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(utf16("USBIndicatorWindow"))), 0)
	if existing != 0 {
		return func() {}, false, nil
	}

	handle, _, callErr := procCreateMutexW.Call(
		0, 0, uintptr(unsafe.Pointer(utf16(`Local\USBKeyboardIndicator.SingleInstance`))),
	)
	if handle == 0 {
		return nil, false, fmt.Errorf("create single-instance mutex: %w", callErr)
	}
	if errors.Is(callErr, syscall.Errno(183)) {
		procCloseHandle.Call(handle)
		return func() {}, false, nil
	}
	return func() { procCloseHandle.Call(handle) }, true, nil
}

func runIndicator(cfg config) error {
	applyIndicatorConfig(cfg)
	if err := registerClass("USBIndicatorWindow", syscall.NewCallback(indicatorWndProc), 0); err != nil {
		return err
	}
	hwnd, err := createWindow(
		wsExTopmost|wsExToolWindow|wsExLayered|wsExTransparent|wsExNoActivate,
		"USBIndicatorWindow", "Keyboard and mouse indicator", wsPopup, 0, 0, indicatorWidth(), indicatorSize, 0, 0,
	)
	if err != nil {
		return err
	}
	indicatorWindow = hwnd
	procSetLayeredWindowAttributes.Call(hwnd, 0x00FF00FF, 0, lwaColorKey)
	positionIndicator(hwnd)

	filter := deviceBroadcastInterface{
		Size:       uint32(unsafe.Sizeof(deviceBroadcastInterface{})),
		DeviceType: dbtDevtypDeviceInterface, Class: keyboardInterfaceGUID,
	}
	deviceNotification, _, err = procRegisterDeviceNotificationW.Call(
		hwnd, uintptr(unsafe.Pointer(&filter)), deviceNotifyWindowHandle,
	)
	if deviceNotification == 0 {
		return fmt.Errorf("register device notification: %w", err)
	}
	defer procUnregisterDeviceNotification.Call(deviceNotification)
	procSetTimer.Call(hwnd, indicatorEventTimer, 1, 0)
	return messageLoop()
}

func indicatorWndProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	switch message {
	case wmDeviceChange:
		if wParam == dbtDeviceRemoveComplete {
			// Removal is the latency-sensitive path: enumerate immediately instead
			// of waiting for the event burst to finish.
			refreshIndicator(hwnd)

			// Reconcile once more after Windows has removed the remaining
			// interfaces of a composite USB keyboard.
			procKillTimer.Call(hwnd, indicatorEventTimer)
			procSetTimer.Call(hwnd, indicatorEventTimer, 200, 0)
		} else if wParam == dbtDeviceArrival {
			// Best-effort only: this is not what bounds arrival latency, since
			// the regular poll usually shows the dot sooner than this fires.
			procKillTimer.Call(hwnd, indicatorEventTimer)
			procSetTimer.Call(hwnd, indicatorEventTimer, 350, 0)
		}
		return 0
	case wmTimer:
		if wParam == indicatorEventTimer {
			procKillTimer.Call(hwnd, indicatorEventTimer)
			refreshIndicator(hwnd)
		} else if wParam == indicatorPollTimer {
			procKillTimer.Call(hwnd, indicatorPollTimer)
			refreshIndicator(hwnd)
		}
		return 0
	case wmDisplayChange:
		positionIndicator(hwnd)
		return 0
	case wmReloadConfig:
		if cfg, err := loadConfig(); err == nil {
			applyIndicatorConfig(cfg)
			positionIndicator(hwnd)
			procInvalidateRect.Call(hwnd, 0, 1)
			refreshIndicator(hwnd)
		}
		return 0
	case wmPaint:
		paintIndicator(hwnd)
		return 0
	case wmClose:
		procDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	result, _, _ := procDefWindowProcW.Call(hwnd, uintptr(message), wParam, lParam)
	return result
}

func applyIndicatorConfig(cfg config) {
	normalizeConfig(&cfg)
	if cfg.MouseMode == "" {
		if choices, err := mouseChoices(); err == nil && len(choices) == 1 {
			cfg.MouseMode = choices[0].Mode
			cfg.MouseID = choices[0].ID
			cfg.MouseSlot = choices[0].Slot
		}
	}
	indicatorKeyboardMatch = cfg.KeyboardMatch
	indicatorMouseMode = cfg.MouseMode
	indicatorMouseID = cfg.MouseID
	indicatorMouseSlot = cfg.MouseSlot
	indicatorPollInterval = uintptr(cfg.PollInterval)
	indicatorPosition = cfg.Position
	indicatorSize = int32(cfg.Size)
	indicatorOffsetX = int32(cfg.OffsetX)
	indicatorOffsetY = int32(cfg.OffsetY)
}

func notifyIndicatorReload() {
	hwnd, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(utf16("USBIndicatorWindow"))), 0)
	if hwnd != 0 {
		procPostMessageW.Call(hwnd, wmReloadConfig, 0, 0)
	}
}

func disableIndicator() {
	hwnd, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(utf16("USBIndicatorWindow"))), 0)
	if hwnd != 0 {
		procSendMessageW.Call(hwnd, wmClose, 0, 0)
	}
}

func refreshIndicator(hwnd uintptr) {
	keyboardOn, keyboardErr := keyboardPresent(indicatorKeyboardMatch)
	mouseOn, mouseErr := mousePresent()
	changed := keyboardOn != indicatorKeyboardOn || mouseOn != indicatorMouseOn
	indicatorKeyboardOn = keyboardErr == nil && keyboardOn
	indicatorMouseOn = mouseErr == nil && mouseOn
	if changed {
		procInvalidateRect.Call(hwnd, 0, 1)
	}
	if indicatorKeyboardOn || indicatorMouseOn {
		positionIndicator(hwnd)
		procShowWindow.Call(hwnd, swShowNoActivate)
		procUpdateWindow.Call(hwnd)
	} else {
		procShowWindow.Call(hwnd, swHide)
	}

	// Device notifications arrive with unbounded, unpredictable latency, so
	// polling is what actually bounds the indicator's update latency. The
	// The configured interval is used in both states: while hidden we still
	// need to notice a re-plug (arrival notifications are unreliable too), and
	// while visible we need to notice removal promptly. Device notifications
	// are only a bonus when they happen to arrive within that window.
	procKillTimer.Call(hwnd, indicatorPollTimer)
	procSetTimer.Call(hwnd, indicatorPollTimer, indicatorPollInterval, 0)
}

func paintIndicator(hwnd uintptr) {
	var ps paintStruct
	dc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	var r rect
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	background, _, _ := procCreateSolidBrush.Call(0x00FF00FF)
	procFillRect.Call(dc, uintptr(unsafe.Pointer(&r)), background)
	procDeleteObject.Call(background)
	if indicatorKeyboardOn {
		paintDot(dc, 0, 0x0058D26A)
	}
	if indicatorMouseOn {
		paintDot(dc, indicatorSize+indicatorGap(), 0x00E39B36)
	}
	procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
}

func paintDot(dc uintptr, left int32, color uintptr) {
	brush, _, _ := procCreateSolidBrush.Call(color)
	old, _, _ := procSelectObject.Call(dc, brush)
	procEllipse.Call(dc, uintptr(left+2), 2, uintptr(left+indicatorSize-2), uintptr(indicatorSize-2))
	procSelectObject.Call(dc, old)
	procDeleteObject.Call(brush)
}

func indicatorGap() int32 {
	// Each ellipse is inset by 2 px inside its cell. A -3 px cell gap leaves
	// roughly 1 px between the visible circle edges without overlap.
	return -3
}

func indicatorWidth() int32 {
	if indicatorMouseMode == "" {
		return indicatorSize
	}
	return indicatorSize*2 + indicatorGap()
}

func positionIndicator(hwnd uintptr) {
	monitor, _, _ := procMonitorFromWindow.Call(hwnd, monitorDefaultNear)
	info := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
	if ok, _, _ := procGetMonitorInfoW.Call(monitor, uintptr(unsafe.Pointer(&info))); ok != 0 {
		const margin int32 = 12
		width := indicatorWidth()
		x := info.Work.Right - width - margin
		y := info.Work.Top + margin
		switch indicatorPosition {
		case "top-left":
			x = info.Work.Left + margin
		case "top-center":
			x = info.Work.Left + (info.Work.Right-info.Work.Left-width)/2
		case "center-left":
			x = info.Work.Left + margin
			y = info.Work.Top + (info.Work.Bottom-info.Work.Top-indicatorSize)/2
		case "center-right":
			y = info.Work.Top + (info.Work.Bottom-info.Work.Top-indicatorSize)/2
		case "bottom-left":
			x = info.Work.Left + margin
			y = info.Work.Bottom - indicatorSize - margin
		case "bottom-center":
			x = info.Work.Left + (info.Work.Right-info.Work.Left-width)/2
			y = info.Work.Bottom - indicatorSize - margin
		case "bottom-right":
			y = info.Work.Bottom - indicatorSize - margin
		case "custom":
			x = info.Work.Left + indicatorOffsetX
			y = info.Work.Top + indicatorOffsetY
		}
		if x+width > info.Work.Right {
			x = info.Work.Right - width
		}
		if x < info.Work.Left {
			x = info.Work.Left
		}
		if y+indicatorSize > info.Work.Bottom {
			y = info.Work.Bottom - indicatorSize
		}
		if y < info.Work.Top {
			y = info.Work.Top
		}
		procSetWindowPos.Call(hwnd, hwndTopmost, uintptr(x), uintptr(y), uintptr(width), uintptr(indicatorSize), swpNoActivate)
	}
}

func mousePresent() (bool, error) {
	switch indicatorMouseMode {
	case "":
		return false, nil
	case "bluetooth":
		return bluetoothMousePresent(indicatorMouseID)
	case "unifying":
		return unifyingMousePresent(indicatorMouseID, indicatorMouseSlot, int(indicatorPollInterval))
	default:
		return false, fmt.Errorf("unsupported mouse mode %q", indicatorMouseMode)
	}
}
