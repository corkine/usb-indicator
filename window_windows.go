//go:build windows

package main

import (
	"errors"
	"fmt"
	"strconv"
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

	indicatorEventTimer = 1
	indicatorPollTimer  = 2
	pollWhenPresentMS   = 100
	pollWhenAbsentMS    = 100
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

var (
	indicatorMatch        string
	indicatorPosition     = defaultPosition
	indicatorSize         = int32(defaultSize)
	indicatorOffsetX      int32
	indicatorOffsetY      int32
	indicatorWindow       uintptr
	deviceNotification    uintptr
	selectorChoices       []keyboardChoice
	selectorResult        config
	selectorSaved         bool
	errSelectionCancelled = errors.New("keyboard selection cancelled")
)

func utf16(s string) *uint16 { return syscall.StringToUTF16Ptr(s) }

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

func runIndicator(cfg config) error {
	applyIndicatorConfig(cfg)
	if err := registerClass("USBIndicatorWindow", syscall.NewCallback(indicatorWndProc), 0); err != nil {
		return err
	}
	hwnd, err := createWindow(
		wsExTopmost|wsExToolWindow|wsExLayered|wsExTransparent|wsExNoActivate,
		"USBIndicatorWindow", "Keyboard indicator", wsPopup, 0, 0, indicatorSize, indicatorSize, 0, 0,
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
			// the 100ms poll usually shows the dot sooner than this fires.
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
	indicatorMatch = cfg.Match
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
	present, err := keyboardPresent(indicatorMatch)
	if err == nil && present {
		positionIndicator(hwnd)
		procShowWindow.Call(hwnd, swShowNoActivate)
		procUpdateWindow.Call(hwnd)
	} else {
		procShowWindow.Call(hwnd, swHide)
	}

	// Device notifications arrive with unbounded, unpredictable latency, so
	// polling is what actually bounds the indicator's update latency. The
	// interval is kept at 100ms in both states: while hidden we still need to
	// notice a re-plug (arrival notifications are unreliable too), and while
	// visible we need to notice removal promptly. Device notifications are
	// only a bonus when they happen to arrive within that window.
	interval := uintptr(pollWhenAbsentMS)
	if present {
		interval = pollWhenPresentMS
	}
	procKillTimer.Call(hwnd, indicatorPollTimer)
	procSetTimer.Call(hwnd, indicatorPollTimer, interval, 0)
}

func paintIndicator(hwnd uintptr) {
	var ps paintStruct
	dc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	var r rect
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	background, _, _ := procCreateSolidBrush.Call(0x00FF00FF)
	procFillRect.Call(dc, uintptr(unsafe.Pointer(&r)), background)
	procDeleteObject.Call(background)
	green, _, _ := procCreateSolidBrush.Call(0x0058D26A)
	old, _, _ := procSelectObject.Call(dc, green)
	procEllipse.Call(dc, 2, 2, uintptr(r.Right-2), uintptr(r.Bottom-2))
	procSelectObject.Call(dc, old)
	procDeleteObject.Call(green)
	procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
}

func positionIndicator(hwnd uintptr) {
	monitor, _, _ := procMonitorFromWindow.Call(hwnd, monitorDefaultNear)
	info := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
	if ok, _, _ := procGetMonitorInfoW.Call(monitor, uintptr(unsafe.Pointer(&info))); ok != 0 {
		const margin int32 = 12
		x := info.Work.Right - indicatorSize - margin
		y := info.Work.Top + margin
		switch indicatorPosition {
		case "top-left":
			x = info.Work.Left + margin
		case "top-center":
			x = info.Work.Left + (info.Work.Right-info.Work.Left-indicatorSize)/2
		case "center-left":
			x = info.Work.Left + margin
			y = info.Work.Top + (info.Work.Bottom-info.Work.Top-indicatorSize)/2
		case "center-right":
			y = info.Work.Top + (info.Work.Bottom-info.Work.Top-indicatorSize)/2
		case "bottom-left":
			x = info.Work.Left + margin
			y = info.Work.Bottom - indicatorSize - margin
		case "bottom-center":
			x = info.Work.Left + (info.Work.Right-info.Work.Left-indicatorSize)/2
			y = info.Work.Bottom - indicatorSize - margin
		case "bottom-right":
			y = info.Work.Bottom - indicatorSize - margin
		case "custom":
			x = info.Work.Left + indicatorOffsetX
			y = info.Work.Top + indicatorOffsetY
		}
		procSetWindowPos.Call(hwnd, hwndTopmost, uintptr(x), uintptr(y), uintptr(indicatorSize), uintptr(indicatorSize), swpNoActivate)
	}
}

func selectAndSaveKeyboard() error {
	choices, err := keyboardChoices()
	if err != nil {
		return err
	}
	current, _ := loadConfig()
	normalizeConfig(&current)
	if current.Match != "" {
		found := false
		for _, choice := range choices {
			if choice.Match == current.Match {
				found = true
				break
			}
		}
		if !found {
			choices = append([]keyboardChoice{{Match: current.Match, Path: "（当前配置，设备暂不在本机）"}}, choices...)
		}
	}
	if len(choices) == 0 {
		return errors.New("没有检测到键盘设备")
	}
	selectorChoices = choices
	selectorResult = config{}
	selectorSaved = false
	if err := registerClass("USBIndicatorSelector", syscall.NewCallback(selectorWndProc), 6); err != nil {
		return err
	}
	title := fmt.Sprintf("USB Keyboard Indicator 设置 - %s", version)
	hwnd, err := createWindow(0, "USBIndicatorSelector", title, wsCaption|wsSysMenu, 200, 200, 760, 480, 0, 0)
	if err != nil {
		return err
	}
	if _, err = createWindow(0, "STATIC", "目标键盘", wsChild|wsVisible, 12, 12, 160, 20, hwnd, 0); err != nil {
		return err
	}
	list, err := createWindow(0, "LISTBOX", "", wsChild|wsVisible|wsVScroll|wsBorder|lbsNotify, 12, 35, 720, 245, hwnd, selectListID)
	if err != nil {
		return err
	}
	selectedKeyboard := 0
	for i, choice := range choices {
		label := choice.Match + "    " + choice.Path
		procSendMessageW.Call(list, lbAddString, 0, uintptr(unsafe.Pointer(utf16(label))))
		if choice.Match == current.Match {
			selectedKeyboard = i
		}
	}
	procSendMessageW.Call(list, lbSetCurSel, uintptr(selectedKeyboard), 0)

	if _, err = createWindow(0, "STATIC", "位置", wsChild|wsVisible, 12, 305, 55, 24, hwnd, 0); err != nil {
		return err
	}
	position, err := createWindow(0, "COMBOBOX", "", wsChild|wsVisible|wsVScroll|cbsDropDownList, 70, 300, 210, 220, hwnd, selectPositionID)
	if err != nil {
		return err
	}
	positionLabels := []string{"左上", "上方居中", "右上", "左侧居中", "右侧居中", "左下", "下方居中", "右下", "自定义（左上角偏移）"}
	selectedPosition := 2
	for i, label := range positionLabels {
		procSendMessageW.Call(position, cbAddString, 0, uintptr(unsafe.Pointer(utf16(label))))
		if validPositions[i] == current.Position {
			selectedPosition = i
		}
	}
	procSendMessageW.Call(position, cbSetCurSel, uintptr(selectedPosition), 0)

	if _, err = createWindow(0, "STATIC", "大小（8–128 px）", wsChild|wsVisible, 320, 305, 135, 24, hwnd, 0); err != nil {
		return err
	}
	if _, err = createWindow(0, "EDIT", strconv.Itoa(current.Size), wsChild|wsVisible|wsBorder|esNumber, 460, 300, 80, 26, hwnd, selectSizeID); err != nil {
		return err
	}
	if _, err = createWindow(0, "STATIC", "自定义偏移：X", wsChild|wsVisible, 12, 345, 110, 24, hwnd, 0); err != nil {
		return err
	}
	if _, err = createWindow(0, "EDIT", strconv.Itoa(current.OffsetX), wsChild|wsVisible|wsBorder|esNumber, 125, 340, 80, 26, hwnd, selectOffsetXID); err != nil {
		return err
	}
	if _, err = createWindow(0, "STATIC", "Y", wsChild|wsVisible, 225, 345, 20, 24, hwnd, 0); err != nil {
		return err
	}
	if _, err = createWindow(0, "EDIT", strconv.Itoa(current.OffsetY), wsChild|wsVisible|wsBorder|esNumber, 250, 340, 80, 26, hwnd, selectOffsetYID); err != nil {
		return err
	}
	if _, err = createWindow(0, "STATIC", "px（相对工作区左上角）", wsChild|wsVisible, 342, 345, 180, 24, hwnd, 0); err != nil {
		return err
	}
	if _, err = createWindow(0, "STATIC", "版本："+version, wsChild|wsVisible, 12, 405, 240, 24, hwnd, 0); err != nil {
		return err
	}
	if _, err = createWindow(0, "BUTTON", "保存", wsChild|wsVisible|bsPushButton, 532, 400, 96, 30, hwnd, selectSaveID); err != nil {
		return err
	}
	if _, err = createWindow(0, "BUTTON", "取消", wsChild|wsVisible|bsPushButton, 636, 400, 96, 30, hwnd, selectCancelID); err != nil {
		return err
	}
	procShowWindow.Call(hwnd, swShow)
	procUpdateWindow.Call(hwnd)
	if err := messageLoop(); err != nil {
		return err
	}
	if !selectorSaved {
		return errSelectionCancelled
	}
	return saveConfig(selectorResult)
}

func selectorWndProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	switch message {
	case wmCommand:
		id := uint16(wParam & 0xffff)
		if id == selectSaveID {
			list := findChild(hwnd, selectListID)
			keyboardIndex, _, _ := procSendMessageW.Call(list, lbGetCurSel, 0, 0)
			positionIndex, _, _ := procSendMessageW.Call(findChild(hwnd, selectPositionID), cbGetCurSel, 0, 0)
			if keyboardIndex == lbErr || int(keyboardIndex) >= len(selectorChoices) || int(positionIndex) >= len(validPositions) {
				showError(errors.New("请选择键盘和指示器位置"))
				return 0
			}
			size, err := readIntControl(hwnd, selectSizeID)
			if err != nil || size < minSize || size > maxSize {
				showError(fmt.Errorf("圆点大小必须在 %d 到 %d px 之间", minSize, maxSize))
				return 0
			}
			offsetX, errX := readIntControl(hwnd, selectOffsetXID)
			offsetY, errY := readIntControl(hwnd, selectOffsetYID)
			if errX != nil || errY != nil || offsetX < 0 || offsetX > maxOffset || offsetY < 0 || offsetY > maxOffset {
				showError(fmt.Errorf("X/Y 偏移必须在 0 到 %d px 之间", maxOffset))
				return 0
			}
			selectorResult = config{
				Match:    selectorChoices[keyboardIndex].Match,
				Position: validPositions[positionIndex],
				Size:     size,
				OffsetX:  offsetX,
				OffsetY:  offsetY,
			}
			selectorSaved = true
			procDestroyWindow.Call(hwnd)
			return 0
		}
		if id == selectCancelID {
			procDestroyWindow.Call(hwnd)
			return 0
		}
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

var procGetDlgItem = user32.NewProc("GetDlgItem")

func findChild(parent, id uintptr) uintptr {
	hwnd, _, _ := procGetDlgItem.Call(parent, id)
	return hwnd
}

func readIntControl(parent, id uintptr) (int, error) {
	buffer := make([]uint16, 16)
	procGetWindowTextW.Call(findChild(parent, id), uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
	return strconv.Atoi(syscall.UTF16ToString(buffer))
}

func showError(err error) {
	text := "USB Indicator: " + err.Error()
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(utf16(text))), uintptr(unsafe.Pointer(utf16("USB Indicator"))), 0x10)
}
