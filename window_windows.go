//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

type point struct{ X, Y int32 }
type rect struct{ Left, Top, Right, Bottom int32 }
type msg struct {
	Window  uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Point   point
}
type wndClassEx struct {
	Size, Style            uint32
	WndProc                uintptr
	ClsExtra, WndExtra     int32
	Instance, Icon, Cursor uintptr
	Background             uintptr
	MenuName, ClassName    *uint16
	IconSmall              uintptr
}
type paintStruct struct {
	DC        uintptr
	Erase     int32
	Paint     rect
	Restore   int32
	IncUpdate int32
	Reserved  [32]byte
}
type monitorInfo struct {
	Size    uint32
	Monitor rect
	Work    rect
	Flags   uint32
}
type deviceBroadcastInterface struct {
	Size       uint32
	DeviceType uint32
	Reserved   uint32
	Class      guid
	Name       uint16
}

const (
	wmDestroy       = 0x0002
	wmPaint         = 0x000F
	wmClose         = 0x0010
	wmCommand       = 0x0111
	wmTimer         = 0x0113
	wmDeviceChange  = 0x0219
	wmDisplayChange = 0x007E
	wmReloadConfig  = 0x8001

	dbtDeviceArrival         = 0x8000
	dbtDeviceRemoveComplete  = 0x8004
	dbtDevtypDeviceInterface = 0x00000005
	deviceNotifyWindowHandle = 0x00000000

	wsPopup         = 0x80000000
	wsCaption       = 0x00C00000
	wsSysMenu       = 0x00080000
	wsVisible       = 0x10000000
	wsChild         = 0x40000000
	wsVScroll       = 0x00200000
	wsBorder        = 0x00800000
	lbsNotify       = 0x0001
	bsPushButton    = 0x00000000
	cbsDropDownList = 0x0003
	esNumber        = 0x2000

	wsExTopmost     = 0x00000008
	wsExToolWindow  = 0x00000080
	wsExLayered     = 0x00080000
	wsExTransparent = 0x00000020
	wsExNoActivate  = 0x08000000

	swHide             = 0
	swShow             = 5
	swShowNoActivate   = 8
	swpNoActivate      = 0x0010
	hwndTopmost        = ^uintptr(0)
	monitorDefaultNear = 2
	lwaColorKey        = 0x00000001

	lbAddString      = 0x0180
	lbGetCurSel      = 0x0188
	lbSetCurSel      = 0x0186
	lbErr            = ^uintptr(0)
	cbAddString      = 0x0143
	cbGetCurSel      = 0x0147
	cbSetCurSel      = 0x014E
	selectListID     = 1001
	selectSaveID     = 1002
	selectCancelID   = 1003
	selectPositionID = 1004
	selectSizeID     = 1005
	selectOffsetXID  = 1006
	selectOffsetYID  = 1007
	selectMouseID    = 1008
	selectPollID     = 1009

	indicatorEventTimer = 1
	indicatorPollTimer  = 2
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procRegisterClassExW             = user32.NewProc("RegisterClassExW")
	procCreateWindowExW              = user32.NewProc("CreateWindowExW")
	procDefWindowProcW               = user32.NewProc("DefWindowProcW")
	procShowWindow                   = user32.NewProc("ShowWindow")
	procUpdateWindow                 = user32.NewProc("UpdateWindow")
	procGetMessageW                  = user32.NewProc("GetMessageW")
	procTranslateMessage             = user32.NewProc("TranslateMessage")
	procDispatchMessageW             = user32.NewProc("DispatchMessageW")
	procPostQuitMessage              = user32.NewProc("PostQuitMessage")
	procDestroyWindow                = user32.NewProc("DestroyWindow")
	procSetTimer                     = user32.NewProc("SetTimer")
	procKillTimer                    = user32.NewProc("KillTimer")
	procBeginPaint                   = user32.NewProc("BeginPaint")
	procEndPaint                     = user32.NewProc("EndPaint")
	procGetClientRect                = user32.NewProc("GetClientRect")
	procFillRect                     = user32.NewProc("FillRect")
	procSetLayeredWindowAttributes   = user32.NewProc("SetLayeredWindowAttributes")
	procSetWindowPos                 = user32.NewProc("SetWindowPos")
	procMonitorFromWindow            = user32.NewProc("MonitorFromWindow")
	procGetMonitorInfoW              = user32.NewProc("GetMonitorInfoW")
	procRegisterDeviceNotificationW  = user32.NewProc("RegisterDeviceNotificationW")
	procUnregisterDeviceNotification = user32.NewProc("UnregisterDeviceNotification")
	procSendMessageW                 = user32.NewProc("SendMessageW")
	procMessageBoxW                  = user32.NewProc("MessageBoxW")
	procFindWindowW                  = user32.NewProc("FindWindowW")
	procPostMessageW                 = user32.NewProc("PostMessageW")
	procInvalidateRect               = user32.NewProc("InvalidateRect")
	procGetWindowTextW               = user32.NewProc("GetWindowTextW")
	procGetModuleHandleW             = kernel32.NewProc("GetModuleHandleW")
	procCreateMutexW                 = kernel32.NewProc("CreateMutexW")
	procCloseHandle                  = kernel32.NewProc("CloseHandle")
	procCreateSolidBrush             = gdi32.NewProc("CreateSolidBrush")
	procDeleteObject                 = gdi32.NewProc("DeleteObject")
	procEllipse                      = gdi32.NewProc("Ellipse")
	procSelectObject                 = gdi32.NewProc("SelectObject")
)

func utf16(s string) *uint16 { return syscall.StringToUTF16Ptr(s) }

func registerClass(name string, wndProc uintptr, background uintptr) error {
	instance, _, _ := procGetModuleHandleW.Call(0)
	wc := wndClassEx{
		Size: uint32(unsafe.Sizeof(wndClassEx{})), WndProc: wndProc,
		Instance: instance, Background: background, ClassName: utf16(name),
	}
	atom, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 && err != syscall.Errno(1410) {
		return err
	}
	return nil
}

func createWindow(exStyle uint32, class, title string, style uint32, x, y, w, h int32, parent, menu uintptr) (uintptr, error) {
	instance, _, _ := procGetModuleHandleW.Call(0)
	hwnd, _, err := procCreateWindowExW.Call(
		uintptr(exStyle), uintptr(unsafe.Pointer(utf16(class))), uintptr(unsafe.Pointer(utf16(title))), uintptr(style),
		uintptr(x), uintptr(y), uintptr(w), uintptr(h), parent, menu, instance, 0,
	)
	if hwnd == 0 {
		return 0, err
	}
	return hwnd, nil
}

func messageLoop() error {
	var message msg
	for {
		result, _, err := procGetMessageW.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		if int32(result) == -1 {
			return err
		}
		if result == 0 {
			return nil
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&message)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&message)))
	}
}
